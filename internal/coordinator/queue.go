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
