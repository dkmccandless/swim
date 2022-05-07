package swim

import (
	"math"
	"net/netip"

	"github.com/dkmccandless/swim/internal/roundrobinrandom"
)

// A stateMachine is a finite state machine that implements the SWIM
// protocol.
type stateMachine struct {
	id          id
	incarnation int

	members  map[id]*profile
	suspects map[id]int  // number of periods under suspicion
	removed  map[id]bool // removed ids // TODO: expire old entries by timestamp

	order roundrobinrandom.Order[id]

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
	Type       packetType
	remoteID   id
	remoteAddr netip.AddrPort

	// for ping requests
	TargetID   id
	TargetAddr netip.AddrPort

	Msgs []*message
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

// A profile contains an ID's membership information.
type profile struct {
	incarnation int
	contacted   bool
	addr        netip.AddrPort
}

func newStateMachine() *stateMachine {
	s := &stateMachine{
		id: randID(),

		members:  make(map[id]*profile),
		suspects: make(map[id]int),
		removed:  make(map[id]bool),

		pingReqs:  make(map[id]id),
		nPingReqs: 2, // TODO: scale according to permissible false positive probability
		maxMsgs:   6, // TODO: revisit guaranteed MTU constraint
	}
	s.mq = newMessageQueue(func() int { return len(s.members) + 1 })
	return s
}

// tick begins a new protocol period and returns a ping, as well as packets to
// notify any members declared suspected or failed and corresponding
// Updates.
func (s *stateMachine) tick() ([]packet, []Update) {
	var ps []packet

	var failed []Update
	for id := range s.suspects {
		if s.suspects[id]++; s.suspects[id] >= s.suspicionTimeout() {
			// Suspicion timeout
			m := s.failedMessage(id)
			s.mq.update(m)
			ps = append(ps, s.makeMessagePing(m))
			u := s.remove(id)
			failed = append(failed, *u)
		}
	}

	if !s.gotAck && s.isMember(s.pingTarget) {
		// Expired ping target
		if !s.isSuspect(s.pingTarget) {
			s.suspects[s.pingTarget] = 0
		}
		m := s.suspectedMessage(s.pingTarget)
		s.mq.update(m)
		ps = append(ps, s.makeMessagePing(m))
	}

	s.pingTarget = s.order.Next()
	s.gotAck = false
	s.pingReqs = map[id]id{}
	if s.pingTarget == "" {
		return ps, failed
	}
	return append(ps, s.makePing(s.pingTarget)), failed
}

// timeout produces ping requests if an ack has not been received from the
// ping target, or else nil.
func (s *stateMachine) timeout() []packet {
	if s.gotAck || !s.isMember(s.pingTarget) {
		return nil
	}
	var ps []packet
	for _, id := range s.order.IndependentSample(s.nPingReqs, s.pingTarget) {
		ps = append(ps, s.makePingReq(id, s.pingTarget, s.members[s.pingTarget].addr))
	}
	return ps
}

// receive processes an incoming packet and produces any necessary outgoing
// packets and Updates in response. The boolean return value reports whether
// the stateMachine can continue participating in the protocol.
func (s *stateMachine) receive(p packet) ([]packet, []Update, bool) {
	if s.removed[p.remoteID] {
		return nil, nil, true
	}
	var us []Update
	for _, m := range p.Msgs {
		if m.Addr == (netip.AddrPort{}) {
			m.Addr = p.remoteAddr
		}
		u, ok := s.processMsg(m)
		if !ok {
			return nil, nil, false
		}
		if u != nil {
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
	if !s.isNews(m) {
		return nil, true
	}
	s.mq.update(m)
	return s.update(m), true
}

// update updates a node's membership status based on a received message and
// returns an Update if the membership list changed.
func (s *stateMachine) update(m *message) *Update {
	id := m.ID
	if m.Type == failed {
		return s.remove(id)
	}
	var u *Update
	if !s.isMember(id) {
		s.members[id] = new(profile)
		s.order.Add(id)
		u = &Update{ID: string(id), IsMember: true, Addr: m.Addr}
	}
	s.members[id].incarnation = m.Incarnation
	s.members[id].addr = m.Addr
	switch m.Type {
	case alive:
		delete(s.suspects, id)
	case suspected:
		s.suspects[id] = 0
	}
	return u
}

// remove removes an id from the list and returns an Update if it was a member.
func (s *stateMachine) remove(id id) *Update {
	if !s.isMember(id) {
		return nil
	}
	u := &Update{ID: string(id), IsMember: false, Addr: s.members[id].addr}
	delete(s.members, id)
	delete(s.suspects, id)
	s.removed[id] = true
	s.order.Remove(id)
	return u
}

// processPacketType processes an incoming packet and returns any necessary
// outgoing packets.
func (s *stateMachine) processPacketType(p packet) []packet {
	switch p.Type {
	case ping:
		return []packet{s.makeAck(p.remoteID)}
	case pingReq:
		s.pingReqs[p.remoteID] = p.TargetID
		return []packet{s.makePing(p.TargetID)}
	case ack:
		if p.remoteID == s.pingTarget || p.TargetID == s.pingTarget {
			s.gotAck = true
		}
		var ps []packet
		for src, target := range s.pingReqs {
			if target == p.remoteID {
				ps = append(ps, s.makeReqAck(src, p.remoteID, p.remoteAddr))
				delete(s.pingReqs, src)
			}
		}
		return ps
	}
	return nil
}

// suspicionTimeout returns the number of periods to wait before declaring a
// suspect failed.
func (s *stateMachine) suspicionTimeout() int {
	return int(3*math.Log(float64(len(s.members)))) + 1
}

// isMember reports whether an id is a member.
func (s *stateMachine) isMember(id id) bool {
	_, ok := s.members[id]
	return ok
}

// isSuspect reports whether an id is suspected.
func (s *stateMachine) isSuspect(id id) bool {
	_, ok := s.suspects[id]
	return ok
}

// isNews reports whether m contains new membership status information.
func (s *stateMachine) isNews(m *message) bool {
	if m == nil {
		return false
	}
	if !s.isMember(m.ID) {
		return !s.removed[m.ID]
	}
	if m.Type == failed {
		return true
	}
	incarnation := s.members[m.ID].incarnation
	if m.Incarnation == incarnation {
		return m.Type == suspected && !s.isSuspect(m.ID)
	}
	return m.Incarnation > incarnation
}

func (s *stateMachine) makePing(dst id) packet {
	return s.makePacket(ping, dst, "", netip.AddrPort{})
}

func (s *stateMachine) makeAck(dst id) packet {
	return s.makePacket(ack, dst, "", netip.AddrPort{})
}

func (s *stateMachine) makePingReq(dst, target id, targetAddr netip.AddrPort) packet {
	return s.makePacket(pingReq, dst, target, targetAddr)
}

func (s *stateMachine) makeReqAck(dst, target id, targetAddr netip.AddrPort) packet {
	return s.makePacket(ack, dst, target, targetAddr)
}

// makePacket assembles a packet and populates it with messages. If dst has
// not been sent to before, one of the messages is an introductory alive
// message.
func (s *stateMachine) makePacket(typ packetType, dst, target id, targetAddr netip.AddrPort) packet {
	var msgs []*message
	if !s.members[dst].contacted {
		s.members[dst].contacted = true
		msgs = append(s.mq.get(s.maxMsgs-1), s.aliveMessage())
	} else {
		msgs = s.mq.get(s.maxMsgs)
	}
	return packet{
		Type:       typ,
		remoteID:   dst,
		remoteAddr: s.members[dst].addr,
		TargetID:   target,
		TargetAddr: targetAddr,
		Msgs:       msgs,
	}
}

// makeMessagePing returns a ping that delivers a single message to its subject.
func (s *stateMachine) makeMessagePing(m *message) packet {
	return packet{
		Type:       ping,
		remoteID:   m.ID,
		remoteAddr: m.Addr,
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
		Incarnation: s.members[id].incarnation,
		Addr:        s.members[id].addr,
	}
}

// failedMessage returns a message reporting an id as failed.
func (s *stateMachine) failedMessage(id id) *message {
	return &message{
		Type: failed,
		ID:   id,
		Addr: s.members[id].addr,
	}
}
