package swim

import (
	"math"
	"math/rand"
)

// A memberList tracks the membership of other nodes in the network and
// maintains a round-robin ordering for ping target selection.
type memberList struct {
	members     map[id]*message
	suspects    map[id]int  // number of periods under suspicion
	uncontacted map[id]bool // ids that have not been sent to
	removed     map[id]bool // removed ids // TODO: expire old entries by timestamp

	order []id
	i     int
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
	if ml.i = (ml.i + 1) % len(ml.order); ml.i == 0 {
		rand.Shuffle(len(ml.order), func(i, j int) {
			ml.order[i], ml.order[j] = ml.order[j], ml.order[i]
		})
	}
	return ml.order[ml.i], failed
}

// update updates a node's membership status based on a received message and
// returns an Update if the membership list changed.
func (ml *memberList) update(msg *message) *Update {
	id := msg.ID
	if !supersedes(msg, ml.members[id]) {
		return nil
	}
	var u *Update
	switch msg.Type {
	case alive:
		u = ml.add(id)
		ml.members[id] = msg
		delete(ml.suspects, id)
	case suspected:
		u = ml.add(id)
		ml.members[id] = msg
		ml.suspects[id] = 0
	case failed:
		return ml.remove(id)
	}
	return u
}

// add adds a new id to the list, inserts it into a random position in the
// order, and returns an Update. If the id is a current or former member, add
// returns nil instead.
func (ml *memberList) add(id id) *Update {
	if ml.isMember(id) || ml.removed[id] {
		return nil
	}
	ml.members[id] = nil
	ml.uncontacted[id] = true

	pos := rand.Intn(len(ml.order) + 1)
	if pos > ml.i {
		ml.order = append(ml.order, id)
		ml.order[pos], ml.order[len(ml.order)-1] = ml.order[len(ml.order)-1], ml.order[pos]
	} else {
		ml.order = append(append(ml.order[:pos], id), ml.order[pos:]...)
		ml.i++
	}
	return &Update{ID: string(id), IsMember: true}
}

// remove removes an id from the list and returns an Update if it was a member.
func (ml *memberList) remove(id id) *Update {
	if !ml.isMember(id) {
		return nil
	}
	delete(ml.members, id)
	delete(ml.suspects, id)
	ml.removed[id] = true

	var pos int
	for pos = range ml.order {
		if ml.order[pos] == id {
			break
		}
	}
	if pos > ml.i {
		ml.order[pos] = ml.order[len(ml.order)-1]
		ml.order = ml.order[:len(ml.order)-1]
	} else {
		ml.order = append(ml.order[:pos], ml.order[pos+1:]...)
		ml.i--
	}
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
