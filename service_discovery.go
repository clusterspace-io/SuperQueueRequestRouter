package main

import (
	"SuperQueueRequestRouter/logger"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/etcd-io/etcd/clientv3"
	"google.golang.org/grpc"
)

var (
	EtcdClient     *clientv3.Client
	PartitionCache *ristretto.Cache

	// Used to track the last time a queue was fetched (if less than TTL, we know we just refreshed cache)
	HerdMap = map[string]time.Time{}
	// Used to stop thundering herd
	HerdLock = sync.RWMutex{}
)

type PartitionSDRecord struct {
	QueueName  string
	Partition  string
	UpdatedAt  time.Time
	IsDraining bool
}

func TryEtcdConnect() {
	logger.Debug("Starting etcd based service discovery")
	hosts := strings.Split(ETCD_HOSTS, ",")
	logger.Debug("Using hosts: ", hosts)
	var err error
	EtcdClient, err = clientv3.New(clientv3.Config{
		Endpoints:   hosts,
		DialTimeout: 2 * time.Second,
		DialOptions: []grpc.DialOption{grpc.WithBlock()}, // Need this to actually block and fail on connect error
	})
	if err != nil {
		logger.Error("Failed to connect to etcd!")
		logger.Error(err)
		panic(err)
	} else {
		logger.Debug("Connected to etcd")
	}
}

func SetupPartitionCache() {
	logger.Debug("Setting up partition cache")
	var err error
	PartitionCache, err = ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e5,     // Track 100k keys
		MaxCost:     1 << 29, // 500MB of space allowed
		BufferItems: 64,
	})
	if err != nil {
		logger.Error("Error setting up partition cache")
		panic(err)
	}
	logger.Debug("Partition cache created")
}

func FetchQueuePartitionsFromEtcd(c context.Context, queue string) ([]*PartitionSDRecord, error) {
	ctx, cancelFunc := context.WithTimeout(c, 2*time.Second)
	defer cancelFunc()
	logger.Debug("Fetching queue partitions for queue ", queue)
	r, err := EtcdClient.KV.Get(ctx, fmt.Sprintf("q_%s", queue), clientv3.WithPrefix())
	if err != nil {
		logger.Error("Error getting queue partitions")
		return nil, err
	}
	vals := []*PartitionSDRecord{}
	for _, i := range r.Kvs {
		logger.Debug("Found partition key ", string(i.Key))
		var record PartitionSDRecord
		err := json.Unmarshal(i.Value, &record)
		if err != nil {
			logger.Error("Error when unmarshalling partition record")
			return nil, err
		}
		vals = append(vals, &record)
	}
	return vals, nil
}

func GetQueuePartition(c context.Context, queue string) ([]*PartitionSDRecord, error) {
	// First check cache
	// Get thundering herd lock otherwise
	HerdLock.Lock()
	defer HerdLock.Unlock()
	// Check if
	// Then fetch from etcd
	// Fill cache and update map
	HerdMap[queue] = time.Now()
}
