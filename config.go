package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

type Configuration struct {
	UPSTREAM_DNS     string
	EXTERNAL_ADDRESS string
	DNS_PORT         string
	INTERCEPTS       []string
	DEBUG            bool
}

// Implements the configuration loading
func (conf *Configuration) GetConfig() {

	buf, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("error with config:", err.Error())
		return
	}

	err = json.Unmarshal(buf, &conf)
	if err != nil {
		fmt.Println("error with config:", err.Error())
	}
	return
}
