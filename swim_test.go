package swim

import (
	"net"
	"testing"

	"kr.dev/diff"
)

var opt = diff.ZeroFields[Update]("Addr")

func TestDetectJoinAndFail(t *testing.T) {
	nodes := launch(2)
	addr0 := nodes[0].localAddr()
	update := func(n int, isMember bool) Update {
		return Update{ID: string(nodes[n].id), IsMember: isMember}
	}
	nodes[1].Join(addr0)
	diff.Test(t, t.Errorf, <-nodes[0].Updates(), update(1, true), opt)
	diff.Test(t, t.Errorf, <-nodes[1].Updates(), update(0, true), opt)

	nodes = append(nodes, launch(1)...)
	nodes[2].Join(addr0)
	diff.Test(t, t.Errorf, <-nodes[0].Updates(), update(2, true), opt)
	diff.Test(t, t.Errorf, <-nodes[1].Updates(), update(2, true), opt)

	// Node 2's updates may arrive in either order
	updates2 := make(map[id]Update)
	for i := 0; i < 2; i++ {
		u := <-nodes[2].Updates()
		updates2[id(u.ID)] = u
	}
	want2 := map[id]Update{
		nodes[0].id: update(0, true),
		nodes[1].id: update(1, true),
	}
	diff.Test(t, t.Errorf, updates2, want2, opt)

	nodes[0].conn.Close()
	diff.Test(t, t.Errorf, <-nodes[1].Updates(), update(0, false), opt)
	diff.Test(t, t.Errorf, <-nodes[2].Updates(), update(0, false), opt)

	nodes[2].conn.Close()
	diff.Test(t, t.Errorf, <-nodes[1].Updates(), update(2, false), opt)
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

func (n *Node) localAddr() net.Addr {
	u := *n.LocalAddr().(*net.UDPAddr)
	u.IP = net.IPv6loopback
	return &u
}
