package main

import (
	"SuperQueueRequestRouter/logger"
	"os"
	"os/signal"
	"syscall"

	"github.com/labstack/gommon/log"
	"github.com/sirupsen/logrus"
)

func main() {
	logger.Logger.Logger.SetLevel(logrus.DebugLevel)
	logger.Info("Starting SuperQueueRequestRouter")
	// Create request router
	RR = &RequestRouter{
		PartitionMap: map[string][]*Partition{
			"test-queue": {
				&Partition{
					Weight:  1,
					Address: "http://localhost:8080",
					ID:      "aaa",
				},
				&Partition{
					Weight:  1,
					Address: "http://localhost:8081",
					ID:      "bbb",
				},
			},
		},
	}

	// Start http server
	go func() {
		StartHTTPServer()
	}()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Info("Closing server")
	Server.Echo.Close()
}
