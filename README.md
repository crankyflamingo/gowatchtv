# gowatchtv
GO implementation of tvproxy, that appears to be a bit more stable than it's python predecessor.
First attempt at Go.

Essentially the purpose of this repo is to provide a means of proxying (MITM-ing) traffic to certain domains, to allow certain TV services to think you're in a different country, without having to use a VPN or pay some service to be your DNS provider (dodgy). In many cacses, only the initial account authentication traffic needs to be proxied, with the actual streaming part (high bandwidth) going directly to your device. Set your intercept substrings accordingly (see config below).

Basic setup is to run this in a cloud instance in the country/region you want to access (e.g. AWS free tier). You then make your machine/ipad/router use that external IP as your DNS server. Alternatively you can run a more legit DNS service (e.g. dnsmasq) somewhere local (e.g. on local network, or in-country cloud instance) and only route certain sites to this, for faster DNS lookups.

Uses a configuration file to determine which sites to proxy:
e.g.


{

	"UPSTREAM_DNS": "208.67.222.222:53",  // OPENDNS
	
	"EXTERNAL_ADDRESS": "1.2.3.4",  // This is the external address of your cloud instance
	
	"DNS_PORT": ":53",  // This is the DNS server port (only change if you're forwarding from local DNS server)
	
	"INTERCEPTS": [".netflix.com", ".hulu.com"] // add to these as appropriate. Not tested on all TV providers.
	
}
