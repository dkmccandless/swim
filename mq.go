package swim

import (
	"container/heap"
	"math"
)

// A messageQueue is a recurrent priority queue for outgoing messages.
// Messages are prioritized by the number of times they have been sent, and
// are removed from the queue after a quota of sends. Each id is the subject
// of at most one message.
type messageQueue struct {
	list     msgList
	numNodes func() int
}

// newMessageQueue creates a new messageQueue and returns a pointer to it.
// numNodes reports the size of the network, which must be nonzero.
func newMessageQueue(numNodes func() int) *messageQueue {
	return &messageQueue{numNodes: numNodes}
}

// get returns up to n messages for sending.
func (mq *messageQueue) get(n int) []*message {
	if n > len(mq.list) {
		n = len(mq.list)
	}
	msgs := make([]*message, n)
	for i, item := range mq.list[:n] {
		msgs[i] = item.m
		mq.list[i].n++
	}

	// quota is the number of times to send a message before removing it from
	// the queue. A small multiple of log(numNodes) suffices to ensure
	// reliable dissemination throughout the network.
	quota := int(3*math.Log(float64(mq.numNodes()))) + 1

	// Fix/Remove messages in reverse order after all of their counters have
	// been incremented. Fixing is not necessary if all messages in the list
	// are used: incrementing all counts preserves heap.Interface's invariants.
	for i := n - 1; i >= 0; i-- {
		if mq.list[i].n >= quota {
			heap.Remove(&mq.list, i)
		} else if n < len(mq.list) {
			heap.Fix(&mq.list, i)
		}
	}
	return msgs
}

// update updates an id's queued message if one exists, or else adds a new
// message.
func (mq *messageQueue) update(m *message) {
	for pos := range mq.list {
		if mq.list[pos].m.ID != m.ID {
			continue
		}
		mq.list[pos] = msgItem{m, 0}
		heap.Fix(&mq.list, pos)
		return
	}
	heap.Push(&mq.list, msgItem{m, 0})
}

// A msgItem records how many times a message has been sent.
type msgItem struct {
	m *message
	n int
}

// a msgList is a priority queue of msgItems. msgList implements heap.Interface.
type msgList []msgItem

func (l msgList) Len() int {
	return len(l)
}

func (l msgList) Less(i, j int) bool {
	return l[i].n < l[j].n
}

func (l msgList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l *msgList) Push(item interface{}) {
	*l = append(*l, item.(msgItem))
}

func (l *msgList) Pop() interface{} {
	item := (*l)[len(*l)-1]
	*l = (*l)[:len(*l)-1]
	return item
}
