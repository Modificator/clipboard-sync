package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/Modificator/clipboard-sync/internal/relay"
)

func main() {
	listenAddr := flag.String("listen", ":8484", "relay listen address")
	token := flag.String("token", "", "shared access token required from clients")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file")
	tlsKey := flag.String("tls-key", "", "TLS key file")
	flag.Parse()
	if *token == "" {
		log.Fatal("relay token is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := log.Default()
	logger.Printf("starting clipboard relay on %s", *listenAddr)
	if err := relay.Listen(ctx, *listenAddr, *tlsCert, *tlsKey, *token, logger); err != nil {
		logger.Fatal(err)
	}
}
