package redis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	goredis "github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"

	"github.com/lapp-coder/task-manager-service/internal/config"
)

func Open(ctx context.Context, cfg config.Redis, log *slog.Logger) (*goredis.Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.Timeout,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, errors.Join(fmt.Errorf("ping redis: %w", err), client.Close())
	}
	goredis.SetLogger(logger{log: log})
	client.AddHook(newBreaker(cfg, log))
	return client, nil
}

type breaker struct {
	cb *gobreaker.CircuitBreaker[struct{}]
}

func newBreaker(cfg config.Redis, log *slog.Logger) breaker {
	return breaker{cb: gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        "redis",
		MaxRequests: 1,
		Timeout:     cfg.BreakerCooldown,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= cfg.BreakerFailures
		},
		IsSuccessful: func(err error) bool {
			return err == nil || errors.Is(err, goredis.Nil)
		},
		IsExcluded: func(err error) bool {
			return errors.Is(err, context.Canceled)
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit breaker state changed", "name", name, "from", from.String(), "to", to.String())
		},
	})}
}

func (b breaker) DialHook(next goredis.DialHook) goredis.DialHook {
	return next
}

func (b breaker) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		return b.run(ctx, func(ctx context.Context) error { return next(ctx, cmd) })
	}
}

func (b breaker) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		return b.run(ctx, func(ctx context.Context) error { return next(ctx, cmds) })
	}
}

type breakerScope struct{}

func (b breaker) run(ctx context.Context, fn func(context.Context) error) error {
	if ctx.Value(breakerScope{}) != nil {
		return fn(ctx)
	}
	ctx = context.WithValue(ctx, breakerScope{}, struct{}{})
	_, err := b.cb.Execute(func() (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

type logger struct {
	log *slog.Logger
}

func (l logger) Printf(ctx context.Context, format string, v ...any) {
	l.log.WarnContext(ctx, fmt.Sprintf(format, v...), "component", "go-redis")
}
