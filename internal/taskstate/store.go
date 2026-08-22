package taskstate

import (
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/shared"
)

// TaskRecord holds a task with its current status and metadata.
type TaskRecord struct {
	TaskSpec   shared.TaskSpec
	Status     shared.TodoStatus
	Assignee   string
	Result     string
	Error      string
	StartedAt  time.Time
	CompletedAt time.Time
}

// Bucket is a simple key-value store for context sharing.
type Bucket map[string]string

// Store holds the task plan, execution state, and context buckets for a request.
type Store struct {
	mu sync.RWMutex

	// Task plan (created by LLM task planner)
	Plan     []shared.TaskSpec
	Records  map[string]*TaskRecord // keyed by task ID

	// Context buckets: named stores for sharing context between agents/tasks
	Buckets map[string]Bucket

	// High-level intents and init results for context tools
	IntentSet   *shared.IntentSet
	InitResults *shared.InitResults

	// Todo items extracted from plan (for todo-style workflow)
	todoItems []shared.TodoItem
}

// NewStore creates an empty store.
func NewStore() *Store {
	return &Store{
		Plan:       nil,
		Records:    make(map[string]*TaskRecord),
		Buckets:    make(map[string]Bucket),
		IntentSet:  nil,
		InitResults: nil,
		todoItems:  nil,
	}
}

// SetPlan installs the task plan and initializes records/todos.
func (s *Store) SetPlan(plan []shared.TaskSpec, todos []shared.TodoItem) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Plan = plan
	s.Records = make(map[string]*TaskRecord)
	for _, t := range plan {
		s.Records[t.ID] = &TaskRecord{TaskSpec: t, Status: shared.TodoStatusPending}
	}
	s.todoItems = todos
}

// SetIntentAndInit stores high-level context for tools.
func (s *Store) SetIntentAndInit(intent *shared.IntentSet, init *shared.InitResults) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IntentSet = intent
	s.InitResults = init
}

// GetPlan returns a copy of the current plan.
func (s *Store) GetPlan() []shared.TaskSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()
	plan := make([]shared.TaskSpec, len(s.Plan))
	copy(plan, s.Plan)
	return plan
}

// GetTask returns a task spec by ID.
func (s *Store) GetTask(id string) (shared.TaskSpec, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Plan == nil {
		return shared.TaskSpec{}, false
	}
	for _, t := range s.Plan {
		if t.ID == id {
			return t, true
		}
	}
	return shared.TaskSpec{}, false
}

// GetRecord returns the execution record for a task.
func (s *Store) GetRecord(id string) (*TaskRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.Records[id]
	return r, ok
}

// SetTaskStatus updates the status of a task.
func (s *Store) SetTaskStatus(id string, status shared.TodoStatus, result, err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.Records[id]; ok {
		r.Status = status
		r.Result = result
		r.Error = err
		now := time.Now()
		if status == shared.TodoStatusInProgress && r.StartedAt.IsZero() {
			r.StartedAt = now
		}
		if status == shared.TodoStatusCompleted || status == shared.TodoStatusBlocked {
			r.CompletedAt = now
		}
	}
}

// GetNextPendingTask returns the next task ready to execute (deps met, pending).
func (s *Store) GetNextPendingTask() *TaskRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	completed := make(map[string]bool)
	for id, r := range s.Records {
		if r.Status == shared.TodoStatusCompleted {
			completed[id] = true
		}
	}

	for _, t := range s.Plan {
		if r, ok := s.Records[t.ID]; ok && r.Status == shared.TodoStatusPending {
			depsMet := true
			for _, dep := range t.Dependencies {
				if !completed[dep] {
					depsMet = false
					break
				}
			}
			if depsMet {
				return r
			}
		}
	}
	return nil
}

// GetAllRecords returns all task records.
func (s *Store) GetAllRecords() []*TaskRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*TaskRecord, 0, len(s.Records))
	for _, r := range s.Records {
		out = append(out, r)
	}
	return out
}

// TodoItems returns current todo items.
func (s *Store) TodoItems() []shared.TodoItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]shared.TodoItem, len(s.todoItems))
	copy(items, s.todoItems)
	return items
}

// Bucket operations

// CreateBucket creates a new empty bucket.
func (s *Store) CreateBucket(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Buckets[name]; ok {
		return nil // idempotent
	}
	s.Buckets[name] = make(Bucket)
	return nil
}

// DeleteBucket deletes a bucket.
func (s *Store) DeleteBucket(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Buckets, name)
}

// BucketGet returns a value from a bucket.
func (s *Store) BucketGet(bucket, key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.Buckets[bucket]
	if !ok {
		return "", false
	}
	v, ok := b[key]
	return v, ok
}

// BucketSet sets a key in a bucket.
func (s *Store) BucketSet(bucket, key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Buckets[bucket] == nil {
		s.Buckets[bucket] = make(Bucket)
	}
	s.Buckets[bucket][key] = value
}

// BucketDelete deletes a key from a bucket.
func (s *Store) BucketDelete(bucket, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.Buckets[bucket]; ok {
		delete(b, key)
	}
}

// BucketList lists all keys in a bucket.
func (s *Store) BucketList(bucket string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.Buckets[bucket]
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	return keys
}

// GetIntentSet returns the current intent set.
func (s *Store) GetIntentSet() *shared.IntentSet {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.IntentSet
}

// GetInitResults returns the init results.
func (s *Store) GetInitResults() *shared.InitResults {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.InitResults
}

// GetAllBuckets returns all buckets for listing (alias for GetBuckets).
func (s *Store) GetAllBuckets() map[string]Bucket {
	return s.GetBuckets()
}

// GetBuckets returns all buckets.
func (s *Store) GetBuckets() map[string]Bucket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Bucket, len(s.Buckets))
	for k, v := range s.Buckets {
		copy := make(Bucket, len(v))
		for kk, vv := range v {
			copy[kk] = vv
		}
		out[k] = copy
	}
	return out
}