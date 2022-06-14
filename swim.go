package swim

import (
	"encoding/json"
	"math/rand"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/dkmccandless/swim/bufchan"
)

const (
	tickAverage = time.Second
	pingTimeout = 200 * time.Millisecond
)

// An Update describes a change to the network membership.
type Update struct {
	ID       string
	IsMember bool
	Addr     netip.AddrPort
}

// A Memo carries user-defined data.
type Memo struct {
	SrcID   string
	SrcAddr netip.AddrPort
	Body    []byte
}

// A Node is a network node participating in the SWIM protocol.
type Node struct {
	mu  sync.Mutex // protects the following fields
	fsm *stateMachine

	id       id // copy of fsm.id
	conn     *net.UDPConn
	updates  bufchan.Chan[Update]
	memos    bufchan.Chan[Memo]
	stopTick chan struct{}
}

// Start creates a new Node and starts running the SWIM protocol on it.
func Start() (*Node, error) {
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}
	updates := bufchan.Make[Update]()
	memos := bufchan.Make[Memo]()
	fsm := newStateMachine(updates.Send(), memos.Send())
	n := &Node{
		fsm:      fsm,
		id:       fsm.id,
		conn:     conn,
		updates:  updates,
		memos:    memos,
		stopTick: make(chan struct{}),
	}
	go n.runReceive()
	go n.runTick()
	return n, nil
}

func (n *Node) runTick() {
	defer close(n.updates.Send())
	periodTimer := time.NewTimer(0)
	pingTimer := stoppedTimer()
	for {
		select {
		case <-periodTimer.C:
			// Choose a random tick period within 10% of tickAverage to
			// desynchronize the nodes' periods
			tickPeriod := time.Duration(float64(tickAverage) * (0.9 + 0.2*rand.Float64()))
			periodTimer.Reset(tickPeriod)
			pingTimer.Reset(pingTimeout)
			n.send(n.tick())
		case <-pingTimer.C:
			n.send(n.timeout())
		case <-n.stopTick:
			return
		}
	}
}

func (n *Node) tick() []packet {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.fsm.tick()
}

func (n *Node) timeout() []packet {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.fsm.timeout()
}

// Join connects n to a remote node. This is typically used to connect a new
// node to an existing network.
func (n *Node) Join(remote netip.AddrPort) error {
	n.mu.Lock()
	p := packet{
		Type: ping,
		Msgs: []*message{n.fsm.aliveMessage()},
	}
	n.mu.Unlock()
	return n.writeTo(p, remote)
}

func (n *Node) send(ps []packet) {
	for _, p := range ps {
		if err := n.writeTo(p, p.remoteAddr); err != nil {
			return
		}
	}
}

// writeTo writes p to addr.
func (n *Node) writeTo(p packet, addr netip.AddrPort) error {
	b, err := json.Marshal(envelope{n.id, p})
	if err != nil {
		panic(err)
	}
	_, err = n.conn.WriteToUDPAddrPort(b, addr)
	return err
}

func (n *Node) runReceive() {
	defer close(n.stopTick)
	for {
		b := make([]byte, 1<<16)
		len, addr, err := n.conn.ReadFromUDPAddrPort(b)
		if err != nil {
			return
		}
		var e envelope
		if err := json.Unmarshal(b[:len], &e); err != nil {
			continue
		}
		e.P.remoteID = e.FromID
		e.P.remoteAddr = addr
		ps, ok := n.receive(e.P)
		if !ok {
			return
		}
		n.send(ps)
	}
}

func (n *Node) receive(p packet) ([]packet, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.fsm.receive(p)
}

// PostMemo disseminates b throughout the network.
func (n *Node) PostMemo(b []byte) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.fsm.addMemo(b)
}

// Updates returns a channel from which Updates can be received. The channel is
// closed when n ceases participation in the protocol.
func (n *Node) Updates() <-chan Update {
	return n.updates.Receive()
}

// Memos returns a channel from which Memos can be received. The channel is
// closed when n ceases participation in the protocol.
func (n *Node) Memos() <-chan Memo {
	return n.memos.Receive()
}

// ID returns n's ID on the network.
func (n *Node) ID() string {
	return string(n.id)
}

// LocalAddr returns the local network address.
func (n *Node) LocalAddr() netip.AddrPort {
	return n.conn.LocalAddr().(*net.UDPAddr).AddrPort()
}

type envelope struct {
	FromID id
	P      packet
}

func stoppedTimer() *time.Timer {
	t := time.NewTimer(0)
	if !t.Stop() {
		<-t.C
	}
	return t
}
