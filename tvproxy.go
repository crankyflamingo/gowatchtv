// tvproxy main entry
package main

import "fmt"

var config = new(Configuration)

func main() {

	config.GetConfig()
	fmt.Println("Starting with config:", config)

	//mitmlock <- 1
	go MitmServer(":80")
	go MitmServer(":443")
	go trimRemoteClientMap()
	// TODO: Have server address and intercepts etc. in a config file
	TvproxySrv(config.DNS_PORT)
	//fmt.Println("Server initiated")
}

// on connect, lookup address and
