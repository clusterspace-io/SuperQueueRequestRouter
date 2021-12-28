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
	Address    string
	IsDraining bool
}

type PartitionStoredRecord []PartitionSDRecord

func TryEtcdConnect() {
	logger.Debug("Starting etcd based service discovery")
	CheckFlags()
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

func FetchQueuePartitionsFromEtcd(c context.Context, queue string) (*PartitionStoredRecord, error) {
	ctx, cancelFunc := context.WithTimeout(c, 2*time.Second)
	defer cancelFunc()
	logger.Debug("Fetching queue partitions for queue ", queue)
	r, err := EtcdClient.KV.Get(ctx, fmt.Sprintf("q_%s", queue), clientv3.WithPrefix())
	if err != nil {
		logger.Error("Error getting queue partitions")
		return nil, err
	}
	vals := PartitionStoredRecord{}
	for _, i := range r.Kvs {
		logger.Debug("Found partition key ", string(i.Key))
		var record PartitionSDRecord
		err := json.Unmarshal(i.Value, &record)
		if err != nil {
			logger.Error("Error when unmarshalling partition record")
			return nil, err
		}
		vals = append(vals, record)
	}
	return &vals, nil
}

func GetQueuePartitions(c context.Context, queue string) (*PartitionStoredRecord, error) {
	var record PartitionStoredRecord
	var ok bool
	// First check cache
	val, found := PartitionCache.Get(queue)
	if found {
		logger.Debug("Found partition in cache for queue ", queue)
		record, ok = val.(PartitionStoredRecord)
		if !ok {
			logger.Error("Error converting cache to byte array")
			return nil, fmt.Errorf("failed to convert cache value to byte array")
		}
		// buf := bytes.NewBuffer(b)
		// decoder := gob.NewDecoder(buf)
		// err := decoder.Decode(&record)
		// if err != nil {
		// 	logger.Error("Error decoding buffer from cache")
		// 	return nil, err
		// }
		return &record, nil
	}
	logger.Debug("Did not find partition in cache for queue ", queue, ", fetching from etcd")
	// Get thundering herd lock otherwise
	HerdLock.Lock()
	defer HerdLock.Unlock()
	// Check if we recently thundering herd blocked or do not have it
	if val, exists := HerdMap[queue]; exists && val.After(time.Now().Add(-10*time.Second)) {
		// Should be in cache now, run again
		return GetQueuePartitions(c, queue)
	}
	// Otherwise we need to go get from etcd
	r, err := FetchQueuePartitionsFromEtcd(c, queue)
	if err != nil {
		logger.Error("Error fetching queue partitions from etcd when backfilling cache")
		return nil, err
	}
	// Fill cache and update map
	// var b bytes.Buffer
	// encoder := gob.NewEncoder(&b)
	// err = encoder.Encode(*r)
	// if err != nil {
	// 	logger.Error("Error encoding partition record when backfilling cache")
	// 	return nil, err
	// }
	// bb, err := ioutil.ReadAll(&b)
	// if err != nil {
	// 	logger.Error("Error reading bytes when backfilling cache")
	// 	return nil, err
	// }
	PartitionCache.SetWithTTL(queue, *r, 64, 10*time.Second)
	HerdMap[queue] = time.Now()
	return r, nil
}
