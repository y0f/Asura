package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/y0f/Asura/internal/api"
	"github.com/y0f/Asura/internal/checker"
	"github.com/y0f/Asura/internal/config"
	"github.com/y0f/Asura/internal/incident"
	"github.com/y0f/Asura/internal/monitor"
	"github.com/y0f/Asura/internal/notifier"
	"github.com/y0f/Asura/internal/storage"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	hashKey := flag.String("hash-key", "", "hash an API key and exit")
	setup := flag.Bool("setup", false, "generate an API key and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("asura %s\n", version)
		os.Exit(0)
	}

	if *setup {
		key, hash := generateAPIKey()
		fmt.Println()
		fmt.Printf("  API Key : %s\n", key)
		fmt.Printf("  Hash    : %s\n", hash)
		fmt.Println()
		fmt.Println("  Put the hash in config.yaml under auth.api_keys[].hash")
		fmt.Println("  Save the API key — it cannot be recovered.")
		fmt.Println()
		os.Exit(0)
	}

	if *hashKey != "" {
		fmt.Println(config.HashAPIKey(*hashKey))
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg.Logging)
	logger.Info("starting asura", "version", version, "listen", cfg.Server.Listen)

	store, err := storage.NewSQLiteStore(cfg.Database.Path, cfg.Database.MaxReadConns)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	logger.Info("database opened", "path", cfg.Database.Path)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	purgeStaleSessionsOnStartup(ctx, store, cfg, logger)

	registry := checker.DefaultRegistry(cfg.Monitor.CommandAllowlist, cfg.Monitor.AllowPrivateTargets)
	incMgr := incident.NewManager(store, logger)
	pipeline := monitor.NewPipeline(store, registry, incMgr, cfg.Monitor.Workers, logger)
	dispatcher := notifier.NewDispatcher(store, logger, cfg.Monitor.AllowPrivateTargets)

	go forwardNotifications(ctx, pipeline, dispatcher)
	go pipeline.Run(ctx)

	heartbeatWatcher := monitor.NewHeartbeatWatcher(store, incMgr, pipeline, cfg.Monitor.HeartbeatCheckInterval, logger)
	go heartbeatWatcher.Run(ctx)

	retentionWorker := storage.NewRetentionWorker(store, cfg.Database.RetentionDays, cfg.Database.RequestLogRetentionDays, cfg.Database.RetentionPeriod, logger)
	go retentionWorker.Run(ctx)

	srv := api.NewServer(cfg, store, pipeline, dispatcher, logger, version)
	go srv.RequestLogWriter().Run(ctx)
	go runRollupWorker(ctx, store, logger)
	httpServer := startHTTPServer(cfg, srv, logger, cancel)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Info("received signal, shutting down", "signal", sig)
	case <-ctx.Done():
	}

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}

	logger.Info("shutdown complete")
}

func forwardNotifications(ctx context.Context, pipeline *monitor.Pipeline, dispatcher *notifier.Dispatcher) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-pipeline.NotifyChan():
			payload := &notifier.Payload{
				EventType: event.EventType,
				Incident:  event.Incident,
				Monitor:   event.Monitor,
				Change:    event.Change,
			}
			dispatcher.NotifyWithPayload(payload)
		}
	}
}

func startHTTPServer(cfg *config.Config, handler http.Handler, logger *slog.Logger, cancel context.CancelFunc) *http.Server {
	httpServer := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	if cfg.Server.TLSCert != "" && cfg.Server.TLSKey != "" {
		httpServer.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		go func() {
			logger.Info("starting HTTPS server", "listen", cfg.Server.Listen)
			if err := httpServer.ListenAndServeTLS(cfg.Server.TLSCert, cfg.Server.TLSKey); err != nil && err != http.ErrServerClosed {
				logger.Error("HTTPS server error", "error", err)
				cancel()
			}
		}()
	} else {
		go func() {
			logger.Info("starting HTTP server", "listen", cfg.Server.Listen)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("HTTP server error", "error", err)
				cancel()
			}
		}()
	}

	return httpServer
}

func generateAPIKey() (key, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to generate random bytes: %v\n", err)
		os.Exit(1)
	}
	key = "ak_" + hex.EncodeToString(b)
	hash = config.HashAPIKey(key)
	return key, hash
}

func setupLogger(cfg config.LoggingConfig) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func runRollupWorker(ctx context.Context, store storage.Store, logger *slog.Logger) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
			if err := store.RollupRequestLogs(ctx, yesterday); err != nil {
				logger.Error("request log rollup failed", "date", yesterday, "error", err)
			}
		}
	}
}

func purgeStaleSessionsOnStartup(ctx context.Context, store storage.Store, cfg *config.Config, logger *slog.Logger) {
	validNames := make([]string, len(cfg.Auth.APIKeys))
	for i, k := range cfg.Auth.APIKeys {
		validNames[i] = k.Name
	}
	deleted, err := store.DeleteSessionsExceptKeyNames(ctx, validNames)
	if err != nil {
		logger.Warn("failed to purge stale sessions", "error", err)
		return
	}
	if deleted > 0 {
		logger.Info("purged sessions for removed API keys", "deleted", deleted)
	}
}
