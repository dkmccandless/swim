package swim

import (
	"encoding/json"
	"math/rand"
	"net"
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
	Addr     net.Addr
	ID       string
	IsMember bool
}

// A Node is a network node participating in the SWIM protocol.
type Node struct {
	mu    sync.Mutex // protects the following fields
	s     *stateMachine
	addrs map[id]net.Addr

	id   id // copy of s.id
	conn net.PacketConn
	ch   bufchan.Chan[Update]
}

// Start creates a new Node and starts running the SWIM protocol on it.
func Start() (*Node, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil, err
	}
	s := newStateMachine()
	n := &Node{
		s:     s,
		addrs: make(map[id]net.Addr),
		id:    s.id,
		conn:  conn,
		ch:    bufchan.Make[Update](),
	}
	go n.runReceive()
	go n.runTick()
	return n, nil
}

func (n *Node) runTick() {
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
			n.send(n.tick()...)
		case <-pingTimer.C:
			n.send(n.s.timeout()...)
		}
	}
}

func (n *Node) tick() []packet {
	n.mu.Lock()
	defer n.mu.Unlock()
	ps, us := n.s.tick()
	for _, u := range us {
		id := id(u.ID)
		u.Addr = n.addrs[id]
		delete(n.addrs, id)
		n.ch.Send() <- u
	}
	return ps
}

// Join connects two previously unconnected nodes. Join is typically called
// from a new node to connect with an existing node, or from an existing node
// to connect with a new node.
func (n *Node) Join(addr net.Addr) {
	n.mu.Lock()
	p := packet{
		Type: ping,
		Msgs: []*message{n.s.aliveMessage()},
	}
	n.mu.Unlock()
	b := n.encode(p, []net.Addr{nil})
	if _, err := n.conn.WriteTo(b, addr); err != nil {
		// TODO: better error handling
		panic(err)
	}
}

func (n *Node) send(ps ...packet) {
	for _, p := range ps {
		dst, addrs := n.getAddrs(p)
		b := n.encode(p, addrs)
		if _, err := n.conn.WriteTo(b, dst); err != nil {
			// TODO: better error handling
			panic(err)
		}
	}
}

func (n *Node) getAddrs(p packet) (dst net.Addr, addrs []net.Addr) {
	n.mu.Lock()
	defer n.mu.Unlock()
	addrs = make([]net.Addr, len(p.Msgs))
	for i, m := range p.Msgs {
		addrs[i] = n.addrs[m.id]
	}
	dst = n.addrs[p.remoteID]
	return
}

func (n *Node) runReceive() {
	for {
		b := make([]byte, 1<<16)
		size, addr, err := n.conn.ReadFrom(b)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		p, addrs, ok := n.decode(b[:size])
		if !ok {
			continue
		}
		ps, us := n.receive(p, addr, addrs)
		for _, u := range us {
			id := id(u.ID)
			n.mu.Lock()
			u.Addr = n.addrs[id]
			if !u.IsMember {
				delete(n.addrs, id)
			}
			n.mu.Unlock()
			n.ch.Send() <- u
		}
		n.send(ps...)
	}
}

func (n *Node) receive(p packet, src net.Addr, addrs []net.Addr) ([]packet, []Update) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.addrs[p.remoteID] == nil {
		// First contact from sender
		n.addrs[p.remoteID] = src
		n.ch.Send() <- Update{src, string(p.remoteID), true}
	}
	// Update address records
	for i, addr := range addrs {
		id := p.Msgs[i].id
		if id == n.id || addr == nil {
			continue
		}
		n.addrs[id] = addr
	}
	return n.s.receive(p)
}

// Updates returns a channel from which Updates can be received.
func (n *Node) Updates() <-chan Update {
	return n.ch.Receive()
}

// ID returns the Node's ID on the network.
func (n *Node) ID() string {
	return string(n.id)
}

// LocalAddr returns the local network address, if known. It calls the
// underlying PacketConn's LocalAddr method.
func (n *Node) LocalAddr() net.Addr {
	return n.conn.LocalAddr()
}

type envelope struct {
	FromID id
	P      packet
	Addrs  []string
}

func (n *Node) encode(p packet, addrs []net.Addr) []byte {
	envAddrs := make([]string, len(addrs))
	for i, a := range addrs {
		if a == nil {
			continue
		}
		envAddrs[i] = a.String()
	}
	b, err := json.Marshal(envelope{n.id, p, envAddrs})
	if err != nil {
		panic(err)
	}
	return b
}

func (n *Node) decode(b []byte) (packet, []net.Addr, bool) {
	var e envelope
	err := json.Unmarshal(b, &e)
	addrs := make([]net.Addr, len(e.Addrs))
	for i, s := range e.Addrs {
		if s == "" {
			continue
		}
		a, err := net.ResolveUDPAddr("udp", s)
		if err != nil {
			// TODO: better error handling
			panic(err)
		}
		addrs[i] = a
	}
	e.P.remoteID = e.FromID
	return e.P, addrs, err == nil
}

func stoppedTimer() *time.Timer {
	t := time.NewTimer(0)
	if !t.Stop() {
		<-t.C
	}
	return t
}
