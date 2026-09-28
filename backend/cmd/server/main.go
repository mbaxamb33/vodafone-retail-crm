package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vodafone/store/internal/auth"
	"vodafone/store/internal/config"
	"vodafone/store/internal/crm"
	"vodafone/store/internal/httpapi"
	"vodafone/store/internal/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err.Error())
		os.Exit(2)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	db, err := postgres.Open(startCtx, cfg.DatabaseURL, nil)
	if err != nil {
		return err
	}
	defer db.Close()
	if cfg.AutoMigrate {
		if err := db.Migrate(startCtx); err != nil {
			return err
		}
	}

	authCfg := auth.DefaultConfig()
	authCfg.SessionTTL = cfg.SessionTTL
	api := httpapi.New(crm.NewService(db), auth.NewService(db, authCfg), httpapi.Config{AppOrigin: cfg.AppOrigin, SessionTTL: cfg.SessionTTL, Features: cfg.Features, StaticDir: cfg.StaticDir, TrustProxy: cfg.TrustProxy}, log, db.Ping)
	srv := &http.Server{
		Addr: cfg.ListenAddr, Handler: api.Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("server starting", "address", cfg.ListenAddr, "env", cfg.Env)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
