package swim

import (
	"encoding/json"
	"errors"
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

// A Node is a network node participating in the SWIM protocol.
type Node struct {
	mu  sync.Mutex // protects the following fields
	fsm *stateMachine

	id      id // copy of fsm.id
	conn    *net.UDPConn
	updates bufchan.Chan[Update]
}

// Start creates a new Node and starts running the SWIM protocol on it.
func Start() (*Node, error) {
	addr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", addr)
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
func (n *Node) Join(remote netip.AddrPort) {
	n.mu.Lock()
	p := packet{
		Type: ping,
		Msgs: []*message{n.fsm.aliveMessage()},
	}
	n.mu.Unlock()
	b := encode(n.id, p, []netip.AddrPort{{}})
	if _, err := n.conn.WriteToUDPAddrPort(b, remote); err != nil {
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
		if dst == (netip.AddrPort{}) {
			continue
		}
		b := encode(n.id, p, addrs)
		if _, err := n.conn.WriteToUDPAddrPort(b, dst); err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// TODO: better error handling
			panic(err)
		}
	}
}

func (n *Node) getAddrs(p packet) (dst netip.AddrPort, addrs []netip.AddrPort) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.fsm.getAddrs(p)
}

func (n *Node) runReceive() {
	for {
		b := make([]byte, 1<<16)
		len, addr, err := n.conn.ReadFromUDPAddrPort(b)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}

		p, addrs, ok := decode(b[:len])
		if !ok {
			continue
		}
		ps, us := n.receive(p, addr, addrs)
		n.sendUpdates(us)
		n.send(ps)
	}
}

func (n *Node) receive(p packet, src netip.AddrPort, addrs []netip.AddrPort) ([]packet, []Update) {
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

// LocalAddr returns the local network address.
func (n *Node) LocalAddr() netip.AddrPort {
	return n.conn.LocalAddr().(*net.UDPAddr).AddrPort()
}

type envelope struct {
	FromID id
	P      packet
	Addrs  []netip.AddrPort
}

func encode(id id, p packet, addrs []netip.AddrPort) []byte {
	b, err := json.Marshal(envelope{id, p, addrs})
	if err != nil {
		panic(err)
	}
	return b
}

func decode(b []byte) (packet, []netip.AddrPort, bool) {
	var e envelope
	err := json.Unmarshal(b, &e)
	e.P.remoteID = e.FromID
	return e.P, e.Addrs, err == nil
}

func stoppedTimer() *time.Timer {
	t := time.NewTimer(0)
	if !t.Stop() {
		<-t.C
	}
	return t
}
