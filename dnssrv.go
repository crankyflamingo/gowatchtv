package main

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/miekg/dns"
)

var addressRegex = regexp.MustCompile("([\\d]{1,3}\\.[\\d]{1,3}\\.[\\d]{1,3}\\.[\\d]{1,3})$")
var addressMap = map[string]string{}
var chanlock = make(chan int, 1)

// updateAddressCache updates the dns map of real names to ip addresses
// used to relay traffic to the intended destination.
func updateAddressCache(r *dns.Msg) {
	u := r.Copy()

	u, err := upstreamLookup(u)
	if err != nil {
		fmt.Println("Network error: Cannnot lookup upstream!", err.Error())
		return
	}

	// find all A records, save the name and address, replace the address and ttl
	names := []string{}
	ip := ""

	// grab all names (and IP from A record), for updating (sans dot at end)
	for _, answer := range u.Answer {
		names = append(names, answer.Header().Name)
		if dns.TypeToString[answer.Header().Rrtype] == "A" {
			found := addressRegex.FindString(answer.String())
			if found != "" {
				ip = found
			} else {
				panic(r.String())
			}
		}
	}

	if ip == "" {
		fmt.Println("Cannot updated map, no IP found")
		fmt.Println(u.String())
		fmt.Println("")
		return
	}

	<-chanlock
	// Update both with . at the end and without
	for _, name := range names {
		addressMap[strings.ToLower(name)] = ip
		addressMap[strings.ToLower(name[:len(name)-1])] = ip
		fmt.Println("Updated address map with ", name, ip)
	}
	chanlock <- 1
}

// refreshCache periodically runs and updates DNS entries for all names in the cache (since they often
// change)
func refreshCache() {
	for {
		keys := []string{}
		time.Sleep(15 * time.Minute)

		for key, _ := range addressMap {
			keys = append(keys, key)
		}
		fmt.Println("Refreshing", len(keys), "domains in cache")
		// iterate seperately in the event modifying addressMap while iterating is bad.
		for _, key := range keys {
			updateCacheByName(key)
		}
	}
}

// upstreamLookup sends a dns.Msg to an upstream DNS provider for a legitimate return
func upstreamLookup(r *dns.Msg) (u *dns.Msg, err error) {
	c, err := net.Dial("udp", config.UPSTREAM_DNS)
	if err != nil {
		fmt.Println("Can't connect to upstream", err.Error())
		return
	}
	defer c.Close()

	co := &dns.Conn{Conn: c}
	if err = co.WriteMsg(r); err != nil {
		fmt.Println("Can't write to upstream", err.Error())
		return r, err
	}
	return co.ReadMsg()
}

// matchesCriteria checks to see if the name request is one we want to intercept
func matchesCriteria(name string) bool {
	for _, intercept := range config.INTERCEPTS {
		if strings.Contains(strings.ToLower(name), intercept) {
			fmt.Printf("%s matches criteria\n", name)
			return true
		}
	}
	return false
}

// hijackResponse retuns a modified version on the input dns.Msg with the A record modified
// to point to our server
func hijackResponse(r *dns.Msg) (m *dns.Msg) {
	m = new(dns.Msg)
	m.SetReply(r)
	m.Answer = make([]dns.RR, 1)

	rr := new(dns.A)
	rr.Hdr = dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 600}
	rr.A = net.ParseIP(config.EXTERNAL_ADDRESS)
	m.Answer[0] = rr
	return m
}

// inAddressCache returns whether a domain is in the cache
func inAddressCache(name string) bool {
	return addressMap[name] != ""
}

func getCachedIp(name string) string {
	if addressMap[name] == "" {
		if addressMap[name+"."] == "" {
			return ""
		}
		return addressMap[name+"."]
	} else {
		return addressMap[name]
	}
}

// interceptRequest gets called upon each DNS request, and we determine
// whether we want to deal with it or not
func interceptRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := r
	err := error(nil)
	defer w.Close()
	if len(r.Question) < 1 {
		return
	}

	// Hijack the response to point to us instead
	if matchesCriteria(r.Question[0].Name) {
		if !(inAddressCache(r.Question[0].Name)) {
			updateAddressCache(r)
		}
		m = hijackResponse(r)

		// Pass it upstream, return the answer
	} else {
		fmt.Println("Passing on ", r.Question[0].Name)
		m, err = upstreamLookup(r)
		if err != nil {
			fmt.Println("Error when passing request through upstream - network problem?")
		}
	}
	w.WriteMsg(m)
}

// updateCacheByName updates the cache entry for the FQDN name
func updateCacheByName(name string) {
	name = dns.Fqdn(name)
	fmt.Println("Updating entry for", name)
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA) // dns.TypeA
	updateAddressCache(m)
}

// TvproxySrv is the main exported method to run the dns server
// It will forward requests we're not interested in, otherwise it'll
// intercept for us and return the external address of this server as
// specified in the config
func TvproxySrv(port string) {

	// chanlock makes sure our global table doesn't hit race conditions
	chanlock <- 1
	pc, err := net.ListenPacket("udp", port)
	if err != nil {
		fmt.Printf("Cannot listen on address %s", port)
		return
	}

	fmt.Printf("Starting server on %s\n", port)

	srv := &dns.Server{Addr: port, Net: "udp", PacketConn: pc, Handler: dns.HandlerFunc(interceptRequest)}
	defer srv.Shutdown()

	// peridically update the cache
	go refreshCache()
	// start the dns server. Ctrl + C (etc) to kill
	srv.ActivateAndServe()
}
