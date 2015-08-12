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
var hostRegex = regexp.MustCompile("Host\\:\\s+(.*?)\\s*$")
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
		}
		break
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
	// TODO: check the age of the entry and remove ones older than cacheExpiry

}

func handleConnection(src net.Conn, port string) {

	buf := make([]byte, 512)
	rlen, err := src.Read(buf)
	if err != nil {
		src.Close()
		return
	}

	data := string(buf[:rlen])
	ip := ""

	// check the destination is one we know about
	host := hostRegex.FindString(data)

	// We've found a hostname
	if host != "" {
		// see if we've looked it up before
		if inAddressCache(host) || inAddressCache(host+".") {
			fmt.Println("Request to", host, "in our cache")
			ip = getCachedIp(host)
		} else if matchesCriteria(host) {
			// check if it matches our criteria, if so we might be dealing with a cached dns entry
			// so we'll have to look up the address again
			fmt.Println("Request to", host, "was not in cache, updating it")
			updateCacheByName(host)
			ip = getCachedIp(host)
		}
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

	go shuttleData(src, dst)
	go shuttleData(dst, src)
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
