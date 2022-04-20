package swim

import (
	"encoding/json"
	"errors"
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
	ID       string
	IsMember bool
	Addr     net.Addr
}

// A Node is a network node participating in the SWIM protocol.
type Node struct {
	mu  sync.Mutex // protects the following fields
	fsm *stateMachine

	id      id // copy of fsm.id
	conn    net.PacketConn
	updates bufchan.Chan[Update]
}

// Start creates a new Node and starts running the SWIM protocol on it.
func Start() (*Node, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil, err
	}
	fsm := newStateMachine()
	n := &Node{
		fsm:     fsm,
		id:      fsm.id,
		conn:    conn,
		updates: bufchan.Make[Update](),
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
			n.send(n.tick())
		case <-pingTimer.C:
			n.send(n.timeout())
		}
	}
}

func (n *Node) tick() []packet {
	n.mu.Lock()
	ps, us := n.fsm.tick()
	n.mu.Unlock()
	n.sendUpdates(us)
	return ps
}

func (n *Node) timeout() []packet {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.fsm.timeout()
}

// Join connects n to a remote node. This is typically used to connect a new
// node to an existing network.
func (n *Node) Join(remoteAddr net.Addr) {
	n.mu.Lock()
	p := packet{
		Type: ping,
		Msgs: []*message{n.fsm.aliveMessage()},
	}
	n.mu.Unlock()
	b := n.encode(p, []net.Addr{nil})
	if _, err := n.conn.WriteTo(b, remoteAddr); err != nil {
		if errors.Is(err, net.ErrClosed) {
			return
		}
		// TODO: better error handling
		panic(err)
	}
}

func (n *Node) send(ps []packet) {
	for _, p := range ps {
		dst, addrs := n.getAddrs(p)
		if dst == nil {
			continue
		}
		b := n.encode(p, addrs)
		if _, err := n.conn.WriteTo(b, dst); err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// TODO: better error handling
			panic(err)
		}
	}
}

func (n *Node) getAddrs(p packet) (dst net.Addr, addrs []net.Addr) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.fsm.getAddrs(p)
}

func (n *Node) runReceive() {
	for {
		b := make([]byte, 1<<16)
		len, addr, err := n.conn.ReadFrom(b)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		p, addrs, ok := n.decode(b[:len])
		if !ok {
			continue
		}
		ps, us := n.receive(p, addr, addrs)
		n.sendUpdates(us)
		n.send(ps)
	}
}

func (n *Node) receive(p packet, src net.Addr, addrs []net.Addr) ([]packet, []Update) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.fsm.receive(p, src, addrs)
}

func (n *Node) sendUpdates(us []Update) {
	for _, u := range us {
		n.updates.Send() <- u
	}
}

// Updates returns a channel from which Updates can be received.
func (n *Node) Updates() <-chan Update {
	return n.updates.Receive()
}

// ID returns n's ID on the network.
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
