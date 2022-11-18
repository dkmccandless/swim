package swim

import (
	"net"
	"net/netip"
	"testing"

	"kr.dev/diff"
)

// An update carries network membership information or user-defined data.
type update struct {
	// typ describes the meaning of the Update.
	typ updateType

	// nodeID is the ID of the source node.
	nodeID string

	// Addr is the address of the source node.
	Addr netip.AddrPort

	// memo carries user-defined data sent by nodeID.
	memo []byte
}

// An updateType describes the meaning of an Update.
type updateType byte

const (
	// joinedUpdate indicates that a node has joinedUpdate the network.
	joinedUpdate updateType = iota

	// sentMemoUpdate indicates that a node has sent a memo.
	sentMemoUpdate

	// failedUpdate indicates that a node has left the network.
	failedUpdate
)

func TestDetectJoinAndFail(t *testing.T) {
	opt := diff.ZeroFields[update]("Addr")
	nodes, chans := launch(2)
	addr0 := nodes[0].localAddrPort()
	makeUpdate := func(typ updateType, n int) update {
		return update{typ: typ, nodeID: string(nodes[n].id)}
	}
	nodes[1].Join(addr0)
	diff.Test(t, t.Errorf, <-chans[0], makeUpdate(joinedUpdate, 1), opt)
	diff.Test(t, t.Errorf, <-chans[1], makeUpdate(joinedUpdate, 0), opt)

	n2, ch2 := launch(1)
	nodes = append(nodes, n2...)
	chans = append(chans, ch2...)
	nodes[2].Join(addr0)
	diff.Test(t, t.Errorf, <-chans[0], makeUpdate(joinedUpdate, 2), opt)
	diff.Test(t, t.Errorf, <-chans[1], makeUpdate(joinedUpdate, 2), opt)

	// Node 2's updates may arrive in either order
	updates2 := make(map[id]update)
	for i := 0; i < 2; i++ {
		u := <-chans[2]
		updates2[id(u.nodeID)] = u
	}
	want2 := map[id]update{
		nodes[0].id: makeUpdate(joinedUpdate, 0),
		nodes[1].id: makeUpdate(joinedUpdate, 1),
	}
	diff.Test(t, t.Errorf, updates2, want2, opt)

	nodes[0].conn.Close()
	diff.Test(t, t.Errorf, <-chans[1], makeUpdate(failedUpdate, 0), opt)
	diff.Test(t, t.Errorf, <-chans[2], makeUpdate(failedUpdate, 0), opt)

	nodes[2].conn.Close()
	diff.Test(t, t.Errorf, <-chans[1], makeUpdate(failedUpdate, 2), opt)
}

func TestPostMemo(t *testing.T) {
	opt := diff.ZeroFields[update]("Addr")
	nodes, chans := launch(3)
	addr0 := nodes[0].localAddrPort()
	nodes[1].Join(addr0)
	nodes[2].Join(addr0)
	<-chans[0]
	<-chans[0]
	<-chans[1]
	<-chans[1]
	<-chans[2]
	<-chans[2]

	s := "Hello, SWIM!"
	nodes[0].PostMemo([]byte(s))
	u := update{typ: sentMemoUpdate, nodeID: string(nodes[0].id), memo: []byte(s)}
	diff.Test(t, t.Errorf, <-chans[1], u, opt)
	diff.Test(t, t.Errorf, <-chans[2], u, opt)
	nodes[1].PostMemo([]byte(s))
	u = update{typ: sentMemoUpdate, nodeID: string(nodes[1].id), memo: []byte(s)}
	diff.Test(t, t.Errorf, <-chans[0], u, opt)
	diff.Test(t, t.Errorf, <-chans[2], u, opt)
}

func launch(n int) ([]*Node, []chan update) {
	nodes := make([]*Node, n)
	chans := make([]chan update, n)
	for i := range nodes {
		i := i
		chans[i] = make(chan update)
		node, err := Start(
			func(id string, addr netip.AddrPort) {
				chans[i] <- update{typ: joinedUpdate, nodeID: id, Addr: addr}
			},
			func(id string, addr netip.AddrPort, memo []byte) {
				chans[i] <- update{typ: sentMemoUpdate, nodeID: id, Addr: addr, memo: memo}
			},
			func(id string) {
				chans[i] <- update{typ: failedUpdate, nodeID: id}
			},
		)
		if err != nil {
			panic(err)
		}
		nodes[i] = node
	}
	return nodes, chans
}

func (n *Node) localAddrPort() netip.AddrPort {
	u := *n.conn.LocalAddr().(*net.UDPAddr)
	u.IP = net.IPv6loopback
	return u.AddrPort()
}
