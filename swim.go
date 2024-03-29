package swim

import (
	"encoding/json"
	"errors"
	"math/rand"
	"net"
	"net/netip"
	"sync"
	"time"
)

const (
	tickAverage = time.Second
	pingTimeout = 200 * time.Millisecond
)

// A Node is a network node participating in the SWIM protocol.
type Node struct {
	mu         sync.Mutex // protects the following fields
	fsm        *stateMachine
	handleJoin func(id string, addr netip.AddrPort)
	handleMemo func(id string, addr netip.AddrPort, memo []byte)
	handleFail func(id string)

	id       id // copy of fsm.id
	conn     *net.UDPConn
	stopTick chan struct{}
}

// Start creates a new Node listening on the local UDP address.
//
// If the address's host is empty or a literal unspecified IP address, the
// Node listens on all available IP addresses of the local system except
// multicast IP addresses. If the port is empty or "0", as in "127.0.0.1:"
// or "[::1]:0", a port number is automatically chosen.
func Start(address string) (*Node, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	n := &Node{
		handleJoin: func(string, netip.AddrPort) {},
		handleMemo: func(string, netip.AddrPort, []byte) {},
		handleFail: func(string) {},

		conn:     conn,
		stopTick: make(chan struct{}),
	}

	wgs := make(map[id]*struct{ join, memo sync.WaitGroup })
	n.fsm = newStateMachine(
		func(id id, addr netip.AddrPort) {
			wg := &struct{ join, memo sync.WaitGroup }{}
			wgs[id] = wg
			wg.join.Add(1)
			go func() {
				defer wg.join.Done()
				n.handleJoin(string(id), addr)
			}()
		},
		func(id id, addr netip.AddrPort, memo []byte) {
			wg := wgs[id]
			wg.memo.Add(1)
			go func() {
				defer wg.memo.Done()
				wg.join.Wait()
				n.handleMemo(string(id), addr, memo)
			}()
		},
		func(id id) {
			wg := wgs[id]
			delete(wgs, id)
			go func() {
				wg.memo.Wait()
				n.handleFail(string(id))
			}()
		},
	)
	n.id = n.fsm.id
	go n.runReceive()
	go n.runTick()
	return n, nil
}

// OnJoin uses f as n's join handler, to be called when a peer joins the
// network.
func (n *Node) OnJoin(f func(nodeID string, addr netip.AddrPort)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handleJoin = f
}

// OnMemo uses f as n's memo handler, to be called when n receives a memo.
// For each peer, calls to f happen after the join handler (if any) returns.
func (n *Node) OnMemo(f func(nodeID string, addr netip.AddrPort, memo []byte)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handleMemo = f
}

// OnFail uses f as n's failure handler, to be called when a peer leaves the
// network. For each peer, the call to f happens after all calls to the memo
// handler (if any) return.
func (n *Node) OnFail(f func(nodeID string)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handleFail = f
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
		e.P.remoteID = e.SrcID
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

// PostMemo disseminates a memo throughout the network. To ensure transmission
// within a single UDP packet, PostMemo enforces a length limit of 500 bytes;
// if len(b) exceeds this, PostMemo returns an error instead.
func (n *Node) PostMemo(b []byte) error {
	if len(b) > 500 {
		return errors.New("body too long")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.fsm.addMemo(b)
	return nil
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
	SrcID id
	P     packet
}

func stoppedTimer() *time.Timer {
	t := time.NewTimer(0)
	if !t.Stop() {
		<-t.C
	}
	return t
}
