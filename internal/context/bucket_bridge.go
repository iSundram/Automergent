package context

import (
	"fmt"
	"sync"

	"github.com/iSundram/Automergent/internal/taskstate"
)

// BucketBridge provides cross-agent context sharing via the taskstate bucket system.
type BucketBridge struct {
	mu    sync.RWMutex
	store *taskstate.Store
}

// NewBucketBridge creates a new bridge to the taskstate bucket system.
func NewBucketBridge(store *taskstate.Store) *BucketBridge {
	return &BucketBridge{store: store}
}

// CreateBucket creates a new named bucket for context sharing.
func (bb *BucketBridge) CreateBucket(name string) error {
	if bb.store == nil {
		return fmt.Errorf("taskstate store not initialized")
	}
	return bb.store.CreateBucket(name)
}

// Get retrieves a value from a bucket.
func (bb *BucketBridge) Get(bucket, key string) (string, bool) {
	if bb.store == nil {
		return "", false
	}
	return bb.store.BucketGet(bucket, key)
}

// Set stores a value in a bucket.
func (bb *BucketBridge) Set(bucket, key, value string) {
	if bb.store == nil {
		return
	}
	bb.store.BucketSet(bucket, key, value)
}

// Delete removes a key from a bucket.
func (bb *BucketBridge) Delete(bucket, key string) {
	if bb.store == nil {
		return
	}
	bb.store.BucketDelete(bucket, key)
}

// ListKeys returns all keys in a bucket.
func (bb *BucketBridge) ListKeys(bucket string) []string {
	if bb.store == nil {
		return nil
	}
	return bb.store.BucketList(bucket)
}

// ListBuckets returns all bucket names.
func (bb *BucketBridge) ListBuckets() []string {
	if bb.store == nil {
		return nil
	}
	buckets := bb.store.GetAllBuckets()
	names := make([]string, 0, len(buckets))
	for name := range buckets {
		names = append(names, name)
	}
	return names
}

// GetAll returns all key-value pairs in a bucket.
func (bb *BucketBridge) GetAll(bucket string) map[string]string {
	if bb.store == nil {
		return nil
	}
	b := bb.store.GetBuckets()[bucket]
	result := make(map[string]string, len(b))
	for k, v := range b {
		result[k] = v
	}
	return result
}

// StoreResult stores an agent's result in a bucket for other agents to consume.
func (bb *BucketBridge) StoreResult(agentID, key, result string) {
	bb.Set("agent_results", fmt.Sprintf("%s:%s", agentID, key), result)
}

// GetResult retrieves an agent's result from the shared bucket.
func (bb *BucketBridge) GetResult(agentID, key string) (string, bool) {
	return bb.Get("agent_results", fmt.Sprintf("%s:%s", agentID, key))
}

// StoreFinding stores a research finding for synthesis.
func (bb *BucketBridge) StoreFinding(source, topic, finding string) {
	bb.Set("findings", fmt.Sprintf("%s:%s", source, topic), finding)
}

// GetFindings retrieves all findings for a topic.
func (bb *BucketBridge) GetFindings(topic string) map[string]string {
	keys := bb.ListKeys("findings")
	result := make(map[string]string)
	prefix := topic + ":"
	for _, k := range keys {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			if v, ok := bb.Get("findings", k); ok {
				result[k] = v
			}
		}
	}
	return result
}

// StoreDecision stores an architectural decision for reference.
func (bb *BucketBridge) StoreDecision(decision, rationale, decidedBy string) {
	bb.Set("decisions", decision, fmt.Sprintf("Rationale: %s | By: %s", rationale, decidedBy))
}

// GetDecisions retrieves all recorded decisions.
func (bb *BucketBridge) GetDecisions() map[string]string {
	return bb.GetAll("decisions")
}
