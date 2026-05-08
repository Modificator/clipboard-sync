package relay

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/Modificator/clipboard-sync/internal/protocol"
)

type Server struct {
	logger       *log.Logger
	listener     net.Listener
	requireToken string
	roomsMu      sync.RWMutex
	rooms        map[string]*roomState
}

type roomState struct {
	mu      sync.RWMutex
	seq     uint64
	latest  map[string]protocol.ClipboardUpdate
	clients map[string]*clientConn
}

type clientConn struct {
	id     string
	name   string
	room   string
	conn   net.Conn
	writer *bufio.Writer
	mu     sync.Mutex
}

func Listen(ctx context.Context, address, certFile, keyFile, requireToken string, logger *log.Logger) error {
	if logger == nil {
		logger = log.Default()
	}

	ln, err := listen(address, certFile, keyFile)
	if err != nil {
		return err
	}
	defer ln.Close()

	s := &Server{
		logger:       logger,
		listener:     ln,
		requireToken: requireToken,
		rooms:        make(map[string]*roomState),
	}

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			logger.Printf("accept error: %v", err)
			continue
		}
		go s.handleConn(ctx, conn)
	}
}

func listen(address, certFile, keyFile string) (net.Listener, error) {
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return nil, fmt.Errorf("both tls cert and key must be provided")
		}
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		return tls.Listen("tcp", address, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})
	}
	return net.Listen("tcp", address)
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	helloEnvelope, err := readEnvelope(reader)
	if err != nil {
		s.logger.Printf("failed to read hello: %v", err)
		return
	}
	if helloEnvelope.Type != protocol.TypeHello || helloEnvelope.Hello == nil {
		_ = writeEnvelope(writer, protocol.Envelope{Type: protocol.TypeError, Error: &protocol.ErrorMessage{Message: "expected hello message"}})
		return
	}
	hello := helloEnvelope.Hello
	if err := s.validateHello(*hello); err != nil {
		_ = writeEnvelope(writer, protocol.Envelope{Type: protocol.TypeError, Error: &protocol.ErrorMessage{Message: err.Error()}})
		return
	}

	client := &clientConn{id: hello.DeviceID, name: hello.DeviceName, room: hello.Room, conn: conn, writer: writer}
	room := s.joinRoom(client)
	defer s.leaveRoom(client)

	if err := client.send(protocol.Envelope{Type: protocol.TypeHelloAck, Ack: &protocol.HelloAck{ServerTime: time.Now().UTC(), Message: "connected"}}); err != nil {
		s.logger.Printf("failed to send hello ack: %v", err)
		return
	}

	if latest := room.snapshotLatest(); len(latest) > 0 {
		for _, update := range latest {
			msg := update
			if err := client.send(protocol.Envelope{Type: protocol.TypeUpdate, Update: &msg}); err != nil {
				return
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		envelope, err := readEnvelope(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.logger.Printf("read error from %s: %v", client.id, err)
			}
			return
		}
		if envelope.Type != protocol.TypeUpdate || envelope.Update == nil {
			_ = client.send(protocol.Envelope{Type: protocol.TypeError, Error: &protocol.ErrorMessage{Message: "unsupported message type"}})
			continue
		}
		update := *envelope.Update
		if err := validateUpdate(client.room, client.id, update); err != nil {
			_ = client.send(protocol.Envelope{Type: protocol.TypeError, Error: &protocol.ErrorMessage{Message: err.Error()}})
			continue
		}
		room.broadcast(client, update)
	}
}

func (s *Server) validateHello(hello protocol.Hello) error {
	if hello.DeviceID == "" {
		return fmt.Errorf("device_id is required")
	}
	if hello.Room == "" {
		return fmt.Errorf("room is required")
	}
	if hello.Token != s.requireToken {
		return fmt.Errorf("authentication failed")
	}
	return nil
}

func validateUpdate(room, deviceID string, update protocol.ClipboardUpdate) error {
	if update.Room != room {
		return fmt.Errorf("room mismatch")
	}
	if update.DeviceID != deviceID {
		return fmt.Errorf("device_id mismatch")
	}
	if update.Kind != protocol.KindText && update.Kind != protocol.KindImage {
		return fmt.Errorf("unsupported clipboard kind")
	}
	if update.Hash == "" {
		return fmt.Errorf("hash is required")
	}
	return nil
}

func (s *Server) joinRoom(client *clientConn) *roomState {
	s.roomsMu.Lock()
	defer s.roomsMu.Unlock()
	room := s.rooms[client.room]
	if room == nil {
		room = &roomState{latest: make(map[string]protocol.ClipboardUpdate), clients: make(map[string]*clientConn)}
		s.rooms[client.room] = room
	}
	room.mu.Lock()
	room.clients[client.id] = client
	room.mu.Unlock()
	s.logger.Printf("device %s joined room %s", client.id, client.room)
	return room
}

func (s *Server) leaveRoom(client *clientConn) {
	s.roomsMu.Lock()
	defer s.roomsMu.Unlock()
	room := s.rooms[client.room]
	if room == nil {
		return
	}
	room.mu.Lock()
	delete(room.clients, client.id)
	empty := len(room.clients) == 0
	room.mu.Unlock()
	if empty {
		delete(s.rooms, client.room)
	}
	s.logger.Printf("device %s left room %s", client.id, client.room)
}

func (r *roomState) snapshotLatest() []protocol.ClipboardUpdate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	updates := make([]protocol.ClipboardUpdate, 0, len(r.latest))
	for _, update := range r.latest {
		updates = append(updates, update)
	}
	return updates
}

func (r *roomState) broadcast(source *clientConn, update protocol.ClipboardUpdate) {
	r.mu.Lock()
	latest, exists := r.latest[update.Kind]
	if exists {
		if latest.Hash == update.Hash {
			r.mu.Unlock()
			return
		}
	}
	r.seq++
	update.ServerSeq = r.seq
	update.Timestamp = update.Timestamp.UTC()
	r.latest[update.Kind] = update
	clients := make([]*clientConn, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.mu.Unlock()

	for _, client := range clients {
		if client.id == source.id {
			continue
		}
		msg := update
		if err := client.send(protocol.Envelope{Type: protocol.TypeUpdate, Update: &msg}); err != nil {
			continue
		}
	}
}

func (c *clientConn) send(envelope protocol.Envelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeEnvelope(c.writer, envelope)
}

func readEnvelope(reader *bufio.Reader) (protocol.Envelope, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return protocol.Envelope{}, err
	}
	var envelope protocol.Envelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return protocol.Envelope{}, err
	}
	return envelope, nil
}

func writeEnvelope(writer *bufio.Writer, envelope protocol.Envelope) error {
	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if _, err := writer.Write(append(payload, '\n')); err != nil {
		return err
	}
	return writer.Flush()
}
