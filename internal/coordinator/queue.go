package coordinator

import (
	"container/heap"
	"sync"
)

// taskQueue is a thread-safe priority queue for tasks.
type taskQueue struct {
	mu    sync.Mutex
	items taskHeap
}

func newTaskQueue() *taskQueue {
	q := &taskQueue{
		items: make(taskHeap, 0),
	}
	heap.Init(&q.items)
	return q
}

// Push adds a task to the queue.
func (q *taskQueue) Push(task *Task) {
	q.mu.Lock()
	defer q.mu.Unlock()
	heap.Push(&q.items, task)
}

// Pop removes and returns the highest priority task.
func (q *taskQueue) Pop() *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}

	return heap.Pop(&q.items).(*Task)
}

// Peek returns the highest priority task without removing it.
func (q *taskQueue) Peek() *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}

	return q.items[0]
}

// Len returns the number of tasks in the queue.
func (q *taskQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Remove removes a task by ID from the queue.
func (q *taskQueue) Remove(taskID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, task := range q.items {
		if task.ID == taskID {
			heap.Remove(&q.items, i)
			return true
		}
	}
	return false
}

// Clear removes all tasks from the queue.
func (q *taskQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = make(taskHeap, 0)
}

// taskHeap implements heap.Interface for tasks.
type taskHeap []*Task

func (h taskHeap) Len() int { return len(h) }

func (h taskHeap) Less(i, j int) bool {
	// Higher priority first
	if h[i].Priority != h[j].Priority {
		return h[i].Priority > h[j].Priority
	}
	// Earlier creation time first (FIFO for same priority)
	return h[i].CreatedAt.Before(h[j].CreatedAt)
}

func (h taskHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *taskHeap) Push(x any) {
	*h = append(*h, x.(*Task))
}

func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	task := old[n-1]
	old[n-1] = nil // avoid memory leak
	*h = old[0 : n-1]
	return task
}

// roleQueues manages per-role task queues.
type roleQueues struct {
	mu     sync.RWMutex
	queues map[AgentRole]*taskQueue
	any    *taskQueue // for tasks with Role=""
}

func newRoleQueues() *roleQueues {
	return &roleQueues{
		queues: make(map[AgentRole]*taskQueue),
		any:    newTaskQueue(),
	}
}

func (rq *roleQueues) getQueue(role AgentRole) *taskQueue {
	rq.mu.RLock()
	q, ok := rq.queues[role]
	rq.mu.RUnlock()
	if ok {
		return q
	}

	rq.mu.Lock()
	defer rq.mu.Unlock()
	// Double-check
	if q, ok = rq.queues[role]; ok {
		return q
	}
	q = newTaskQueue()
	rq.queues[role] = q
	return q
}

func (rq *roleQueues) getAnyQueue() *taskQueue {
	return rq.any
}

// Push adds a task to the appropriate queue based on its role.
func (rq *roleQueues) Push(task *Task) {
	if task.Role == "" {
		rq.any.Push(task)
	} else {
		rq.getQueue(task.Role).Push(task)
	}
}

// Pop removes and returns the highest priority task for the given role.
// If role is provided, it checks both the role-specific queue and the "any" queue.
func (rq *roleQueues) Pop(role AgentRole) *Task {
	// First try role-specific queue
	if role != "" {
		q := rq.getQueue(role)
		if task := q.Pop(); task != nil {
			return task
		}
	}
	// Then try the "any" queue
	return rq.any.Pop()
}

// Peek returns the highest priority task for the given role without removing it.
func (rq *roleQueues) Peek(role AgentRole) *Task {
	if role != "" {
		q := rq.getQueue(role)
		if task := q.Peek(); task != nil {
			return task
		}
	}
	return rq.any.Peek()
}

// Len returns the total number of tasks across all queues.
func (rq *roleQueues) Len() int {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	total := rq.any.Len()
	for _, q := range rq.queues {
		total += q.Len()
	}
	return total
}

// Remove removes a task by ID from any queue.
func (rq *roleQueues) Remove(taskID string) bool {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	if rq.any.Remove(taskID) {
		return true
	}
	for _, q := range rq.queues {
		if q.Remove(taskID) {
			return true
		}
	}
	return false
}

// Clear removes all tasks from all queues.
func (rq *roleQueues) Clear() {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	rq.any.Clear()
	for _, q := range rq.queues {
		q.Clear()
	}
}

// RoleLen returns the number of tasks in a specific role's queue.
func (rq *roleQueues) RoleLen(role AgentRole) int {
	if role == "" {
		return rq.any.Len()
	}
	rq.mu.RLock()
	q, ok := rq.queues[role]
	rq.mu.RUnlock()
	if !ok {
		return 0
	}
	return q.Len()
}

// AnyLen returns the number of tasks in the "any" queue.
func (rq *roleQueues) AnyLen() int {
	return rq.any.Len()
}
