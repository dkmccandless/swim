package swim

import (
	"math"

	"github.com/dkmccandless/swim/internal/roundrobinrandom"
)

// A memberList tracks the membership of other nodes in the network and
// maintains a round-robin ordering for ping target selection.
type memberList struct {
	members     map[id]*message
	suspects    map[id]int  // number of periods under suspicion
	uncontacted map[id]bool // ids that have not been sent to
	removed     map[id]bool // removed ids // TODO: expire old entries by timestamp

	order roundrobinrandom.Order[id]
}

func newMemberList() *memberList {
	return &memberList{
		members:     make(map[id]*message),
		suspects:    make(map[id]int),
		uncontacted: make(map[id]bool),
		removed:     make(map[id]bool),
	}
}

// tick begins a new protocol period and returns the ping target and Updates
// for any ids declared failed.
func (ml *memberList) tick() (target id, failed []Update) {
	for id := range ml.suspects {
		if ml.suspects[id]++; ml.suspects[id] >= ml.suspicionTimeout() {
			failed = append(failed, *ml.remove(id))
		}
	}
	return ml.order.Next(), failed
}

// update updates a node's membership status based on a received message and
// returns an Update if the membership list changed.
func (ml *memberList) update(msg *message) *Update {
	id := msg.ID
	if ml.removed[id] || !supersedes(msg, ml.members[id]) {
		return nil
	}
	if msg.Type == failed {
		return ml.remove(id)
	}
	var u *Update
	if !ml.isMember(id) {
		ml.uncontacted[id] = true
		ml.order.Add(id)
		u = &Update{ID: string(id), IsMember: true}
	}
	ml.members[id] = msg
	switch msg.Type {
	case alive:
		delete(ml.suspects, id)
	case suspected:
		ml.suspects[id] = 0
	}
	return u
}

// remove removes an id from the list and returns an Update if it was a member.
func (ml *memberList) remove(id id) *Update {
	if !ml.isMember(id) {
		return nil
	}
	delete(ml.members, id)
	delete(ml.suspects, id)
	ml.removed[id] = true
	ml.order.Remove(id)
	return &Update{ID: string(id), IsMember: false}
}

// suspicionTimeout returns the number of periods to wait before declaring a
// suspect failed.
func (ml *memberList) suspicionTimeout() int {
	return int(3*math.Log(float64(len(ml.members)))) + 1
}

// isMember reports whether an id is a member.
func (ml *memberList) isMember(id id) bool {
	_, ok := ml.members[id]
	return ok
}
