package swim

import (
	"encoding/json"
	"net"
	"time"

	"swim/bufchan"
)

const (
	tickPeriod  = time.Second
	pingTimeout = 200 * time.Millisecond
)

type Update struct {
	Addr     net.Addr
	ID       string
	IsMember bool
}

type Driver struct {
	s     *stateMachine
	addrs map[id]net.Addr
	id    id // copy of s.id
	conn  net.PacketConn
	ch    bufchan.Chan[Update]
}

func Open() (*Driver, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil, err
	}
	s := newStateMachine()
	d := &Driver{
		s:     s,
		addrs: make(map[id]net.Addr),
		id:    s.id,
		conn:  conn,
		ch:    bufchan.Make[Update](),
	}
	go d.runReceive()
	go d.runTick()
	return d, nil
}

func (d *Driver) runTick() {
	ticker := time.NewTicker(tickPeriod)
	defer ticker.Stop()
	pingTimer := stoppedTimer()
	for {
		select {
		case <-ticker.C:
			pingTimer.Reset(pingTimeout)
			ps, us := d.s.tick()
			for _, u := range us {
				id := id(u.ID)
				u.Addr = d.addrs[id]
				delete(d.addrs, id)
				d.ch.Send() <- u
			}
			d.send(ps...)
		case <-pingTimer.C:
			d.send(d.s.timeout()...)
		}
	}
}

func (d *Driver) SendHello(addr net.Addr) {
	p := packet{
		Type: ping,
		Msgs: []*message{d.s.aliveMessage()},
	}
	b := d.encode(p, []net.Addr{nil})
	if _, err := d.conn.WriteTo(b, addr); err != nil {
		// TODO: better error handling
		panic(err)
	}
}

func (d *Driver) send(ps ...packet) {
	for _, p := range ps {
		addrs := make([]net.Addr, len(p.Msgs))
		for i, m := range p.Msgs {
			addrs[i] = d.addrs[m.id]
		}
		b := d.encode(p, addrs)
		if _, err := d.conn.WriteTo(b, d.addrs[p.remoteID]); err != nil {
			// TODO: better error handling
			panic(err)
		}
	}
}

func (d *Driver) runReceive() {
	for {
		b := make([]byte, 1<<16)
		n, addr, err := d.conn.ReadFrom(b)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		p, addrs, ok := d.decode(b[:n])
		if !ok {
			continue
		}
		if d.addrs[p.remoteID] == nil {
			// First contact from sender
			d.addrs[p.remoteID] = addr
			d.ch.Send() <- Update{addr, string(p.remoteID), true}
		}
		// Update address records
		for i, addr := range addrs {
			id := p.Msgs[i].id
			if id == d.id || addr == nil {
				continue
			}
			d.addrs[id] = addr
		}

		ps, us := d.s.receive(p)
		for _, u := range us {
			id := id(u.ID)
			u.Addr = d.addrs[id]
			if !u.IsMember {
				delete(d.addrs, id)
			}
			d.ch.Send() <- u
		}
		d.send(ps...)
	}
}

func (d *Driver) Updates() <-chan Update {
	return d.ch.Receive()
}

func (d *Driver) ID() string {
	return string(d.id)
}

func (d *Driver) LocalAddr() net.Addr {
	return d.conn.LocalAddr()
}

type envelope struct {
	FromID id
	P      packet
	Addrs  []net.Addr
}

func (d *Driver) encode(p packet, addrs []net.Addr) []byte {
	b, err := json.Marshal(envelope{d.id, p, addrs})
	if err != nil {
		panic(err)
	}
	return b
}

func (d *Driver) decode(b []byte) (packet, []net.Addr, bool) {
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
