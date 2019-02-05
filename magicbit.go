package main

import (
	"log"
	"net"
)

const (
	server_addr     = "224.0.0.1:9999"
)

func ping(server_addr string) {
	addr, err := net.ResolveUDPAddr("udp", server_addr)
	if err != nil {
		log.Fatal(err)
	}

	client, err := net.DialUDP("udp", nil, addr)
	log.Print("Placing magic on the wire")
	client.Write([]byte("RELOAD"))
}

func main() {
	ping(server_addr)
}
