package main

import (
	"crypto/tls"
	"github.com/murakmii/c99-minimal-h2s/h2s"
	"log"
	"net/http"
	"os"
)

func main() {
	log.SetPrefix("[h2] ")

	cert, err := tls.LoadX509KeyPair(os.Args[1], os.Args[2])
	if err != nil {
		log.Panicf("failed to load certification file: %s", err)
	}

	h2s.NewServer(cert).ListenAndServe(":8080", http.HandlerFunc(handle))
}

func handle(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("<html><body><h1>Hello, HTTP/2!</h1></body></html>"))
}
