package swim

import (
	"math"
	"net/netip"

	"github.com/dkmccandless/swim/internal/roundrobinrandom"
	"github.com/dkmccandless/swim/internal/rpq"
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

	msgQueue *rpq.Queue[id, *message]

	pingTarget id
	gotAck     bool

	pingReqs map[id]id

	nPingReqs int
	maxMsgs   int

	updates chan<- Update
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
	NodeID      id
	Incarnation int
	Addr        netip.AddrPort
}

// A profile contains an ID's membership information.
type profile struct {
	incarnation int
	contacted   bool
	addr        netip.AddrPort
}

// newStateMachine initializes a new stateMachine emitting Updates on the
// provided channel, which must never block.
func newStateMachine(updates chan<- Update) *stateMachine {
	s := &stateMachine{
		id: randID(),

		members:  make(map[id]*profile),
		suspects: make(map[id]int),
		removed:  make(map[id]bool),

		pingReqs:  make(map[id]id),
		nPingReqs: 2, // TODO: scale according to permissible false positive probability
		maxMsgs:   6, // TODO: revisit guaranteed MTU constraint

		updates: updates,
	}
	s.msgQueue = rpq.New[id, *message](func() int {
		// A small multiple of the logarithm of the size of the network
		// suffices to ensure reliable dissemination.
		return int(3*math.Log(float64(len(s.members)+1))) + 1
	})
	return s
}

// tick begins a new protocol period and returns a ping, as well as packets to
// notify any members declared suspected or failed.
func (s *stateMachine) tick() []packet {
	var ps []packet

	for id := range s.suspects {
		if s.suspects[id]++; s.suspects[id] >= s.suspicionTimeout() {
			// Suspicion timeout
			m := s.failedMessage(id)
			s.msgQueue.Upsert(id, m)
			ps = append(ps, s.makeMessagePing(m))
			s.remove(id)
		}
	}

	if id := s.pingTarget; !s.gotAck && s.isMember(id) {
		// Expired ping target
		if !s.isSuspect(id) {
			s.suspects[id] = 0
		}
		m := s.suspectedMessage(id)
		s.msgQueue.Upsert(id, m)
		ps = append(ps, s.makeMessagePing(m))
	}

	s.pingTarget = s.order.Next()
	s.gotAck = false
	s.pingReqs = map[id]id{}
	if s.pingTarget == "" {
		return ps
	}
	return append(ps, s.makePing(s.pingTarget))
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

// receive processes an incoming packet and returns any necessary outgoing
// packets and a boolean value reporting whether the stateMachine can continue
// participating in the protocol.
func (s *stateMachine) receive(p packet) ([]packet, bool) {
	if s.removed[p.remoteID] {
		return nil, true
	}
	for _, m := range p.Msgs {
		if m.Addr == (netip.AddrPort{}) {
			m.Addr = p.remoteAddr
		}
		if !s.processMsg(m) {
			return nil, false
		}
	}
	return s.processPacketType(p), true
}

// processMsg processes a received message and reports whether the stateMachine
// can continue participating in the protocol.
func (s *stateMachine) processMsg(m *message) bool {
	if m.NodeID == s.id {
		if m.Type == suspected && m.Incarnation == s.incarnation {
			s.incarnation++
			s.msgQueue.Upsert(s.id, s.aliveMessage())
		}
		return m.Type != failed
	}
	if s.isNews(m) {
		s.msgQueue.Upsert(m.NodeID, m)
		s.updateStatus(m)
	}
	return true
}

// updateStatus updates a node's membership status based on a received message
// and emits an Update if the membership list changed.
func (s *stateMachine) updateStatus(m *message) {
	id := m.NodeID
	if m.Type == failed {
		s.remove(id)
		return
	}
	if !s.isMember(id) {
		s.members[id] = new(profile)
		s.order.Add(id)
		s.updates <- Update{ID: string(id), IsMember: true, Addr: m.Addr}
	}
	s.members[id].incarnation = m.Incarnation
	s.members[id].addr = m.Addr
	switch m.Type {
	case alive:
		delete(s.suspects, id)
	case suspected:
		s.suspects[id] = 0
	}
}

// remove removes an id from the list and emits an Update if it was a member.
func (s *stateMachine) remove(id id) {
	if !s.isMember(id) {
		return
	}
	s.updates <- Update{ID: string(id), IsMember: false, Addr: s.members[id].addr}
	delete(s.members, id)
	delete(s.suspects, id)
	s.removed[id] = true
	s.order.Remove(id)
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
	id := m.NodeID
	if !s.isMember(id) {
		return !s.removed[id]
	}
	if m.Type == failed {
		return true
	}
	incarnation := s.members[id].incarnation
	if m.Incarnation == incarnation {
		return m.Type == suspected && !s.isSuspect(id)
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
		msgs = append(s.msgQueue.PopN(s.maxMsgs-1), s.aliveMessage())
	} else {
		msgs = s.msgQueue.PopN(s.maxMsgs)
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
		remoteID:   m.NodeID,
		remoteAddr: m.Addr,
		Msgs:       []*message{m},
	}
}

// aliveMessage returns a message reporting the stateMachine as alive.
func (s *stateMachine) aliveMessage() *message {
	return &message{
		Type:        alive,
		NodeID:      s.id,
		Incarnation: s.incarnation,
	}
}

// suspectedMessage returns a message reporting an id as suspected.
func (s *stateMachine) suspectedMessage(id id) *message {
	return &message{
		Type:        suspected,
		NodeID:      id,
		Incarnation: s.members[id].incarnation,
		Addr:        s.members[id].addr,
	}
}

// failedMessage returns a message reporting an id as failed.
func (s *stateMachine) failedMessage(id id) *message {
	return &message{
		Type:   failed,
		NodeID: id,
		Addr:   s.members[id].addr,
	}
}
