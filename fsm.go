package swim

import (
	"net/netip"
)

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
	Type       packetType
	remoteID   id
	remoteAddr netip.AddrPort
	Target     id // for ping requests
	Msgs       []*message
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
	Addr        netip.AddrPort
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
		m := s.failedMessage(id)
		s.mq.update(m)
		ps = append(ps, s.makeMessagePing(m))
		delete(s.addrs, id)
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
	for _, id := range s.ml.order.IndependentSample(s.nPingReqs, s.pingTarget) {
		ps = append(ps, s.makePingReq(id, s.pingTarget))
	}
	return ps
}

// receive processes an incoming packet and produces any necessary outgoing
// packets and Updates in response. The boolean return value reports whether
// the stateMachine can continue participating in the protocol.
func (s *stateMachine) receive(p packet) ([]packet, []Update, bool) {
	if s.ml.removed[p.remoteID] {
		return nil, nil, true
	}
	if s.addrs[p.remoteID] == (netip.AddrPort{}) {
		// First contact from sender
		s.addrs[p.remoteID] = p.remoteAddr
	}
	// Update address records and populate empty message addresses
	for i, m := range p.Msgs {
		switch {
		case m.ID == s.id:
			continue
		case m.Addr == netip.AddrPort{}:
			p.Msgs[i].Addr = p.remoteAddr
		default:
			s.addrs[m.ID] = m.Addr
		}
	}

	var us []Update
	for _, m := range p.Msgs {
		u, ok := s.processMsg(m)
		if !ok {
			return nil, nil, false
		}
		if u != nil {
			id := id(u.ID)
			u.Addr = s.addrs[id]
			if !u.IsMember {
				delete(s.addrs, id)
			}
			us = append(us, *u)
		}
	}
	return s.processPacketType(p), us, true
}

// processMsg returns an Update if m results in a change of membership, or else
// nil. The boolean return value is false if the stateMachine has been declared
// failed, and true otherwise.
func (s *stateMachine) processMsg(m *message) (*Update, bool) {
	if m.ID == s.id {
		switch m.Type {
		case suspected:
			if m.Incarnation == s.incarnation {
				s.incarnation++
				s.mq.update(s.aliveMessage())
			}
		case failed:
			return nil, false
		}
		return nil, true
	}
	if !s.ml.isNews(m) {
		return nil, true
	}
	s.mq.update(m)
	return s.ml.update(m), true
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
	if !s.ml.members[dst].contacted {
		s.ml.members[dst].contacted = true
		msgs = append(s.mq.get(s.maxMsgs-1), s.aliveMessage())
	} else {
		msgs = s.mq.get(s.maxMsgs)
	}
	return packet{
		Type:       typ,
		remoteID:   dst,
		remoteAddr: s.addrs[dst],
		Target:     target,
		Msgs:       msgs,
	}
}

// makeMessagePing returns a ping that delivers a single message to its subject.
func (s *stateMachine) makeMessagePing(m *message) packet {
	return packet{
		Type:       ping,
		remoteID:   m.ID,
		remoteAddr: s.addrs[m.ID],
		Msgs:       []*message{m},
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
		Incarnation: s.ml.members[id].incarnation,
		Addr:        s.addrs[id],
	}
}

// failedMessage returns a message reporting an id as failed.
func (s *stateMachine) failedMessage(id id) *message {
	return &message{
		Type: failed,
		ID:   id,
		Addr: s.addrs[id],
	}
}
