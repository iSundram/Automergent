package coordinator

import (
	"sort"
	"testing"
	"time"
)

func TestTaskQueue_PushPop(t *testing.T) {
	q := newTaskQueue()

	t1 := &Task{ID: "t1", Priority: PriorityNormal, CreatedAt: time.Now()}
	t2 := &Task{ID: "t2", Priority: PriorityHigh, CreatedAt: time.Now()}
	t3 := &Task{ID: "t3", Priority: PriorityLow, CreatedAt: time.Now()}

	q.Push(t1)
	q.Push(t2)
	q.Push(t3)

	if q.Len() != 3 {
		t.Fatalf("expected 3 tasks, got %d", q.Len())
	}

	// Pop should return highest priority first.
	got := q.Pop()
	if got.ID != "t2" {
		t.Errorf("expected t2 (high priority), got %s", got.ID)
	}

	got = q.Pop()
	if got.ID != "t1" {
		t.Errorf("expected t1 (normal priority), got %s", got.ID)
	}

	got = q.Pop()
	if got.ID != "t3" {
		t.Errorf("expected t3 (low priority), got %s", got.ID)
	}

	if q.Pop() != nil {
		t.Error("expected nil from empty queue")
	}
}

func TestTaskQueue_Peek(t *testing.T) {
	q := newTaskQueue()

	t1 := &Task{ID: "t1", Priority: PriorityNormal, CreatedAt: time.Now()}
	t2 := &Task{ID: "t2", Priority: PriorityHigh, CreatedAt: time.Now()}

	q.Push(t1)
	q.Push(t2)

	peeked := q.Peek()
	if peeked == nil || peeked.ID != "t2" {
		t.Errorf("expected peek to return t2, got %v", peeked)
	}

	// Peek should not remove.
	if q.Len() != 2 {
		t.Errorf("expected 2 tasks after peek, got %d", q.Len())
	}
}

func TestTaskQueue_Remove(t *testing.T) {
	q := newTaskQueue()

	t1 := &Task{ID: "t1", Priority: PriorityNormal, CreatedAt: time.Now()}
	t2 := &Task{ID: "t2", Priority: PriorityHigh, CreatedAt: time.Now()}

	q.Push(t1)
	q.Push(t2)

	if !q.Remove("t1") {
		t.Error("expected Remove to return true")
	}
	if q.Len() != 1 {
		t.Errorf("expected 1 task after remove, got %d", q.Len())
	}
	if q.Remove("nonexistent") {
		t.Error("expected Remove to return false for nonexistent task")
	}
}

func TestTaskQueue_FIFO_SamePriority(t *testing.T) {
	q := newTaskQueue()

	now := time.Now()
	t1 := &Task{ID: "t1", Priority: PriorityNormal, CreatedAt: now}
	t2 := &Task{ID: "t2", Priority: PriorityNormal, CreatedAt: now.Add(time.Millisecond)}
	t3 := &Task{ID: "t3", Priority: PriorityNormal, CreatedAt: now.Add(2 * time.Millisecond)}

	q.Push(t1)
	q.Push(t2)
	q.Push(t3)

	// Same priority should be FIFO by creation time.
	got := q.Pop()
	if got.ID != "t1" {
		t.Errorf("expected t1 (earliest), got %s", got.ID)
	}
	got = q.Pop()
	if got.ID != "t2" {
		t.Errorf("expected t2, got %s", got.ID)
	}
}

func TestTaskQueue_Clear(t *testing.T) {
	q := newTaskQueue()
	q.Push(&Task{ID: "t1", Priority: PriorityNormal, CreatedAt: time.Now()})
	q.Push(&Task{ID: "t2", Priority: PriorityHigh, CreatedAt: time.Now()})

	q.Clear()
	if q.Len() != 0 {
		t.Errorf("expected 0 after clear, got %d", q.Len())
	}
}

func TestRoleQueues_PushPop(t *testing.T) {
	rq := newRoleQueues()

	t1 := &Task{ID: "t1", Role: RoleCoder, Priority: PriorityNormal, CreatedAt: time.Now()}
	t2 := &Task{ID: "t2", Role: RoleResearcher, Priority: PriorityHigh, CreatedAt: time.Now()}
	t3 := &Task{ID: "t3", Role: "", Priority: PriorityLow, CreatedAt: time.Now()} // "any" queue

	rq.Push(t1)
	rq.Push(t2)
	rq.Push(t3)

	// Pop for coder should get t1.
	got := rq.Pop(RoleCoder)
	if got == nil || got.ID != "t1" {
		t.Errorf("expected t1 for coder, got %v", got)
	}

	// Pop for researcher should get t2.
	got = rq.Pop(RoleResearcher)
	if got == nil || got.ID != "t2" {
		t.Errorf("expected t2 for researcher, got %v", got)
	}

	// Pop for tester should get t3 from "any" queue.
	got = rq.Pop(RoleTester)
	if got == nil || got.ID != "t3" {
		t.Errorf("expected t3 (any queue) for tester, got %v", got)
	}

	if rq.Len() != 0 {
		t.Errorf("expected 0 tasks, got %d", rq.Len())
	}
}

func TestRoleQueues_RoleLen(t *testing.T) {
	rq := newRoleQueues()

	rq.Push(&Task{ID: "t1", Role: RoleCoder, Priority: PriorityNormal, CreatedAt: time.Now()})
	rq.Push(&Task{ID: "t2", Role: RoleCoder, Priority: PriorityHigh, CreatedAt: time.Now()})
	rq.Push(&Task{ID: "t3", Role: RoleResearcher, Priority: PriorityNormal, CreatedAt: time.Now()})

	if rq.RoleLen(RoleCoder) != 2 {
		t.Errorf("expected 2 coder tasks, got %d", rq.RoleLen(RoleCoder))
	}
	if rq.RoleLen(RoleResearcher) != 1 {
		t.Errorf("expected 1 researcher task, got %d", rq.RoleLen(RoleResearcher))
	}
	if rq.RoleLen(RoleTester) != 0 {
		t.Errorf("expected 0 tester tasks, got %d", rq.RoleLen(RoleTester))
	}
}

func TestTaskHeap_Ordering(t *testing.T) {
	now := time.Now()
	h := taskHeap{
		{ID: "low", Priority: PriorityLow, CreatedAt: now},
		{ID: "critical", Priority: PriorityCritical, CreatedAt: now},
		{ID: "normal", Priority: PriorityNormal, CreatedAt: now},
		{ID: "high", Priority: PriorityHigh, CreatedAt: now},
	}

	sort.Sort(h)

	expected := []string{"critical", "high", "normal", "low"}
	for i, id := range expected {
		if h[i].ID != id {
			t.Errorf("position %d: expected %s, got %s", i, id, h[i].ID)
		}
	}
}
