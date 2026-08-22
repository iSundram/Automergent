package taskstate

import "github.com/iSundram/Automergent/internal/shared"

// TaskStore is the interface implemented by prompt state and consumed by tools.
type TaskStore interface {
	// Plan access
	GetPlan() []shared.TaskSpec
	GetTask(id string) (shared.TaskSpec, bool)
	GetRecord(id string) (*TaskRecord, bool)
	GetAllRecords() []*TaskRecord
	GetNextPendingTask() *TaskRecord
	SetTaskStatus(id string, status shared.TodoStatus, result, err string)

	// Todo
	TodoItems() []shared.TodoItem

	// Context buckets
	CreateBucket(name string) error
	DeleteBucket(name string)
	BucketGet(bucket, key string) (string, bool)
	BucketSet(bucket, key, value string)
	BucketDelete(bucket, key string)
	BucketList(bucket string) []string
	GetAllBuckets() map[string]Bucket

	// High-level context
	GetIntentSet() *shared.IntentSet
	GetInitResults() *shared.InitResults
	GetBuckets() map[string]Bucket
}