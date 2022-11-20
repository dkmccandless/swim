package swim

import (
	"net"
	"net/netip"
	"testing"

	"kr.dev/diff"
)

type update struct {
	typ    updateType
	nodeID string
	memo   []byte
}

type updateType byte

const (
	joinedUpdate updateType = iota
	sentMemoUpdate
	failedUpdate
)

func TestOnJoin(t *testing.T) {
	n0, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	addr0 := n0.localAddrPort()

	met1 := make(chan string)
	n0.OnJoin(func(id string, _ netip.AddrPort) {
		met1 <- id
	})

	n1, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	n1.Join(addr0)
	diff.Test(t, t.Errorf, <-met1, n1.ID())

	type record struct {
		id  string
		met bool
	}
	met2 := make(chan record)
	n0.OnJoin(func(id string, _ netip.AddrPort) {
		met2 <- record{id, true}
	})

	n2, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	n2.Join(addr0)
	diff.Test(t, t.Errorf, (<-met2).id, n2.ID())
}

func TestHandlerOrder(t *testing.T) {
	n, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan updateType)
	n.OnFail(func(string) { ch <- failedUpdate })
	n.OnJoin(func(string, netip.AddrPort) { ch <- joinedUpdate })
	n.OnMemo(func(string, netip.AddrPort, []byte) { ch <- sentMemoUpdate })

	// n receives a memo from an unknown source
	n.receive(packet{
		Type:     ping,
		remoteID: "XYZ",
		Msgs: []*message{
			{
				Type:   alive,
				NodeID: "XYZ",
				MemoID: "123",
				Body:   []byte("Hello, SWIM!"),
			},
		},
	})

	diff.Test(t, t.Errorf, <-ch, joinedUpdate)
	diff.Test(t, t.Errorf, <-ch, sentMemoUpdate)
	diff.Test(t, t.Errorf, <-ch, failedUpdate)
}

func TestDetectJoinAndFail(t *testing.T) {
	nodes, chans := launch(2)
	addr0 := nodes[0].localAddrPort()
	makeUpdate := func(typ updateType, n int) update {
		return update{typ: typ, nodeID: string(nodes[n].id)}
	}
	nodes[1].Join(addr0)
	diff.Test(t, t.Errorf, <-chans[0], makeUpdate(joinedUpdate, 1))
	diff.Test(t, t.Errorf, <-chans[1], makeUpdate(joinedUpdate, 0))

	n2, ch2 := launch(1)
	nodes = append(nodes, n2...)
	chans = append(chans, ch2...)
	nodes[2].Join(addr0)
	diff.Test(t, t.Errorf, <-chans[0], makeUpdate(joinedUpdate, 2))
	diff.Test(t, t.Errorf, <-chans[1], makeUpdate(joinedUpdate, 2))

	// Node 2's updates may arrive in either order
	updates2 := make(map[id]update)
	for i := 0; i < 2; i++ {
		u := <-chans[2]
		updates2[id(u.nodeID)] = u
	}
	diff.Test(t, t.Errorf, updates2[nodes[0].id], makeUpdate(joinedUpdate, 0))
	diff.Test(t, t.Errorf, updates2[nodes[1].id], makeUpdate(joinedUpdate, 1))

	nodes[0].conn.Close()
	diff.Test(t, t.Errorf, <-chans[1], makeUpdate(failedUpdate, 0))
	diff.Test(t, t.Errorf, <-chans[2], makeUpdate(failedUpdate, 0))

	nodes[2].conn.Close()
	diff.Test(t, t.Errorf, <-chans[1], makeUpdate(failedUpdate, 2))
}

func TestPostMemo(t *testing.T) {
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
	diff.Test(t, t.Errorf, <-chans[1], u)
	diff.Test(t, t.Errorf, <-chans[2], u)
	nodes[1].PostMemo([]byte(s))
	u = update{typ: sentMemoUpdate, nodeID: string(nodes[1].id), memo: []byte(s)}
	diff.Test(t, t.Errorf, <-chans[0], u)
	diff.Test(t, t.Errorf, <-chans[2], u)
}

func launch(n int) ([]*Node, []chan update) {
	nodes := make([]*Node, n)
	chans := make([]chan update, n)
	for i := range nodes {
		i := i
		chans[i] = make(chan update)
		n, err := Start()
		if err != nil {
			panic(err)
		}
		n.OnJoin(func(id string, _ netip.AddrPort) {
			chans[i] <- update{typ: joinedUpdate, nodeID: id}
		})
		n.OnMemo(func(id string, _ netip.AddrPort, memo []byte) {
			chans[i] <- update{typ: sentMemoUpdate, nodeID: id, memo: memo}
		})
		n.OnFail(func(id string) {
			chans[i] <- update{typ: failedUpdate, nodeID: id}
		})
		nodes[i] = n
	}
	return nodes, chans
}

func (n *Node) localAddrPort() netip.AddrPort {
	u := *n.conn.LocalAddr().(*net.UDPAddr)
	u.IP = net.IPv6loopback
	return u.AddrPort()
}
