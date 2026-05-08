package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Modificator/clipboard-sync/internal/clipboard"
	"github.com/Modificator/clipboard-sync/internal/config"
	"github.com/Modificator/clipboard-sync/internal/protocol"
)

type state struct {
	mu            sync.Mutex
	lastTextHash  string
	lastImageHash string
}

func main() {
	configPath := flag.String("config", defaultConfigPath(), "path to client config file")
	flag.Parse()

	cfg, err := config.LoadClientConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	if cfg.DeviceName == "" {
		cfg.DeviceName = cfg.DeviceID
	}
	if cfg.TLSEnabled && cfg.TLSSkipVerify {
		log.Printf("warning: tls.skip_verify=true disables certificate validation and should only be used for local testing")
	}

	backend, err := clipboard.NewBackend(cfg.ImageHelperPath)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	for {
		if err := runClient(ctx, cfg, backend); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("connection error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func runClient(ctx context.Context, cfg config.ClientConfig, backend clipboard.Backend) error {
	conn, err := dialServer(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	client := &wireClient{reader: reader, writer: writer}

	hello := protocol.Envelope{Type: protocol.TypeHello, Hello: &protocol.Hello{DeviceID: cfg.DeviceID, DeviceName: cfg.DeviceName, Platform: runtime.GOOS, Room: cfg.Room, Token: cfg.Token}}
	if err := client.send(hello); err != nil {
		return err
	}
	ack, err := client.read()
	if err != nil {
		return err
	}
	if ack.Type == protocol.TypeError && ack.Error != nil {
		return fmt.Errorf("server rejected client: %s", ack.Error.Message)
	}
	if ack.Type != protocol.TypeHelloAck {
		return fmt.Errorf("expected hello ack, got %s", ack.Type)
	}

	st := &state{}
	recvErr := make(chan error, 1)
	go func() {
		recvErr <- receiveLoop(ctx, client, backend, st)
	}()

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	pollCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	initialHashes(pollCtx, backend, cfg, st)
	cancel()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-recvErr:
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			return nil
		case <-ticker.C:
			pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if cfg.EnableText {
				if err := syncText(pollCtx, cfg, backend, client, st); err != nil {
					log.Printf("text sync skipped: %v", err)
				}
			}
			if cfg.EnableImage {
				if err := syncImage(pollCtx, cfg, backend, client, st); err != nil {
					log.Printf("image sync skipped: %v", err)
				}
			}
			cancel()
		}
	}
}

type wireClient struct {
	reader *bufio.Reader
	writer *bufio.Writer
	mu     sync.Mutex
}

func (c *wireClient) send(envelope protocol.Envelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if _, err := c.writer.Write(append(payload, '\n')); err != nil {
		return err
	}
	return c.writer.Flush()
}

func (c *wireClient) read() (protocol.Envelope, error) {
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		return protocol.Envelope{}, err
	}
	var envelope protocol.Envelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return protocol.Envelope{}, err
	}
	return envelope, nil
}

func receiveLoop(ctx context.Context, client *wireClient, backend clipboard.Backend, st *state) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		envelope, err := client.read()
		if err != nil {
			return err
		}
		if envelope.Type == protocol.TypeError && envelope.Error != nil {
			return errors.New(envelope.Error.Message)
		}
		if envelope.Type != protocol.TypeUpdate || envelope.Update == nil {
			continue
		}
		applyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = applyUpdate(applyCtx, backend, *envelope.Update, st)
		cancel()
		if err != nil {
			log.Printf("failed to apply update: %v", err)
		}
	}
}

func initialHashes(ctx context.Context, backend clipboard.Backend, cfg config.ClientConfig, st *state) {
	if cfg.EnableText {
		if item, err := backend.ReadText(ctx); err == nil {
			st.mu.Lock()
			st.lastTextHash = item.Hash
			st.mu.Unlock()
		}
	}
	if cfg.EnableImage {
		if item, err := backend.ReadImage(ctx); err == nil {
			st.mu.Lock()
			st.lastImageHash = item.Hash
			st.mu.Unlock()
		}
	}
}

func syncText(ctx context.Context, cfg config.ClientConfig, backend clipboard.Backend, client *wireClient, st *state) error {
	item, err := backend.ReadText(ctx)
	if err != nil {
		return err
	}
	st.mu.Lock()
	if item.Hash == st.lastTextHash {
		st.mu.Unlock()
		return nil
	}
	st.mu.Unlock()
	if err := client.send(protocol.Envelope{Type: protocol.TypeUpdate, Update: &protocol.ClipboardUpdate{DeviceID: cfg.DeviceID, DeviceName: cfg.DeviceName, Room: cfg.Room, Kind: protocol.KindText, MIMEType: item.MIMEType, Hash: item.Hash, Text: item.Text, Timestamp: time.Now().UTC()}}); err != nil {
		return err
	}
	st.mu.Lock()
	st.lastTextHash = item.Hash
	st.mu.Unlock()
	return nil
}

func syncImage(ctx context.Context, cfg config.ClientConfig, backend clipboard.Backend, client *wireClient, st *state) error {
	item, err := backend.ReadImage(ctx)
	if err != nil {
		return err
	}
	st.mu.Lock()
	if item.Hash == st.lastImageHash {
		st.mu.Unlock()
		return nil
	}
	st.mu.Unlock()
	if err := client.send(protocol.Envelope{Type: protocol.TypeUpdate, Update: &protocol.ClipboardUpdate{DeviceID: cfg.DeviceID, DeviceName: cfg.DeviceName, Room: cfg.Room, Kind: protocol.KindImage, MIMEType: item.MIMEType, Hash: item.Hash, DataBase64: base64.StdEncoding.EncodeToString(item.Data), Timestamp: time.Now().UTC()}}); err != nil {
		return err
	}
	st.mu.Lock()
	st.lastImageHash = item.Hash
	st.mu.Unlock()
	return nil
}

func applyUpdate(ctx context.Context, backend clipboard.Backend, update protocol.ClipboardUpdate, st *state) error {
	switch update.Kind {
	case protocol.KindText:
		if update.Hash == "" || update.Text == "" {
			return fmt.Errorf("invalid text update")
		}
		st.mu.Lock()
		if update.Hash == st.lastTextHash {
			st.mu.Unlock()
			return nil
		}
		st.mu.Unlock()
		if err := backend.WriteText(ctx, update.Text); err != nil {
			return err
		}
		st.mu.Lock()
		st.lastTextHash = update.Hash
		st.mu.Unlock()
		return nil
	case protocol.KindImage:
		data, err := base64.StdEncoding.DecodeString(update.DataBase64)
		if err != nil {
			return err
		}
		st.mu.Lock()
		if update.Hash == st.lastImageHash {
			st.mu.Unlock()
			return nil
		}
		st.mu.Unlock()
		if err := backend.WriteImage(ctx, data, update.MIMEType); err != nil {
			return err
		}
		st.mu.Lock()
		st.lastImageHash = update.Hash
		st.mu.Unlock()
		return nil
	default:
		return fmt.Errorf("unsupported update kind %q", update.Kind)
	}
}

func dialServer(cfg config.ClientConfig) (net.Conn, error) {
	if !cfg.TLSEnabled {
		return net.Dial("tcp", cfg.ServerAddress)
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.TLSSkipVerify}
	if cfg.TLSCertFile != "" {
		pem, err := os.ReadFile(cfg.TLSCertFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("failed to parse tls cert bundle")
		}
		tlsConfig.RootCAs = pool
	}
	serverName := cfg.ServerAddress
	if host, _, err := net.SplitHostPort(cfg.ServerAddress); err == nil {
		serverName = host
	}
	tlsConfig.ServerName = strings.TrimSpace(serverName)
	return tls.Dial("tcp", cfg.ServerAddress, tlsConfig)
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config/client.ini"
	}
	return home + "/.config/clipboard-sync/client.ini"
}
