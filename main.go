package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/labstack/gommon/log"
)

func main() {
	go func() {
		StartHTTPServer()
	}()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Info("Closing server")
	Server.Echo.Close()
}
