package main

import (
	"math/rand"
	"time"
)

type NackRecordRequest struct {
	DelayMS *float64 `json:"delay_ms"`
}

// Gets a random partition from a queue, ignores draining partitions
func GetRandomPartition(p *PartitionStoredRecord) *PartitionSDRecord {
	var nonDrainingPartitions []PartitionSDRecord
	for _, i := range *p {
		if !i.IsDraining {
			nonDrainingPartitions = append(nonDrainingPartitions, i)
		}
	}
	if len(nonDrainingPartitions) == 0 {
		return nil
	}
	rand.Seed(time.Now().UnixNano())
	index := rand.Intn(len(nonDrainingPartitions))
	return &nonDrainingPartitions[index]
}

// Gets a specific partition of a queue, includes draining partitions
func GetPartition(p *PartitionStoredRecord, partitionID string) *PartitionSDRecord {
	for _, i := range *p {
		if i.Partition == partitionID {
			return &i
		}
	}
	return nil
}
