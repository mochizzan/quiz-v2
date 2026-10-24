// Command server runs the quiz-v2 monolith: it loads and validates
// configuration, opens the database pool and serves the Fiber app on cfg.Addr.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
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

// ensureUploads makes the upload root exist and proves it is writable. A
// directory that exists but rejects writes (a root-owned volume mounted into
// the non-root user) is the common container failure, and it is invisible
// until someone tries to upload.
func ensureUploads(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	probe, err := os.CreateTemp(dir, ".write-probe-*")
	if err != nil {
		return fmt.Errorf("%s is not writable: %w", dir, err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return fmt.Errorf("close probe in %s: %w", dir, err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("remove probe %s: %w", name, err)
	}
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	logger := log.New(os.Stdout, "", log.LstdFlags)

	cfg := config.Load()
	logger.Printf("config loaded: addr=%s uploads=%s", cfg.Addr, cfg.UploadsDir)

	// Configuration that cannot honour a security rule is fatal, never a
	// degraded start (IDN-050, OPS-232).
	if err := config.Validate(cfg); err != nil {
		logger.Printf("fatal: %v", err)
		os.Exit(1)
	}
	if err := store.CheckDSN(cfg.DBDSN); err != nil {
		logger.Printf("fatal: %v", err)
		os.Exit(1)
	}

	// The upload root must exist and accept a write before serving. Failing
	// here is fatal on purpose: a broken volume would otherwise surface as a
	// 500 on a real upload, long after the operator could see it.
	if err := ensureUploads(cfg.UploadsDir); err != nil {
		logger.Printf("fatal: uploads: %v", err)
		os.Exit(1)
	}

	db, err := store.Open(cfg.DBDSN)
	if err != nil {
		logger.Printf("fatal: open store: %v", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()
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
	app, err := httpserver.NewWithDeps(cfg, httpserver.Deps{DB: db, Pinger: pinger})
	if err != nil {
		logger.Printf("fatal: build app: %v", err)
		os.Exit(1)
	}

	logger.Printf("server starting on %s", cfg.Addr)
	if err := app.Listen(cfg.Addr); err != nil {
		logger.Printf("fatal: listen: %v", err)
		os.Exit(1)
	}
}
