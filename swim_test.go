package swim

import (
	"net"
	"net/netip"
	"testing"

	"kr.dev/diff"
)

// An Update carries network membership information or user-defined data.
type Update struct {
	// Type describes the meaning of the Update.
	Type UpdateType

	// NodeID is the ID of the source node.
	NodeID string

	// Addr is the address of the source node.
	Addr netip.AddrPort

	// Memo carries user-defined data sent by a node identified by NodeID using
	// PostMemo. It is defined if Type is SentMemo, and nil otherwise.
	Memo []byte
}

// An UpdateType describes the meaning of an Update.
type UpdateType byte

const (
	// Joined indicates that a node has joined the network.
	Joined UpdateType = iota

	// SentMemo indicates that a node has sent a memo.
	SentMemo

	// Failed indicates that a node has left the network.
	Failed
)

func TestDetectJoinAndFail(t *testing.T) {
	opt := diff.ZeroFields[Update]("Addr")
	nodes, chans := launch(2)
	addr0 := nodes[0].localAddrPort()
	update := func(typ UpdateType, n int) Update {
		return Update{Type: typ, NodeID: string(nodes[n].id)}
	}
	nodes[1].Join(addr0)
	diff.Test(t, t.Errorf, <-chans[0], update(Joined, 1), opt)
	diff.Test(t, t.Errorf, <-chans[1], update(Joined, 0), opt)

	n2, ch2 := launch(1)
	nodes = append(nodes, n2...)
	chans = append(chans, ch2...)
	nodes[2].Join(addr0)
	diff.Test(t, t.Errorf, <-chans[0], update(Joined, 2), opt)
	diff.Test(t, t.Errorf, <-chans[1], update(Joined, 2), opt)

	// Node 2's updates may arrive in either order
	updates2 := make(map[id]Update)
	for i := 0; i < 2; i++ {
		u := <-chans[2]
		updates2[id(u.NodeID)] = u
	}
	want2 := map[id]Update{
		nodes[0].id: update(Joined, 0),
		nodes[1].id: update(Joined, 1),
	}
	diff.Test(t, t.Errorf, updates2, want2, opt)

	nodes[0].conn.Close()
	diff.Test(t, t.Errorf, <-chans[1], update(Failed, 0), opt)
	diff.Test(t, t.Errorf, <-chans[2], update(Failed, 0), opt)

	nodes[2].conn.Close()
	diff.Test(t, t.Errorf, <-chans[1], update(Failed, 2), opt)
}

func TestPostMemo(t *testing.T) {
	opt := diff.ZeroFields[Update]("Addr")
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
	u := Update{Type: SentMemo, NodeID: string(nodes[0].id), Memo: []byte(s)}
	diff.Test(t, t.Errorf, <-chans[1], u, opt)
	diff.Test(t, t.Errorf, <-chans[2], u, opt)
	nodes[1].PostMemo([]byte(s))
	u = Update{Type: SentMemo, NodeID: string(nodes[1].id), Memo: []byte(s)}
	diff.Test(t, t.Errorf, <-chans[0], u, opt)
	diff.Test(t, t.Errorf, <-chans[2], u, opt)
}

func launch(n int) ([]*Node, []chan Update) {
	nodes := make([]*Node, n)
	chans := make([]chan Update, n)
	for i := range nodes {
		i := i
		chans[i] = make(chan Update)
		node, err := Start(
			func(id string, addr netip.AddrPort) {
				chans[i] <- Update{Type: Joined, NodeID: id, Addr: addr}
			},
			func(id string, addr netip.AddrPort, memo []byte) {
				chans[i] <- Update{Type: SentMemo, NodeID: id, Addr: addr, Memo: memo}
			},
			func(id string) {
				chans[i] <- Update{Type: Failed, NodeID: id}
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
