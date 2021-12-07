package main

import (
	"math/rand"
	"time"
)

var (
	RR *RequestRouter
)

type RequestRouter struct {
	PartitionMap map[string][]*Partition
}

type Partition struct {
	Weight int
	// Including the protocol
	Address string
}

// Gets a random partition from a queue
func GetRandomPartition(queue string) *Partition {
	if q, exists := RR.PartitionMap[queue]; exists {
		rand.Seed(time.Now().UnixMilli())
		index := rand.Intn(len(q))
		return q[index]
	}
	return nil
}
