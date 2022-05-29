package crypto_chateau

import (
	"bufio"
	"context"
	"errors"
	"github.com/Oringik/crypto-chateau/generated"
	"github.com/Oringik/crypto-chateau/transport"
	"log"
	"net"
	"sync"
)

const (
	msgDelim = '\n'
)

type handlerFunc func(context.Context, Message) (Message, error)
type streamFunc func(context.Context, Message, *Peer) (Message, error)

type Server struct {
	Config   *Config
	Handlers map[string]*Handler
	// key: ip address  value: client peer
	Clients    map[string]*Peer
	shutdownCh chan struct{}
}

type Config struct {
	IP   string
	Port string
}

func NewServer(cfg *Config) *Server {
	return &Server{
		Config:     cfg,
		Handlers:   make(map[string]*Handler),
		Clients:    make(map[string]*Peer),
		shutdownCh: make(chan struct{}),
	}
}

func (s *Server) Run(ctx context.Context, endpoint generated.Endpoint) error {
	_, err := net.ResolveTCPAddr("tcp", s.Config.IP+":"+s.Config.Port)
	if err != nil {
		return err
	}

	initHandlers(endpoint, s.Handlers)

	wg := sync.WaitGroup{}

	wg.Add(1)

	clientCh := make(chan *Peer)

	go func() {
		s.listenClients(ctx, clientCh)
		wg.Done()
	}()

	s.handleRequests(ctx, clientCh)

	wg.Wait()

	return nil
}

func (s *Server) handleRequests(ctx context.Context, clientChan <-chan *Peer) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-clientChan:
			go s.handleRequest(ctx, client)
		default:
			continue
		}
	}
}

func (s *Server) handleRequest(ctx context.Context, peer *Peer) {
	defer peer.Close()

	securedConnect, err := transport.ClientHandshake(ctx, peer.conn)
	if err != nil {
		log.Println(err)
		return
	}

	peer.conn = securedConnect

	err = s.handleMethod(ctx, peer)
	if err != nil {
		log.Println(err)
		return
	}
}

func (s *Server) handleMethod(ctx context.Context, peer *Peer) error {
	bytesMsg, err := bufio.NewReader(peer).ReadBytes(msgDelim)
	if err != nil {
		return err
	}

	handlerName, n, err := GetHandlerName(bytesMsg)
	if err != nil {
		return err
	}

	handler, ok := s.Handlers[string(handlerName)]
	if !ok {
		return errors.New("unknown handler " + string(handlerName))
	}

	if n >= len(bytesMsg) {
		return errors.New("incorrect message")
	}

	requestMsg, err := ParseMessage(bytesMsg, handler.requestMsgType)
	if err != nil {
		return err
	}

	switch handler.callFunc.(type) {
	case handlerFunc:
		fnc := handler.callFunc.(handlerFunc)
		responseMsg, err := fnc(ctx, requestMsg)
		if err != nil {
			writeErr := peer.WriteError(err)
			return writeErr
		}

		err = peer.WriteResponse(responseMsg)
		if err != nil {
			return err
		}
	case streamFunc:
		fnc := handler.callFunc.(streamFunc)
		responseMsg, err := fnc(ctx, requestMsg, peer)
		if err != nil {
			writeErr := peer.WriteError(err)
			return writeErr
		}

		err = peer.WriteResponse(responseMsg)
		if err != nil {
			return err
		}
	default:
		return errors.New("incorrect handler format: InternalError")
	}

	return nil
}

func (s *Server) listenClients(ctx context.Context, clientChan chan<- *Peer) {
	listener, err := net.Listen("tcp", s.Config.IP+":"+s.Config.Port)
	if err != nil {
		log.Println(err)
		s.shutdownCh <- struct{}{}
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue
				}
				log.Println("Failed to accept connection:", err.Error())
			}

			peer := NewPeer(conn)

			clientChan <- peer
		}
	}
}