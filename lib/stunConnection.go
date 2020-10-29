package turner

import (
	"fmt"
	"io"
	"net"
	"time"

	"gortc.io/turnc"
)

type StunConnection struct {
	Conn       net.Conn
	MultiRead  io.Reader
	CntrClient turnc.Client
	DataClient turnc.Client
}

// Read data from peer.
func (c *StunConnection) Read(b []byte) (n int, err error) {
	return c.MultiRead.Read(b)
}

func (c *StunConnection) Write(b []byte) (n int, err error) {
	return c.Conn.Write(b)
}

// Close stops all refreshing loops for permission and removes it from
// allocation.
func (c *StunConnection) Close() error {
	fmt.Println("[*] Shut it all down")
	c.CntrClient.Close()
	c.DataClient.Close()
	return c.Conn.Close()
}

// LocalAddr is relayed address from TURN server.
func (c *StunConnection) LocalAddr() net.Addr {
	return c.Conn.LocalAddr()
}

// RemoteAddr is peer address.
func (c *StunConnection) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

// SetDeadline implements net.Conn.
func (c *StunConnection) SetDeadline(t time.Time) error {
	return c.Conn.SetDeadline(t)
}

// SetReadD
func (c *StunConnection) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *StunConnection) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}
