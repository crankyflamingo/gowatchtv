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

var cacheMutex = new(sync.Mutex)
var cacheTrimRunning = false
var clientCache = map[string]*TemporalCache{}

// shuttleData moves data back and forth between client and intended destination
func shuttleData(src, dst net.Conn, wg *sync.WaitGroup) {
	// TODO: There is likely a sexier way to do this using channels and the like
	buf := make([]byte, 2048)
	defer wg.Done()
	for {
		rlen, err := src.Read(buf)
		if err == nil && rlen > 0 {
			_, err = dst.Write(buf[:rlen])
		} else {
			//fmt.Printf("Shuttle data complete to %s <-> %s\n", src.RemoteAddr().String(), dst.RemoteAddr().String())
			return
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
	cacheMutex.Lock()
	clientCache[client] = temp
	cacheMutex.Unlock()
}

func trimClientCache() {
	for {
		time.Sleep(30 * time.Minute)
		fmt.Println("Flushing old cached client entries (", len(clientCache), " total )")

		cacheMutex.Lock()
		for client, temp := range clientCache {
			if time.Since(temp.set) > cacheExpiry {
				delete(clientCache, client)
			}
		}
		cacheMutex.Unlock()
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

	defer src.Close()

	fmt.Println("\nConnection on port", port)

	buf := make([]byte, 2048)
	host := ""
	ip := ""

	rlen, err := src.Read(buf)
	if err != nil {
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
		fmt.Printf("Request to unknown host %s, dropping", data[:20])
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
		return
	}

	// keep our client's intended destination cached for a while in case we need it
	updateClientCache(src.RemoteAddr().String(), ip)

	// connect to intended recipient, shuttle data between client and intended host
	fmt.Printf("MITMing: Host: %s, IP: %s, Port: %s\n", host, ip, port)
	dst, err := net.Dial("tcp", ip+port)
	if err != nil {
		fmt.Println("Unable to contact intended IP", host, src.RemoteAddr().String(), ip, err.Error())
		return
	}
	defer dst.Close()

	// using channels to send data to and fro

	// send data that was originally meant for dest that we've already grabbed and molested
	_, err = dst.Write(buf[:rlen])
	if err != nil {
		fmt.Println("Unable to write data to intended IP", ip, port, err.Error())
		return
	}

	wg := new(sync.WaitGroup)
	wg.Add(2)

	go shuttleData(dst, src, wg)
	go shuttleData(src, dst, wg)

	// defer close of connections after traffic shuttling complete
	wg.Wait()
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
	//if !cacheTrimRunning {
	//	cacheTrimRunning = true
	//	go trimClientCache()
	//}

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
