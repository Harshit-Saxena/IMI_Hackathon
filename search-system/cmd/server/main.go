package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/yourusername/search-system/internal/api"
	"github.com/yourusername/search-system/internal/config"
	"github.com/yourusername/search-system/internal/db"
	"github.com/yourusername/search-system/internal/outbox"
	"github.com/yourusername/search-system/internal/upsert"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// ── Config ────────────────────────────────────────────────────────────────
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}
	logger.Info().Str("env", cfg.App.Env).Int("port", cfg.App.Port).Msg("config loaded")

	// ── Database ──────────────────────────────────────────────────────────────
	database, err := db.Connect(cfg.Postgres)
	if err != nil {
		logger.Fatal().Err(err).
			Str("host", cfg.Postgres.Host).
			Msg("failed to connect to PostgreSQL")
	}
	defer database.Close()
	logger.Info().Str("host", cfg.Postgres.Host).Msg("connected to PostgreSQL")

	// ── Migrations ────────────────────────────────────────────────────────────
	if err := db.Migrate(database); err != nil {
		logger.Fatal().Err(err).Msg("migrations failed")
	}
	logger.Info().Msg("all migrations applied")

	// ── Phase 2 components ────────────────────────────────────────────────────
	ob := outbox.New(database)

	engineCfg := upsert.Config{
		BatchSize:   cfg.Search.BatchSize,
		WorkerCount: cfg.Search.WorkerCount,
	}

	// ── HTTP server ───────────────────────────────────────────────────────────
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := api.NewRouter(database, ob, engineCfg)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.App.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info().Int("port", cfg.App.Port).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("server exited cleanly")
}
