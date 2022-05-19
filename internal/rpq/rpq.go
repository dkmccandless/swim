// Package rpq implements a recurrent priority queue: a priority queue whose
// elements can be returned multiple times, up to a specified quota,
// prioritized according to how many times they have already been returned.
package rpq

import "container/heap"

// A Queue is a recurrent priority queue of values of type V, optionally
// indexed by keys of type K. Keys that are not the zero value of type K are
// unique within a Queue.
type Queue[K comparable, V any] struct {
	pq    priorityQueue[K, V]
	quota func() int
}

// An item is a key-value pair with an associated return count.
type item[K comparable, V any] struct {
	key   K
	value V
	count int
}

// New initializes a new Queue. Quota describes the minimum number of times an
// item will be returned by Pop or PopN before it is removed from the Queue.
func New[K comparable, V any](quota func() int) *Queue[K, V] {
	return &Queue[K, V]{
		pq:    makePriorityQueue[K, V](),
		quota: quota,
	}
}

// Upsert inserts a key-value pair into the Queue, or updates value if key is
// already present.
func (q *Queue[K, V]) Upsert(key K, value V) {
	if i, ok := q.pq.index[key]; ok {
		q.pq.items[i].value = value
		q.pq.items[i].count = 0
		heap.Fix(&q.pq, i)
	} else {
		heap.Push(&q.pq, &item[K, V]{key: key, value: value})
	}
}

// Pop returns a value of the highest priority and removes it from the Queue if
// the number of times it has been returned is greater than or equal to the
// value returned by quota. Pop panics if the Queue is empty.
func (q *Queue[K, V]) Pop() V {
	it := heap.Pop(&q.pq).(*item[K, V])
	if it.count++; it.count < q.quota() {
		heap.Push(&q.pq, it)
	}
	return it.value
}

// PopN returns up to n distinct items of the highest priorities. If there are
// at least n items in the queue, PopN returns n of them, or else all of them.
// PopN removes any returned items from the Queue for which the number of times
// they have been returned is greater than or equal to the value returned by
// quota.
func (q *Queue[K, V]) PopN(n int) []V {
	quota := q.quota()
	var values []V
	var reinsert []*item[K, V]
	for q.pq.Len() > 0 && len(values) < n {
		it := heap.Pop(&q.pq).(*item[K, V])
		values = append(values, it.value)
		if it.count++; it.count < quota {
			reinsert = append(reinsert, it)
		}
	}
	for _, it := range reinsert {
		heap.Push(&q.pq, it)
	}
	return values
}

// Remove removes key and its associated value from the Queue. If key is the
// zero value of type K or is not present, Remove is a no-op.
func (q *Queue[K, V]) Remove(key K) {
	if i, ok := q.pq.index[key]; ok {
		heap.Remove(&q.pq, i)
	}
}

// Len returns the number of items in the Queue.
func (q *Queue[K, V]) Len() int { return q.pq.Len() }

// A priorityQueue implements heap.Interface and holds items.
type priorityQueue[K comparable, V any] struct {
	items []*item[K, V]
	index map[K]int
}

func makePriorityQueue[K comparable, V any]() priorityQueue[K, V] {
	return priorityQueue[K, V]{index: make(map[K]int)}
}

func (pq priorityQueue[K, V]) Len() int { return len(pq.items) }

func (pq priorityQueue[K, V]) Less(i, j int) bool {
	return pq.items[i].count < pq.items[j].count
}

func (pq priorityQueue[K, V]) Swap(i, j int) {
	a, b := pq.items[j], pq.items[i]
	pq.index[a.key] = i
	pq.index[b.key] = j
	pq.items[i], pq.items[j] = a, b
}

func (pq *priorityQueue[K, V]) Push(a any) {
	item := a.(*item[K, V])
	pq.index[item.key] = len(pq.items)
	pq.items = append(pq.items, item)
}

func (pq *priorityQueue[K, V]) Pop() any {
	last := len(pq.items) - 1
	item := pq.items[last]
	pq.items[last] = nil
	pq.items = pq.items[:last]
	delete(pq.index, item.key)
	return item
}
