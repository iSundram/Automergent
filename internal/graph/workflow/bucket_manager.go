package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ContextBucketManager struct {
	store StoreInterface
	mu    sync.RWMutex
}

func NewContextBucketManager(store StoreInterface) *ContextBucketManager {
	return &ContextBucketManager{
		store: store,
	}
}

func (m *ContextBucketManager) CreateBucket(ctx context.Context, taskID uuid.UUID, bucketType ContextBucketType, name, description, owner string, sharePolicy SharePolicy) (*ContextBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	shortID, err := m.nextShortIDUnsafe(ctx)
	if err != nil {
		return nil, err
	}
	bucket := &ContextBucket{
		ShortID:     shortID,
		WorkflowID:  taskID,
		Name:        name,
		Type:        bucketType,
		Description: description,
		SharePolicy: sharePolicy,
		Owner:       owner,
		Data:        json.RawMessage("{}"),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	node, err := bucket.toNode()
	if err != nil {
		return nil, fmt.Errorf("create bucket node: %w", err)
	}

	if err := m.store.CreateNode(ctx, node); err != nil {
		return nil, fmt.Errorf("persist bucket: %w", err)
	}

	bucket.ID = node.ID
	return bucket, nil
}

func (m *ContextBucketManager) CreateBucketForTodo(ctx context.Context, todoID uuid.UUID, name, description, owner string, sharePolicy SharePolicy) (*ContextBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	shortID, err := m.nextShortIDUnsafe(ctx)
	if err != nil {
		return nil, err
	}
	bucket := &ContextBucket{
		ShortID:     shortID,
		TodoID:      todoID,
		Name:        name,
		Type:        ContextBucketTypeTodo,
		Description: description,
		SharePolicy: sharePolicy,
		Owner:       owner,
		Data:        json.RawMessage("{}"),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	node, err := bucket.toNode()
	if err != nil {
		return nil, fmt.Errorf("create bucket node: %w", err)
	}

	if err := m.store.CreateNode(ctx, node); err != nil {
		return nil, fmt.Errorf("persist bucket: %w", err)
	}

	bucket.ID = node.ID
	return bucket, nil
}

func (m *ContextBucketManager) GetBucket(ctx context.Context, bucketID uuid.UUID) (*ContextBucket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	node, err := m.store.GetNode(ctx, bucketID)
	if err != nil {
		return nil, err
	}

	if node.Type != NodeTypeContextBucket {
		return nil, ErrBucketNotFound
	}

	var bucket ContextBucket
	if err := json.Unmarshal(node.Data, &bucket); err != nil {
		return nil, fmt.Errorf("unmarshal bucket: %w", err)
	}
	bucket.ID = node.ID
	return &bucket, nil
}

func (m *ContextBucketManager) GetBucketByTodo(ctx context.Context, todoID uuid.UUID) (*ContextBucket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes, err := m.store.ListNodes(ctx, NodeTypeContextBucket, 100, 0)
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		var bucket ContextBucket
		if err := json.Unmarshal(node.Data, &bucket); err != nil {
			continue
		}
		if bucket.TodoID == todoID {
			bucket.ID = node.ID
			return &bucket, nil
		}
	}
	return nil, ErrBucketNotFound
}

func (m *ContextBucketManager) ShareContext(ctx context.Context, fromBucketID, toBucketID uuid.UUID, policy SharePolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fromBucket, err := m.getBucketUnsafe(ctx, fromBucketID)
	if err != nil {
		return err
	}

	toBucket, err := m.getBucketUnsafe(ctx, toBucketID)
	if err != nil {
		return err
	}

	fromData := make(map[string]interface{})
	if err := json.Unmarshal(fromBucket.Data, &fromData); err != nil {
		return fmt.Errorf("unmarshal from bucket: %w", err)
	}

	toData := make(map[string]interface{})
	if err := json.Unmarshal(toBucket.Data, &toData); err != nil {
		return fmt.Errorf("unmarshal to bucket: %w", err)
	}

	switch policy {
	case SharePolicyFull:
		for k, v := range fromData {
			toData[k] = v
		}
	case SharePolicySummary:
		summary := map[string]interface{}{
			"summary":     fromBucket.Description,
			"bucket_id":   fromBucket.ID.String(),
			"bucket_type": fromBucket.Type,
			"owner":       fromBucket.Owner,
			"updated_at":  fromBucket.UpdatedAt,
		}
		for k, v := range summary {
			toData[k] = v
		}
	case SharePolicyPartial:
		for _, key := range fromBucket.ShareKeys {
			if val, ok := fromData[key]; ok {
				toData[key] = val
			}
		}
	case SharePolicyInjected:
	case SharePolicyNone:
		return nil
	default:
		return ErrInvalidSharePolicy
	}

	toBucket.Data, err = json.Marshal(toData)
	if err != nil {
		return fmt.Errorf("marshal to bucket data: %w", err)
	}
	toBucket.UpdatedAt = time.Now()
	toBucket.SharePolicy = policy

	node, err := toBucket.toNode()
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}

	return m.store.UpdateNode(ctx, node)
}

func (m *ContextBucketManager) InjectMessage(ctx context.Context, toBucketID uuid.UUID, message *InjectedMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bucket, err := m.getBucketUnsafe(ctx, toBucketID)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bucket.Data, &data); err != nil {
		data = make(map[string]interface{})
	}

	injectedMessages, ok := data["injected_messages"].([]interface{})
	if !ok {
		injectedMessages = []interface{}{}
	}

	msgData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	var msgMap map[string]interface{}
	if err := json.Unmarshal(msgData, &msgMap); err != nil {
		return fmt.Errorf("unmarshal message: %w", err)
	}

	injectedMessages = append(injectedMessages, msgMap)
	data["injected_messages"] = injectedMessages

	bucket.Data, err = json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal bucket data: %w", err)
	}
	bucket.UpdatedAt = time.Now()

	node, err := bucket.toNode()
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}

	return m.store.UpdateNode(ctx, node)
}

func (m *ContextBucketManager) GetBucketSummary(ctx context.Context, bucketID uuid.UUID) (*BucketSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bucket, err := m.getBucketUnsafe(ctx, bucketID)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bucket.Data, &data); err != nil {
		data = make(map[string]interface{})
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}

	return &BucketSummary{
		BucketID:    bucket.ID,
		ShortID:     bucket.ShortID,
		Name:        bucket.Name,
		Type:        bucket.Type,
		ItemCount:   len(data),
		LastUpdated: bucket.UpdatedAt,
		SharePolicy: bucket.SharePolicy,
		Keys:        keys,
	}, nil
}

// ResolveBucketID accepts either the durable UUID or the compact cN handle
// shown in prompts and the TUI.
func (m *ContextBucketManager) ResolveBucketID(ctx context.Context, value string) (uuid.UUID, error) {
	if id, err := uuid.Parse(value); err == nil {
		return id, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	nodes, err := m.store.ListNodes(ctx, NodeTypeContextBucket, 10000, 0)
	if err != nil {
		return uuid.Nil, err
	}
	for _, node := range nodes {
		var bucket ContextBucket
		if json.Unmarshal(node.Data, &bucket) == nil && bucket.ShortID == value {
			return node.ID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("context bucket %q not found", value)
}

func (m *ContextBucketManager) SetBucketOwner(ctx context.Context, bucketID uuid.UUID, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket, err := m.getBucketUnsafe(ctx, bucketID)
	if err != nil {
		return err
	}
	bucket.Owner = owner
	bucket.UpdatedAt = time.Now()
	node, err := bucket.toNode()
	if err != nil {
		return err
	}
	return m.store.UpdateNode(ctx, node)
}

func (m *ContextBucketManager) GetAgentBucket(ctx context.Context, taskID uuid.UUID, owner string) (*ContextBucket, error) {
	m.mu.RLock()
	nodes, err := m.store.ListNodes(ctx, NodeTypeContextBucket, 1000, 0)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		var bucket ContextBucket
		if json.Unmarshal(node.Data, &bucket) == nil && bucket.WorkflowID == taskID && bucket.Type == ContextBucketTypeAgent && bucket.Owner == owner {
			bucket.ID = node.ID
			return &bucket, nil
		}
	}
	return nil, ErrBucketNotFound
}

func (m *ContextBucketManager) nextShortIDUnsafe(ctx context.Context) (string, error) {
	nodes, err := m.store.ListNodes(ctx, NodeTypeContextBucket, 10000, 0)
	if err != nil {
		return "", fmt.Errorf("list context buckets: %w", err)
	}
	maxID := 0
	for _, node := range nodes {
		var bucket ContextBucket
		if json.Unmarshal(node.Data, &bucket) != nil || len(bucket.ShortID) < 2 || bucket.ShortID[0] != 'c' {
			continue
		}
		if n, err := strconv.Atoi(bucket.ShortID[1:]); err == nil && n > maxID {
			maxID = n
		}
	}
	return fmt.Sprintf("c%d", maxID+1), nil
}

func (m *ContextBucketManager) ListBuckets(ctx context.Context, taskID uuid.UUID) ([]*ContextBucket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes, err := m.store.ListNodes(ctx, NodeTypeContextBucket, 1000, 0)
	if err != nil {
		return nil, err
	}

	var buckets []*ContextBucket
	for _, node := range nodes {
		var bucket ContextBucket
		if err := json.Unmarshal(node.Data, &bucket); err != nil {
			continue
		}
		if bucket.WorkflowID == taskID || bucket.TodoID != uuid.Nil {
			bucket.ID = node.ID
			buckets = append(buckets, &bucket)
		}
	}
	return buckets, nil
}

func (m *ContextBucketManager) ResumeCoderContext(ctx context.Context, taskID uuid.UUID) (*ContextBucket, error) {
	m.mu.RLock()
	nodes, err := m.store.ListNodes(ctx, NodeTypeContextBucket, 1000, 0)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	var coderBucket *ContextBucket
	for _, node := range nodes {
		var bucket ContextBucket
		if err := json.Unmarshal(node.Data, &bucket); err != nil {
			continue
		}
		if bucket.WorkflowID == taskID && bucket.Type == ContextBucketTypeAgent && bucket.Owner == "coder" {
			bucket.ID = node.ID
			coderBucket = &bucket
			break
		}
	}

	if coderBucket == nil {
		return m.CreateBucket(ctx, taskID, ContextBucketTypeAgent, "coder", "Coder agent context bucket", "coder", SharePolicyFull)
	}

	return coderBucket, nil
}

func (m *ContextBucketManager) UpdateBucketData(ctx context.Context, bucketID uuid.UUID, key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bucket, err := m.getBucketUnsafe(ctx, bucketID)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bucket.Data, &data); err != nil {
		data = make(map[string]interface{})
	}

	data[key] = value
	bucket.Data, err = json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal bucket data: %w", err)
	}
	bucket.UpdatedAt = time.Now()

	node, err := bucket.toNode()
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}

	return m.store.UpdateNode(ctx, node)
}

func (m *ContextBucketManager) GetBucketData(ctx context.Context, bucketID uuid.UUID, key string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bucket, err := m.getBucketUnsafe(ctx, bucketID)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bucket.Data, &data); err != nil {
		return nil, nil
	}

	return data[key], nil
}

func (m *ContextBucketManager) DeleteBucket(ctx context.Context, bucketID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.DeleteNode(ctx, bucketID)
}

func (m *ContextBucketManager) getBucketUnsafe(ctx context.Context, bucketID uuid.UUID) (*ContextBucket, error) {
	node, err := m.store.GetNode(ctx, bucketID)
	if err != nil {
		return nil, err
	}
	if node.Type != NodeTypeContextBucket {
		return nil, ErrBucketNotFound
	}
	var bucket ContextBucket
	if err := json.Unmarshal(node.Data, &bucket); err != nil {
		return nil, fmt.Errorf("unmarshal bucket: %w", err)
	}
	bucket.ID = node.ID
	return &bucket, nil
}

func (b *ContextBucket) toNode() (*Node, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	id := b.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return &Node{
		ID:        id,
		Type:      NodeTypeContextBucket,
		Data:      data,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}, nil
}
