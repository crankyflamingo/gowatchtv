// tvproxy main entry
package main

import "fmt"

var config = new(Configuration)
var DEBUG = false

func main() {

	config.GetConfig()
	fmt.Println("Starting with config:", config)
	DEBUG = config.DEBUG

	go MitmServer(":80")
	go MitmServer(":443")

	TvproxySrv(config.DNS_PORT)
}

// on connect, lookup address and
