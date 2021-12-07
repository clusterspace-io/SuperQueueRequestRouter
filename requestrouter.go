package main

type RequestRouter struct {
	PartitionMap map[string][]Partition
}

type Partition struct {
	Weight  int
	Address string
}
