package main

import (
	"math/rand"
	"time"
)

// Gets a random partition from a queue
func GetRandomPartition(p *PartitionStoredRecord) PartitionSDRecord {
	rand.Seed(time.Now().UnixNano())
	index := rand.Intn(len(*p))
	return (*p)[index]
}
