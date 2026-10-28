package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	goredis "github.com/redis/go-redis/v9"

	"github.com/lapp-coder/task-manager-service/internal/config"
	"github.com/lapp-coder/task-manager-service/internal/logging"
	"github.com/lapp-coder/task-manager-service/internal/service"
	"github.com/lapp-coder/task-manager-service/internal/storage/mysql"
	"github.com/lapp-coder/task-manager-service/internal/storage/redis"
	"github.com/lapp-coder/task-manager-service/internal/token"
	httptransport "github.com/lapp-coder/task-manager-service/internal/transport/http"
)

const startupTimeout = 30 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	log := logging.New(os.Stdout, cfg.LogLevel)

	if err := run(cfg, log); err != nil {
		log.Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	db, err := mysql.Open(startCtx, cfg.MySQL)
	if err != nil {
		return err
	}
	defer closeWithLog(log, "mysql", db.Close)

	rdb, err := redis.Open(startCtx, cfg.Redis, log)
	if err != nil {
		return err
	}
	defer closeWithLog(log, "redis", rdb.Close)

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           router(cfg, log, db, rdb),
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info("http server started", "addr", cfg.HTTP.Addr)
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}
	return nil
}

func router(cfg config.Config, log *slog.Logger, db *sql.DB, rdb *goredis.Client) http.Handler {
	tx := mysql.NewTxManager(db)
	users := mysql.NewUsers(db)
	teams := mysql.NewTeams(db)
	tokens := token.NewJWT(cfg.JWT.Secret, cfg.JWT.TTL)

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	return httptransport.NewRouter(httptransport.Deps{
		Auth:  service.NewAuth(users, tokens),
		Teams: service.NewTeams(tx, teams, users, mysql.NewStats(db)),
		Tasks: service.NewTasks(tx, mysql.NewTasks(db), teams,
			redis.NewTaskCache(rdb, cfg.Cache.TasksTTL, log)),
		Tokens:         tokens,
		Limiter:        redis.NewLimiter(rdb, cfg.Limit.Requests, cfg.Limit.Window),
		Ready:          db.PingContext,
		Registry:       registry,
		RequestTimeout: cfg.HTTP.RequestTimeout,
		Log:            log,
	})
}

func closeWithLog(log *slog.Logger, name string, closeFn func() error) {
	if err := closeFn(); err != nil {
		log.Error("close resource", "resource", name, "error", err)
	}
}
