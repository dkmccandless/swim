package swim

import (
	"encoding/base32"
	"math/rand"
	"net/netip"
)

type id string

// A stateMachine is a finite state machine that implements the SWIM
// protocol.
type stateMachine struct {
	id          id
	incarnation int

	addrs map[id]netip.AddrPort
	ml    *memberList
	mq    *messageQueue

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
	Type        status
	ID          id
	Incarnation int
}

func newStateMachine() *stateMachine {
	s := &stateMachine{
		id: randID(),

		addrs: make(map[id]netip.AddrPort),
		ml:    newMemberList(),

		pingReqs:  make(map[id]id),
		nPingReqs: 2, // TODO: scale according to permissible false positive probability
		maxMsgs:   6, // TODO: revisit guaranteed MTU constraint
	}
	s.mq = newMessageQueue(func() int { return len(s.ml.members) + 1 })
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
		id := id(u.ID)
		u.Addr = s.addrs[id]
		delete(s.addrs, id)
		m := s.failedMessage(id)
		s.mq.update(m)
		ps = append(ps, s.makeMessagePing(m))
	}

	if s.pingTarget == "" {
		return ps, failed
	}
	return append(ps, s.makePing(s.pingTarget)), failed
}

// timeout produces ping requests if an ack has not been received from the
// ping target, or else nil.
func (s *stateMachine) timeout() []packet {
	if s.gotAck || !s.ml.isMember(s.pingTarget) {
		return nil
	}
	var ps []packet
	for _, i := range rand.Perm(len(s.ml.order)) {
		id := s.ml.order[i]
		if id == s.pingTarget {
			continue
		}
		ps = append(ps, s.makePingReq(id, s.pingTarget))
		if len(ps) == s.nPingReqs {
			break
		}
	}
	return ps
}

// receive processes an incoming packet and produces any necessary outgoing
// packets and Updates in response.
func (s *stateMachine) receive(p packet, src netip.AddrPort, addrs []netip.AddrPort) ([]packet, []Update) {
	if s.addrs[p.remoteID] == (netip.AddrPort{}) {
		// First contact from sender
		s.addrs[p.remoteID] = src
	}
	// Update address records
	for i, addr := range addrs {
		id := p.Msgs[i].ID
		if id == s.id || addr == (netip.AddrPort{}) {
			continue
		}
		s.addrs[id] = addr
	}

	var us []Update
	for _, m := range p.Msgs {
		if u := s.processMsg(m); u != nil {
			id := id(u.ID)
			u.Addr = s.addrs[id]
			if !u.IsMember {
				delete(s.addrs, id)
			}
			us = append(us, *u)
		}
	}
	return s.processPacketType(p), us
}

func (s *stateMachine) processMsg(m *message) *Update {
	if m.ID == s.id {
		switch m.Type {
		case suspected:
			if m.Incarnation == s.incarnation {
				s.incarnation++
				s.mq.update(s.aliveMessage())
			}
		case failed:
			// TODO
		}
		return nil
	}
	if !supersedes(m, s.ml.members[m.ID]) {
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

func (s *stateMachine) getAddrs(p packet) (dst netip.AddrPort, addrs []netip.AddrPort) {
	dst = s.addrs[p.remoteID]
	if dst == (netip.AddrPort{}) {
		return netip.AddrPort{}, nil
	}
	addrs = make([]netip.AddrPort, len(p.Msgs))
	for i, m := range p.Msgs {
		addrs[i] = s.addrs[m.ID]
	}
	return dst, addrs
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
		remoteID: m.ID,
		Msgs:     []*message{m},
	}
}

// aliveMessage returns a message reporting the stateMachine as alive.
func (s *stateMachine) aliveMessage() *message {
	return &message{
		Type:        alive,
		ID:          s.id,
		Incarnation: s.incarnation,
	}
}

// suspectedMessage returns a message reporting an id as suspected.
func (s *stateMachine) suspectedMessage(id id) *message {
	return &message{
		Type:        suspected,
		ID:          id,
		Incarnation: s.ml.members[id].Incarnation,
	}
}

// failedMessage returns a message reporting an id as failed.
func (s *stateMachine) failedMessage(id id) *message {
	return &message{
		Type: failed,
		ID:   id,
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
	if a.ID != b.ID {
		return false
	}
	if b.Type == failed {
		return false
	}
	if a.Type == failed {
		return true
	}
	if a.Incarnation == b.Incarnation {
		return a.Type == suspected && b.Type == alive
	}
	return a.Incarnation > b.Incarnation
}

func randID() id {
	b := make([]byte, 15)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return id(base32.StdEncoding.EncodeToString(b))
}
