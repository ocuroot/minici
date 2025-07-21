package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/ocuroot/minici"
)

func main() {
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()
	address := fmt.Sprintf(":%d", *port)

	ciServer := minici.NewCIServer()
	server := NewRESTServer(ciServer, address)

	err := server.Start()
	if err != nil {
		log.Fatalf("%v", err)
	}
}
