package main

import (
	"fmt"
	"net"
	"regexp"
	"sync"
	"time"
)

const (
	cacheExpiry = 10 * time.Minute
)

type TemporalCache struct {
	set  time.Time
	dest string
}

var mutex = new(sync.Mutex)
var cacheTrimRunning = false
var clientCache = map[string]*TemporalCache{}

// shuttleData moves data back and forth between client and intended destination
func shuttleData(src, dst net.Conn) {
	// TODO: There is likely a sexier way to do this using channels and the like
	defer src.Close()

	buf := make([]byte, 1024)
	for {
		rlen, err := src.Read(buf)
		if err == nil && rlen > 0 {
			dst.Write(buf[:rlen])
		} else {
			break
		}
	}
}

func getClientFromCache(client string) (ip string) {
	if clientCache[client] != nil {
		return clientCache[client].dest
	}
	return ""
}

func updateClientCache(client string, ip string) {
	temp := new(TemporalCache)
	temp.dest = ip
	temp.set = time.Now()
	mutex.Lock()
	clientCache[client] = temp
	mutex.Unlock()
}

func trimClientCache() {
	for {
		time.Sleep(30 * time.Minute)
		fmt.Println("Flushing old cached client entries (", len(clientCache), " total )")

		mutex.Lock()
		for client, temp := range clientCache {
			if time.Since(temp.set) > cacheExpiry {
				delete(clientCache, client)
			}
		}
		mutex.Unlock()
	}
}

var httpHost = regexp.MustCompile("Host:\\s([^\\r]+)")
var httpsHost = regexp.MustCompile("\x00\x00.([\\w\\.\\-\\_]{10,50})\x00")

func extractHostFromRequest(data *string) (host string) {
	// we either get a HTTP connection so the host looks like:
	// Host: www.blah.com\x0d\x0a
	// or HTTPS where the host looks like:
	// <len byte>www.blah.com\x00
	if !matchesCriteria(*data) {
		//fmt.Println("Doesn't match criteria")
		return ""
	}
	matches := httpHost.FindStringSubmatch(*data)
	if matches != nil {
		//fmt.Printf("matches http: %s", strings.Join(matches, ","))
		return matches[1]
	}
	matches = httpsHost.FindStringSubmatch(*data)
	if matches != nil {
		//fmt.Printf("matches http: %s", strings.Join(matches, ","))
		return matches[1]
	}
	return ""
}

func handleConnection(src net.Conn, port string) {

	fmt.Println("\nConnection on port", port)

	buf := make([]byte, 512)
	host := ""
	ip := ""

	rlen, err := src.Read(buf)
	if err != nil {
		src.Close()
		return
	}
	data := string(buf)

	host = extractHostFromRequest(&data)
	//dataDump(buf)

	// We've found a hostname
	if host != "" {
		// check if it matches our criteria, if so we might be dealing with a cached dns entry
		// so we'll have to look up the address again
		if !inAddressCache(host) {
			updateCacheByName(host)
		}
		ip = getCachedIp(host)
	} else if !matchesCriteria(data) {
		// otherwise it's an unwanted connection
		fmt.Printf("Request to unknown host dropping")
		src.Close()
		return
	}

	// There was no match, see if we've seen the client before. This may happen when we get sessions that don't
	// re-submit a header
	if ip == "" {
		ip = getClientFromCache(src.RemoteAddr().String())
		if ip != "" {
			fmt.Println("Got clientCache hit for ip", ip)
		}
	}

	if ip == "" {
		fmt.Println("Unable to match incoming connection to known destination", host, src.RemoteAddr().String())
		src.Close()
		return
	}

	// keep our client's intended destination cached for a while in case we need it
	updateClientCache(src.RemoteAddr().String(), ip)

	// connect to intended recipient, shuttle data between client and intended host
	fmt.Printf("MITMing: Host: %s, IP: %s, Port: %s\n", host, ip, port)
	dst, err := net.Dial("tcp", ip+port)
	if err != nil {
		fmt.Println("Unable to contact intended IP", host, src.RemoteAddr().String(), ip, err.Error())
		src.Close()
		return
	}

	// send data that was originally meant for dest that we've already grabbed and molested
	_, err = dst.Write(buf[:rlen])
	if err != nil {
		fmt.Println("Unable to write data to intended IP", ip, port, err.Error())
		src.Close()
		dst.Close()
		return
	}

	go shuttleData(dst, src)
	go shuttleData(src, dst)
}

// MitmServer listens on a given port and shuttles traffic (usually HTTP/HTTPS from client to intended destination)
func MitmServer(port string) {

	// Just specify port in ":<port>" to mean all interfaces
	ln, err := net.Listen("tcp", port)
	if err != nil {
		fmt.Println("Cannot listen on port", port, " are you running as admin?", err.Error())
		return
	}

	// only want the one thread checking the cache, but we'll run at least two instances of MitmServer
	if !cacheTrimRunning {
		cacheTrimRunning = true
		go trimClientCache()
	}

	fmt.Println("MITM server listening on", port, "for connections")
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error when trying to Accept connection, bailing", err.Error())
			return
		}
		go handleConnection(conn, port)
	}

}
