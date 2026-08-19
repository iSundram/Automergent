package healing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrBucketNotFound = errors.New("bucket not found")
)

type ContextCleanup struct {
	mu           sync.RWMutex
	store        GraphStore
	config       StalenessConfig
	injectedPrompts map[uuid.UUID][]*InjectedPrompt
	contextCache  map[uuid.UUID]*ContextEntry
}

type InjectedPrompt struct {
	ID          uuid.UUID       `json:"id"`
	BucketID    uuid.UUID       `json:"bucket_id"`
	Prompt      string          `json:"prompt"`
	TaskID      uuid.UUID       `json:"task_id,omitempty"`
	AgentID     uuid.UUID       `json:"agent_id,omitempty"`
	InjectedAt  time.Time       `json:"injected_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Used        bool            `json:"used"`
	UsedAt      *time.Time      `json:"used_at,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type ContextEntry struct {
	BucketID    uuid.UUID       `json:"bucket_id"`
	Content     string          `json:"content"`
	ContentHash string          `json:"content_hash"`
	Sources     []string        `json:"sources"`
	CreatedAt   time.Time       `json:"created_at"`
	AccessedAt  time.Time       `json:"accessed_at"`
	AccessCount int             `json:"access_count"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

func NewContextCleanup(store GraphStore, config StalenessConfig) *ContextCleanup {
	return &ContextCleanup{
		store:           store,
		config:          config,
		injectedPrompts: make(map[uuid.UUID][]*InjectedPrompt),
		contextCache:    make(map[uuid.UUID]*ContextEntry),
	}
}

func (cc *ContextCleanup) CleanupStaleContext(ctx context.Context, bucketID uuid.UUID) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	prompts := cc.injectedPrompts[bucketID]
	if len(prompts) == 0 {
		return nil
	}

	now := time.Now()
	validPrompts := make([]*InjectedPrompt, 0, len(prompts))
	removedCount := 0

	for _, prompt := range prompts {
		if prompt.Used && prompt.UsedAt != nil && now.Sub(*prompt.UsedAt) > cc.config.PromptTTL {
			removedCount++
			continue
		}
		if now.After(prompt.ExpiresAt) {
			removedCount++
			continue
		}
		validPrompts = append(validPrompts, prompt)
	}

	cc.injectedPrompts[bucketID] = validPrompts

	if removedCount > 0 {
		if err := cc.persistPromptCleanup(ctx, bucketID, validPrompts); err != nil {
			return fmt.Errorf("persist prompt cleanup: %w", err)
		}
	}

	return cc.DeduplicateContext(ctx, bucketID)
}

func (cc *ContextCleanup) PruneStaleNodes(ctx context.Context) (int64, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	now := time.Time{}
	cutoff := now.Add(-cc.config.NodeTTL)

	nodes, err := cc.store.ListNodes(ctx, "context_bucket", 0, 0)
	if err != nil {
		return 0, fmt.Errorf("list context buckets: %w", err)
	}

	var removed int64
	for _, node := range nodes {
		bucket, ok := node.(map[string]interface{})
		if !ok {
			continue
		}

		createdAtStr, _ := bucket["created_at"].(string)
		createdAt, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			continue
		}

		if createdAt.Before(cutoff) {
			idStr, _ := bucket["id"].(string)
			id, _ := uuid.Parse(idStr)
			if err := cc.store.DeleteNode(ctx, id); err != nil {
				continue
			}
			removed++
			delete(cc.injectedPrompts, id)
			delete(cc.contextCache, id)
		}
	}

	return removed, nil
}

func (cc *ContextCleanup) CleanupInjectedPrompts(ctx context.Context) (int64, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	now := time.Time{}
	var totalRemoved int64

	for bucketID, prompts := range cc.injectedPrompts {
		validPrompts := make([]*InjectedPrompt, 0, len(prompts))
		for _, prompt := range prompts {
			if prompt.Used && prompt.UsedAt != nil && now.Sub(*prompt.UsedAt) > cc.config.PromptTTL {
				totalRemoved++
				continue
			}
			if now.After(prompt.ExpiresAt) {
				totalRemoved++
				continue
			}
			validPrompts = append(validPrompts, prompt)
		}
		cc.injectedPrompts[bucketID] = validPrompts

		if len(validPrompts) != len(prompts) {
			if err := cc.persistPromptCleanup(ctx, bucketID, validPrompts); err != nil {
				return totalRemoved, err
			}
		}
	}

	return totalRemoved, nil
}

func (cc *ContextCleanup) DeduplicateContext(ctx context.Context, bucketID uuid.UUID) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if _, exists := cc.contextCache[bucketID]; !exists {
		return nil
	}

	nodes, err := cc.store.ListNodes(ctx, "memory", 0, 0)
	if err != nil {
		return fmt.Errorf("list memories: %w", err)
	}

	seen := make(map[string]*ContextEntry)
	var duplicates []uuid.UUID

	for _, node := range nodes {
		mem, ok := node.(map[string]interface{})
		if !ok {
			continue
		}

		bucketIDStr, _ := mem["bucket_id"].(string)
		if bucketIDStr != bucketID.String() {
			continue
		}

		content, _ := mem["content"].(string)
		hash := hashContent(content)

		if existing, found := seen[hash]; found {
			existing.AccessCount++
			existing.AccessedAt = time.Now()
			idStr, _ := mem["id"].(string)
			id, _ := uuid.Parse(idStr)
			duplicates = append(duplicates, id)
		} else {
			seen[hash] = &ContextEntry{
				BucketID:    bucketID,
				Content:     content,
				ContentHash: hash,
				CreatedAt:   time.Now(),
				AccessedAt:  time.Now(),
				AccessCount: 1,
			}
		}
	}

	for _, id := range duplicates {
		if err := cc.store.DeleteNode(ctx, id); err != nil {
			continue
		}
	}

	return nil
}

func (cc *ContextCleanup) TrackInjectedPrompt(ctx context.Context, prompt *InjectedPrompt) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if prompt.ID == uuid.Nil {
		prompt.ID = uuid.New()
	}
	if prompt.InjectedAt.IsZero() {
		prompt.InjectedAt = time.Now()
	}
	if prompt.ExpiresAt.IsZero() {
		prompt.ExpiresAt = prompt.InjectedAt.Add(cc.config.PromptTTL)
	}

	cc.injectedPrompts[prompt.BucketID] = append(cc.injectedPrompts[prompt.BucketID], prompt)

	return cc.persistPrompt(ctx, prompt)
}

func (cc *ContextCleanup) MarkPromptUsed(ctx context.Context, promptID uuid.UUID) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	for bucketID, prompts := range cc.injectedPrompts {
		for _, prompt := range prompts {
			if prompt.ID == promptID {
				now := time.Now()
				prompt.Used = true
				prompt.UsedAt = &now
				return cc.persistPromptCleanup(ctx, bucketID, prompts)
			}
		}
	}

	return nil
}

func (cc *ContextCleanup) GetActivePrompts(bucketID uuid.UUID) []*InjectedPrompt {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	prompts := cc.injectedPrompts[bucketID]
	result := make([]*InjectedPrompt, 0, len(prompts))
	now := time.Time{}

	for _, p := range prompts {
		if p.Used && p.UsedAt != nil && now.Sub(*p.UsedAt) > cc.config.PromptTTL {
			continue
		}
		if now.After(p.ExpiresAt) {
			continue
		}
		result = append(result, p)
	}

	return result
}

func (cc *ContextCleanup) GetContextStats(bucketID uuid.UUID) map[string]interface{} {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	prompts := cc.injectedPrompts[bucketID]
	active := 0
	used := 0
	expired := 0
	now := time.Time{}

	for _, p := range prompts {
		if p.Used {
			used++
		}
		if now.After(p.ExpiresAt) {
			expired++
		} else {
			active++
		}
	}

	return map[string]interface{}{
		"total_prompts":   len(prompts),
		"active_prompts":  active,
		"used_prompts":    used,
		"expired_prompts": expired,
		"bucket_id":       bucketID.String(),
	}
}

func (cc *ContextCleanup) persistPrompt(ctx context.Context, prompt *InjectedPrompt) error {
	data, err := json.Marshal(prompt)
	if err != nil {
		return err
	}
	node := map[string]interface{}{
		"id":         prompt.ID.String(),
		"type":       "injected_prompt",
		"data":       data,
		"created_at": prompt.InjectedAt,
		"updated_at": prompt.InjectedAt,
	}
	return cc.store.CreateNode(ctx, node)
}

func (cc *ContextCleanup) persistPromptCleanup(ctx context.Context, bucketID uuid.UUID, prompts []*InjectedPrompt) error {
	for _, prompt := range prompts {
		data, err := json.Marshal(prompt)
		if err != nil {
			continue
		}
		node := map[string]interface{}{
			"id":         prompt.ID.String(),
			"type":       "injected_prompt",
			"data":       data,
			"created_at": prompt.InjectedAt,
			"updated_at": time.Now(),
		}
		_ = cc.store.UpdateNode(ctx, node)
	}
	return nil
}

func (cc *ContextCleanup) LoadPrompts(ctx context.Context, bucketID uuid.UUID) error {
	nodes, err := cc.store.ListNodes(ctx, "injected_prompt", 0, 0)
	if err != nil {
		return err
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	var loaded []*InjectedPrompt
	for _, node := range nodes {
		n, ok := node.(map[string]interface{})
		if !ok {
			continue
		}
		dataBytes, _ := json.Marshal(n["data"])
		var prompt InjectedPrompt
		if err := json.Unmarshal(dataBytes, &prompt); err != nil {
			continue
		}
		if prompt.BucketID == bucketID {
			loaded = append(loaded, &prompt)
		}
	}
	cc.injectedPrompts[bucketID] = loaded
	return nil
}

func hashContent(content string) string {
	h := 0
	for i := 0; i < len(content); i++ {
		h = 31*h + int(content[i])
	}
	return fmt.Sprintf("%x", h)
}

func (cc *ContextCleanup) ClearBucketCache(bucketID uuid.UUID) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	delete(cc.injectedPrompts, bucketID)
	delete(cc.contextCache, bucketID)
}