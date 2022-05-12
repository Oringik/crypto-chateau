package crypto_chateau

import "net"

type Peer struct {
	conn net.Conn
}

func NewPeer(conn net.Conn) *Peer {
	return &Peer{
		conn: conn,
	}
}

func (p *Peer) WriteResponse(msg Message) error {
	bytesMsg, err := msg.Marshal()
	if err != nil {
		return err
	}

	_, err = p.conn.Write(bytesMsg)
	return err
}

func (p *Peer) WriteError(err error) error {
	msg := "error: " + err.Error()

	_, writeErr := p.conn.Write([]byte(msg))

	return writeErr
}

func (p *Peer) Write(data []byte) (int, error) {
	n, err := p.conn.Write(data)

	return n, err
}

func (p *Peer) Read(b []byte) (int, error) {
	n, err := p.conn.Read(b)

	return n, err
}

func (p *Peer) Close() error {
	err := p.conn.Close()

	return err
}
