package swim

import (
	"net"
	"net/netip"
	"testing"

	"kr.dev/diff"
)

func TestDetectJoinAndFail(t *testing.T) {
	opt := diff.ZeroFields[Update]("Addr")
	nodes := launch(2)
	addr0 := nodes[0].localAddrPort()
	update := func(typ UpdateType, n int) Update {
		return Update{Type: typ, NodeID: string(nodes[n].id)}
	}
	nodes[1].Join(addr0)
	diff.Test(t, t.Errorf, <-nodes[0].Updates(), update(Joined, 1), opt)
	diff.Test(t, t.Errorf, <-nodes[1].Updates(), update(Joined, 0), opt)

	nodes = append(nodes, launch(1)...)
	nodes[2].Join(addr0)
	diff.Test(t, t.Errorf, <-nodes[0].Updates(), update(Joined, 2), opt)
	diff.Test(t, t.Errorf, <-nodes[1].Updates(), update(Joined, 2), opt)

	// Node 2's updates may arrive in either order
	updates2 := make(map[id]Update)
	for i := 0; i < 2; i++ {
		u := <-nodes[2].Updates()
		updates2[id(u.NodeID)] = u
	}
	want2 := map[id]Update{
		nodes[0].id: update(Joined, 0),
		nodes[1].id: update(Joined, 1),
	}
	diff.Test(t, t.Errorf, updates2, want2, opt)

	nodes[0].conn.Close()
	diff.Test(t, t.Errorf, <-nodes[1].Updates(), update(Failed, 0), opt)
	diff.Test(t, t.Errorf, <-nodes[2].Updates(), update(Failed, 0), opt)

	nodes[2].conn.Close()
	diff.Test(t, t.Errorf, <-nodes[1].Updates(), update(Failed, 2), opt)
}

func TestPostMemo(t *testing.T) {
	opt := diff.ZeroFields[Update]("Addr")
	nodes := launch(3)
	addr0 := nodes[0].localAddrPort()
	nodes[1].Join(addr0)
	nodes[2].Join(addr0)
	<-nodes[0].Updates()
	<-nodes[0].Updates()
	<-nodes[1].Updates()
	<-nodes[1].Updates()
	<-nodes[2].Updates()
	<-nodes[2].Updates()

	s := "Hello, SWIM!"
	nodes[0].PostMemo([]byte(s))
	u := Update{Type: SentMemo, NodeID: string(nodes[0].id), Memo: []byte(s)}
	diff.Test(t, t.Errorf, <-nodes[1].Updates(), u, opt)
	diff.Test(t, t.Errorf, <-nodes[2].Updates(), u, opt)
	nodes[1].PostMemo([]byte(s))
	u = Update{Type: SentMemo, NodeID: string(nodes[1].id), Memo: []byte(s)}
	diff.Test(t, t.Errorf, <-nodes[0].Updates(), u, opt)
	diff.Test(t, t.Errorf, <-nodes[2].Updates(), u, opt)
}

func launch(n int) []*Node {
	nodes := make([]*Node, n)
	for i := range nodes {
		node, err := Start()
		if err != nil {
			panic(err)
		}
		nodes[i] = node
	}
	return nodes
}

func (n *Node) localAddrPort() netip.AddrPort {
	u := *n.conn.LocalAddr().(*net.UDPAddr)
	u.IP = net.IPv6loopback
	return u.AddrPort()
}
