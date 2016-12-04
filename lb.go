package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
	redis "gopkg.in/redis.v5"
)

type RedisInstance struct {
	ID     string
	Memory int
}

var (
	TTL            = 60 * time.Second
	redisClients   = make(map[string]*redis.Client)
	instanceUsage  []*RedisInstance
	instanceUsageM = sync.RWMutex{}
)

func printUsageStatus() {
	log.Printf("There are %d redis instances", len(instanceUsage))
	for i, inst := range instanceUsage {
		entries := len(redisClients[inst.ID].Keys("session:*").Val())
		log.Printf("[%d] - ID:%s | Entries:%d", i, inst.ID, entries)
	}
}

// TODO: use Consul KV to make sure the ID has not been used before
func randomID() string {
	c := 6
	b := make([]byte, c)
	crand.Read(b)
	return hex.EncodeToString(b)
}

func parseRedisInfo(info string) map[string]string {
	var infom = make(map[string]string)
	pairs := strings.Split(info, "\r\n")
	for _, pair := range pairs {
		sp := strings.Split(pair, ":")
		if len(sp) != 2 {
			continue
		}
		k, v := sp[0], sp[1]
		infom[k] = v
	}
	return infom
}

func startRedisRefresher(d time.Duration) {
	ticker := time.NewTicker(d)

	go func() {
		for {
			select {
			case <-ticker.C:
				go refreshRedisInstances()
			}
		}
	}()
}

func refreshRedisInstances() {
	cservices, _, err := consulClient.Catalog().Service("redis", "sessia", nil)
	if err != nil {
		log.Fatal(err)
	}
	var localInstanceUsage []*RedisInstance

	for _, service := range cservices {
		client := redis.NewClient(&redis.Options{
			Addr: fmt.Sprintf("%s:%d", service.ServiceAddress, 6379),
		})
		val, err := client.Get("sessia:id").Result()
		if err != nil {
			newID := randomID()
			client.Set("sessia:id", newID, 0)
			redisClients[newID] = client
			val = newID
		} else if _, ok := redisClients[val]; !ok {
			redisClients[val] = client
		}

		// Update instanceUsage
		instance := &RedisInstance{}
		meminfo := parseRedisInfo(client.Info("memory").Val())
		instance.Memory, _ = strconv.Atoi(meminfo["used_memory"])
		instance.ID = val
		localInstanceUsage = append(localInstanceUsage, instance)
	}
	instanceUsageM.Lock()
	instanceUsage = localInstanceUsage
	instanceUsageM.Unlock()
	printUsageStatus()
}

// saveEntry saves []byte entry and returns UUID-instID
func saveEntry(entry []byte) string {
	instanceUsageM.RLock()
	defer instanceUsageM.RUnlock()
	// TODO: support memory based load balancing
	// TODO: do not pick instances marked for destruction
	inst := instanceUsage[rand.Intn(len(instanceUsage))]
	client := redisClients[inst.ID]
	sessionID := uuid.NewV4().String()
	client.Set("session:"+sessionID, entry, TTL)

	return fmt.Sprintf("%s-%s", sessionID, inst.ID)
}

func getEntry(id string) ([]byte, error) {
	instanceUsageM.RLock()
	defer instanceUsageM.RUnlock()
	// TODO: handle bad IDs
	instIndex := strings.LastIndex(id, "-")
	instID := id[instIndex+1:]
	client := redisClients[instID]
	key := id[:instIndex]
	val, err := client.Get("session:" + key).Bytes()
	if err != nil {
		return []byte{}, err
	}
	return val, nil
}
