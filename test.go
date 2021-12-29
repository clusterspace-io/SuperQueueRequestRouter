package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

const url = "https://localhost:9090/hc"

var httpVersion = flag.Int("version", 2, "HTTP version")

func main() {
	flag.Parse()
	client := &http.Client{}

	// Create a pool with the server certificate since it is not signed
	// by a known CA
	caCert, err := ioutil.ReadFile("../SuperQueueRequestRouter/domain.crt")
	if err != nil {
		log.Fatalf("Reading server certificate: %s", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Create TLS configuration with the certificate of the server
	tlsConfig := &tls.Config{
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}

	// Use the proper transport in the client
	switch *httpVersion {
	case 1:
		client.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
		log.Println("Using http 1")
	case 2:
		client.Transport = &http2.Transport{
			TLSClientConfig: tlsConfig,
		}
		log.Println("Using http 2")
	}

	// Perform the request
	s := time.Now()
	for i := 0; i < 10000; i++ {
		resp, err := client.Get(url)
		if err != nil {
			log.Fatalf("Failed get: %s", err)
		}
		resp.Body.Close()
	}
	log.Println("Done in", time.Since(s))
	// defer resp.Body.Close()
	// body, err := ioutil.ReadAll(resp.Body)
	// if err != nil {
	// 	log.Fatalf("Failed reading response body: %s", err)
	// }
	// fmt.Printf(
	// 	"Got response %d: %s %s\n",
	// 	resp.StatusCode, resp.Proto, string(body))
}
