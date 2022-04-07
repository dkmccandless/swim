package swim

import (
	"encoding/base32"
	"math/rand"
)

type id string

// A stateMachine is a finite state machine that implements the SWIM
// protocol.
type stateMachine struct {
	id          id
	incarnation int

	ml *memberList
	mq *messageQueue

	pingTarget id
	gotAck     bool

	pingReqs map[id]id

	nPingReqs int
	maxMsgs   int
}

// A packetType describes the meaning of a packet.
type packetType byte

const (
	ping packetType = iota
	pingReq
	ack
)

// A packet represents a network packet.
type packet struct {
	Type     packetType
	remoteID id
	Target   id // for ping requests
	Msgs     []*message
}

// A status describes a node's membership status.
type status byte

const (
	alive status = iota
	suspected
	failed
)

// A message carries membership information.
type message struct {
	typ         status
	id          id
	incarnation int
}

func newStateMachine() *stateMachine {
	s := &stateMachine{
		id: randID(),
		ml: newMemberList(),

		pingReqs:  make(map[id]id),
		nPingReqs: 2, // TODO: scale according to permissible false positive probability
		maxMsgs:   6, // TODO: revisit guaranteed MTU constraint
	}
	s.mq = newMessageQueue(func() int { return len(s.ml.members) + 1 })
	s.mq.update(s.aliveMessage())
	return s
}

// tick begins a new protocol period and returns a ping, as well as packets to
// notify any members declared suspected or failed and corresponding
// Updates.
func (s *stateMachine) tick() ([]packet, []Update) {
	if len(s.ml.members) == 0 {
		return nil, nil
	}
	s.pingReqs = map[id]id{}
	var ps []packet

	// Handle expired ping target
	if !s.gotAck && s.ml.isMember(s.pingTarget) {
		// Avoid resetting an existing suspicion count
		s.ml.suspects[s.pingTarget] = s.ml.suspects[s.pingTarget]
		m := s.suspectedMessage(s.pingTarget)
		s.mq.update(m)
		ps = append(ps, s.makeMessagePing(m))
	}

	var failed []Update
	s.pingTarget, failed = s.ml.tick()
	s.gotAck = false

	// Handle suspicion timeouts
	for _, u := range failed {
		m := s.failedMessage(id(u.ID))
		s.mq.update(m)
		ps = append(ps, s.makeMessagePing(m))
	}

	return append(ps, s.makePing(s.pingTarget)), failed
}

// timeout produces ping requests if an ack has not been received from the
// ping target, or else nil.
func (s *stateMachine) timeout() []packet {
	if s.gotAck || !s.ml.isMember(s.pingTarget) {
		return nil
	}

	if len(s.ml.members) <= s.nPingReqs {
		var ps []packet
		for id := range s.ml.members {
			if id != s.pingTarget {
				ps = append(ps, s.makePingReq(id, s.pingTarget))
			}
		}
		return ps
	}

	var ps []packet
	used := map[id]bool{s.pingTarget: true}
	for len(ps) < s.nPingReqs {
		id := s.ml.order[rand.Intn(len(s.ml.order))]
		if used[id] {
			continue
		}
		used[id] = true
		ps = append(ps, s.makePingReq(id, s.pingTarget))
	}
	return ps
}

// receive processes an incoming packet and produces any necessary outgoing
// packets and Updates in response.
func (s *stateMachine) receive(p packet) ([]packet, []Update) {
	var us []Update
	for _, m := range p.Msgs {
		if u := s.processMsg(m); u != nil {
			us = append(us, *u)
		}
	}
	return s.processPacketType(p), us
}

func (s *stateMachine) processMsg(m *message) *Update {
	if m.id == s.id {
		switch m.typ {
		case suspected:
			if m.incarnation == s.incarnation {
				s.incarnation++
				s.mq.update(s.aliveMessage())
			}
		case failed:
			// TODO
		}
		return nil
	}
	if !supersedes(m, s.ml.members[m.id]) {
		return nil
	}
	s.mq.update(m)
	return s.ml.update(m)
}

func (s *stateMachine) processPacketType(p packet) []packet {
	switch p.Type {
	case ping:
		return []packet{s.makeAck(p.remoteID)}
	case pingReq:
		s.pingReqs[p.remoteID] = p.Target
		return []packet{s.makePing(p.Target)}
	case ack:
		if p.remoteID == s.pingTarget || p.Target == s.pingTarget {
			s.gotAck = true
		}
		var ps []packet
		for src, target := range s.pingReqs {
			if p.remoteID == target {
				ps = append(ps, s.makeReqAck(src, target))
				delete(s.pingReqs, src)
			}
		}
		return ps
	}
	return nil
}

func (s *stateMachine) makePing(dst id) packet {
	return s.makePacket(ping, dst, dst)
}

func (s *stateMachine) makeAck(dst id) packet {
	return s.makePacket(ack, dst, dst)
}

func (s *stateMachine) makePingReq(dst, target id) packet {
	return s.makePacket(pingReq, dst, target)
}

func (s *stateMachine) makeReqAck(dst, target id) packet {
	return s.makePacket(ack, dst, target)
}

// makePacket assembles a packet and populates it with messages. If dst has
// not been sent to before, one of the messages is an introductory alive
// message.
func (s *stateMachine) makePacket(typ packetType, dst, target id) packet {
	var msgs []*message
	if s.ml.uncontacted[dst] {
		delete(s.ml.uncontacted, dst)
		msgs = append(s.mq.get(s.maxMsgs-1), s.aliveMessage())
	} else {
		msgs = s.mq.get(s.maxMsgs)
	}
	return packet{
		Type:     typ,
		remoteID: dst,
		Target:   target,
		Msgs:     msgs,
	}
}

// makeMessagePing returns a ping that delivers a single message to its subject.
func (s *stateMachine) makeMessagePing(m *message) packet {
	return packet{
		Type:     ping,
		remoteID: m.id,
		Msgs:     []*message{m},
	}
}

// aliveMessage returns a message reporting the stateMachine as alive.
func (s *stateMachine) aliveMessage() *message {
	return &message{
		typ:         alive,
		id:          s.id,
		incarnation: s.incarnation,
	}
}

// suspectedMessage returns a message reporting an id as suspected.
func (s *stateMachine) suspectedMessage(id id) *message {
	return &message{
		typ:         suspected,
		id:          id,
		incarnation: s.ml.members[id].incarnation,
	}
}

// failedMessage returns a message reporting an id as failed.
func (s *stateMachine) failedMessage(id id) *message {
	return &message{
		typ: failed,
		id:  id,
	}
}

// supersedes reports whether a supersedes b.
func supersedes(a, b *message) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	if a.id != b.id {
		return false
	}
	if b.typ == failed {
		return false
	}
	if a.typ == failed {
		return true
	}
	if a.incarnation == b.incarnation {
		return a.typ == suspected && b.typ == alive
	}
	return a.incarnation > b.incarnation
}

func randID() id {
	b := make([]byte, 15)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return id(base32.StdEncoding.EncodeToString(b))
}
