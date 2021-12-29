package main

import (
	"SuperQueueRequestRouter/logger"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/labstack/gommon/log"
	"github.com/sirupsen/logrus"
)

func main() {
	logger.Logger.Logger.SetLevel(logrus.InfoLevel)
	rand.Seed(time.Now().UnixNano())
	if os.Getenv("TEST_MODE") == "true" {
		logger.Warn("TEST_MODE true, enabling cpu profiling")
		f, perr := os.Create("cpu.pprof")
		if perr != nil {
			panic(perr)
		}
		runtime.SetBlockProfileRate(100)
		perr = pprof.StartCPUProfile(f)
		if perr != nil {
			panic(perr)
		}
		defer f.Close()
		defer pprof.StopCPUProfile()
	}
	logger.Info("Starting SuperQueueRequestRouter")
	// Create request router
	// RR = &RequestRouter{
	// 	PartitionMap: map[string][]*Partition{
	// 		"test-queue": {
	// 			&Partition{
	// 				Weight:  1,
	// 				Address: "http://localhost:8080",
	// 				ID:      "aaa",
	// 			},
	// 			&Partition{
	// 				Weight:  1,
	// 				Address: "http://localhost:8081",
	// 				ID:      "bbb",
	// 			},
	// 		},
	// 	},
	// }

	// Start http server
	go func() {
		StartHTTPServer()
	}()

	TryEtcdConnect()
	SetupPartitionCache()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Info("Closing server")
	Server.Echo.Close()
}
