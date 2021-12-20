package main

import (
	"math/rand"
	"time"
)

type NackRecordRequest struct {
	DelayMS *float64 `json:"delay_ms"`
}

// Gets a random partition from a queue
func GetRandomPartition(p *PartitionStoredRecord) PartitionSDRecord {
	rand.Seed(time.Now().UnixNano())
	index := rand.Intn(len(*p))
	return (*p)[index]
}

func GetPartition(p *PartitionStoredRecord, partitionID string) *PartitionSDRecord {
	for _, i := range *p {
		if i.Partition == partitionID {
			return &i
		}
	}
	return nil
}
