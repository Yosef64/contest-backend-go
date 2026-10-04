// Command api runs the Victory HTTP API.
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

	"github.com/joho/godotenv"

	"victor-contest-go/internal/api"
	"victor-contest-go/internal/auth"
	"victor-contest-go/internal/config"
	"victor-contest-go/internal/db"
	"victor-contest-go/internal/media"
	"victor-contest-go/internal/store"
)

func main() {
	_ = godotenv.Load() // optional .env in development
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	applied, err := db.Migrate(ctx, pool)
	if err != nil {
		return err
	}
	if len(applied) > 0 {
		slog.Info("applied migrations", "versions", applied)
	}
	if err := bootstrapAdmin(ctx, cfg, store.New(pool)); err != nil {
		return err
	}

	m, err := media.New(cfg.CloudinaryURL, cfg.UploadDir, cfg.PublicBaseURL)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewServer(cfg, pool, m).Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute, // video uploads
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}

// bootstrapAdmin creates the first owner account when the admins table is
// empty and BOOTSTRAP_ADMIN_EMAIL / BOOTSTRAP_ADMIN_PASSWORD are set.
func bootstrapAdmin(ctx context.Context, cfg config.Config, s *store.Store) error {
	n, err := s.CountAdmins(ctx)
	if err != nil || n > 0 {
		return err
	}
	if cfg.BootstrapAdminEmail == "" || len(cfg.BootstrapAdminPassword) < 8 {
		slog.Warn("no admin accounts exist; set BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD (8+ chars) to create the owner")
		return nil
	}
	hash, err := auth.HashPassword(cfg.BootstrapAdminPassword)
	if err != nil {
		return err
	}
	_, err = s.CreateAdmin(ctx, store.Admin{Name: "Owner", Email: cfg.BootstrapAdminEmail, PasswordHash: hash, IsApproved: true, IsOwner: true})
	if err == nil {
		slog.Info("created owner admin", "email", cfg.BootstrapAdminEmail)
	}
	return err
}
