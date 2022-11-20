package swim

import (
	"math"
	"net/netip"

	"github.com/dkmccandless/swim/internal/roundrobinrandom"
	"github.com/dkmccandless/swim/internal/rpq"
)

// A stateMachine is a finite state machine that implements the SWIM protocol.
type stateMachine struct {
	id          id
	incarnation int

	members  map[id]*profile
	suspects map[id]int  // number of periods under suspicion
	removed  map[id]bool // removed ids // TODO: expire old entries by timestamp

	order roundrobinrandom.Order[id]

	msgQueue  *rpq.Queue[id, *message]
	memoQueue *rpq.Queue[id, *message]
	seenMemos map[id]bool

	pingTarget id
	gotAck     bool
	pingReqs   map[id]id

	nPingReqs int
	maxMsgs   int

	handleJoin func(id, netip.AddrPort)
	handleMemo func(id, netip.AddrPort, []byte)
	handleFail func(id)
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
	TargetID   id             `json:",omitempty"`
	TargetAddr netip.AddrPort `json:",omitempty"`

	Msgs []*message `json:",omitempty"`
}

// A msgType describes the meaning of a message.
type msgType byte

const (
	alive msgType = iota
	suspected
	failed
)

// A message carries membership information or memo data.
type message struct {
	Type        msgType
	NodeID      id
	Addr        netip.AddrPort
	Incarnation int

	// for memo
	MemoID id     `json:",omitempty"`
	Body   []byte `json:",omitempty"`
}

// A profile contains an ID's membership information.
type profile struct {
	incarnation int
	contacted   bool
	addr        netip.AddrPort
}

// newStateMachine initializes a new stateMachine emitting membership
// information and memos via the provided handler callbacks.
func newStateMachine(
	handleJoin func(id, netip.AddrPort),
	handleMemo func(id, netip.AddrPort, []byte),
	handleFail func(id),
) *stateMachine {
	s := &stateMachine{
		id: randID(),

		members:  make(map[id]*profile),
		suspects: make(map[id]int),
		removed:  make(map[id]bool),

		seenMemos: make(map[id]bool),

		pingReqs:  make(map[id]id),
		nPingReqs: 2, // TODO: scale according to permissible false positive probability
		maxMsgs:   6, // TODO: revisit guaranteed MTU constraint

		handleJoin: handleJoin,
		handleMemo: handleMemo,
		handleFail: handleFail,
	}

	s.msgQueue = rpq.New[id, *message](s.disseminationFactor)
	s.memoQueue = rpq.New[id, *message](s.disseminationFactor)
	return s
}

// tick begins a new protocol period and returns a ping, as well as packets to
// notify any members declared suspected or failed.
func (s *stateMachine) tick() []packet {
	var ps []packet
	for id := range s.suspects {
		if s.suspects[id]++; s.suspects[id] >= s.disseminationFactor() {
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
	s.gotAck = false
	s.pingReqs = map[id]id{}
	s.pingTarget = s.order.Next()
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
// packets and a boolean value reporting whether s can continue participating
// in the protocol.
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

// processMsg processes a received message and reports whether s can continue
// participating in the protocol.
func (s *stateMachine) processMsg(m *message) bool {
	if m.NodeID == s.id {
		if m.Type == suspected && m.Incarnation == s.incarnation {
			s.incarnation++
			s.msgQueue.Upsert(s.id, s.aliveMessage())
		}
		return m.Type != failed
	}
	if s.isMemberNews(m) {
		s.updateStatus(m)
		s.msgQueue.Upsert(m.NodeID, stripMemo(m))
	}
	if len(m.Body) > 0 && !s.seenMemos[m.MemoID] && s.isMember(m.NodeID) {
		s.seenMemos[m.MemoID] = true
		s.memoQueue.Upsert(m.MemoID, m)
		s.handleMemo(m.NodeID, m.Addr, m.Body)
	}
	return true
}

// updateStatus updates a node's membership status based on a received message
// and calls a handler if the membership list changed.
func (s *stateMachine) updateStatus(m *message) {
	id := m.NodeID
	if m.Type == failed {
		s.remove(id)
		return
	}
	if !s.isMember(id) {
		s.members[id] = new(profile)
		s.order.Add(id)
		s.handleJoin(id, m.Addr)
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

// remove removes an id from the list and calls handleFail if it was a member.
func (s *stateMachine) remove(id id) {
	if !s.isMember(id) {
		return
	}
	s.handleFail(id)
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

// disseminationFactor returns 2*log(n) rounded up, where n is the size of the
// network. Each message must be sent a small multiple of log(n) times to ensure
// reliable dissemination. Consequently, this is also the dissemination
// timescale, and by extension the number of protocol periods to wait before
// declaring a suspect failed.
func (s *stateMachine) disseminationFactor() int {
	const λ = 2 // must be greater than 1
	return int(math.Ceil(λ * math.Log(float64(len(s.members)+1))))
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

// isMemberNews reports whether m contains new membership status information.
func (s *stateMachine) isMemberNews(m *message) bool {
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
	// TODO: treat message sizes vs. packet capacity in more detail
	var msgs []*message
	if !s.members[dst].contacted {
		s.members[dst].contacted = true
		msgs = append(msgs, s.aliveMessage())
	}
	if s.memoQueue.Len() > 0 {
		msgs = append(msgs, s.memoQueue.Pop())
	}
	return packet{
		Type:       typ,
		remoteID:   dst,
		remoteAddr: s.members[dst].addr,
		TargetID:   target,
		TargetAddr: targetAddr,
		Msgs:       append(msgs, s.msgQueue.PopN(s.maxMsgs-len(msgs))...),
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

// aliveMessage returns a message reporting s as alive.
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

// addMemo adds a new memo carrying b to the memo queue.
func (s *stateMachine) addMemo(b []byte) {
	m := s.aliveMessage()
	memoID := randID()
	m.MemoID = memoID
	m.Body = b
	s.memoQueue.Upsert(memoID, m)
	s.seenMemos[memoID] = true
}

// stripMemo returns a copy of m without its memo data, if any.
func stripMemo(m *message) *message {
	n := new(message)
	*n = *m
	n.MemoID = ""
	n.Body = nil
	return n
}
