package swim

import (
	"encoding/json"
	"net"
	"time"
)

const (
	tickPeriod  = time.Second
	pingTimeout = 200 * time.Millisecond
)

type Driver struct {
	s     *stateMachine
	conn  net.PacketConn
	addrs map[id]net.Addr
}

func Open() (*Driver, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil, err
	}
	d := &Driver{
		s:     newStateMachine(),
		conn:  conn,
		addrs: make(map[id]net.Addr),
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
			d.send(d.s.tick()...)
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
	b := d.encode(p)
	if _, err := d.conn.WriteTo(b, addr); err != nil {
		// TODO: better error handling
		panic(err)
	}
}

func (d *Driver) send(ps ...packet) {
	for _, p := range ps {
		addr := d.addrs[p.remoteID]
		b := d.encode(p)
		if _, err := d.conn.WriteTo(b, addr); err != nil {
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
		p, ok := d.decode(b[:n])
		if !ok {
			continue
		}
		d.addrs[p.remoteID] = addr
		d.send(d.s.receive(p)...)
	}
}

func (d *Driver) LocalAddr() net.Addr {
	return d.conn.LocalAddr()
}

type envelope struct {
	FromID id
	P      packet
}

func (d *Driver) encode(p packet) []byte {
	b, err := json.Marshal(envelope{d.s.id, p})
	if err != nil {
		panic(err)
	}
	return b
}

func (d *Driver) decode(b []byte) (packet, bool) {
	var e envelope
	err := json.Unmarshal(b, &e)
	e.P.remoteID = e.FromID
	return e.P, err == nil
}

func stoppedTimer() *time.Timer {
	t := time.NewTimer(0)
	if !t.Stop() {
		<-t.C
	}
	return t
}
