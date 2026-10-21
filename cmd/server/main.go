// Command server runs the quiz-v2 monolith: it loads configuration, opens the
// database store and serves the Fiber app on cfg.Addr.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"quiz-v2/internal/config"
	"quiz-v2/internal/httpserver"
	"quiz-v2/internal/store"
)

// advisoryPingTimeout bounds the single startup ping. An unreachable database
// only warns here: /healthz performs its own 2s ping per request and reports
// 503 until the database returns, so startup never depends on DB uptime.
const advisoryPingTimeout = 5 * time.Second

// pingerFunc adapts a plain function to httpserver.Pinger, letting main wire
// the free function store.Ping into the app constructor.
type pingerFunc func(context.Context) error

// Ping calls f.
func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	cfg := config.Load()
	logger.Printf("config loaded: addr=%s", cfg.Addr)

	db, err := store.Open(cfg.DBDSN)
	if err != nil {
		logger.Printf("fatal: open store: %v", err)
		os.Exit(1)
	}
	logger.Printf("store opened")

	ctx, cancel := context.WithTimeout(context.Background(), advisoryPingTimeout)
	err = store.Ping(ctx, db)
	cancel()
	if err != nil {
		logger.Printf("warning: advisory ping failed (server continues, /healthz will report 503): %v", err)
	} else {
		logger.Printf("advisory ping ok")
	}

	pinger := pingerFunc(func(ctx context.Context) error { return store.Ping(ctx, db) })
	app := httpserver.NewWithStore(cfg, pinger)

	logger.Printf("server starting on %s", cfg.Addr)
	if err := app.Listen(cfg.Addr); err != nil {
		logger.Printf("fatal: listen: %v", err)
		os.Exit(1)
	}
}
