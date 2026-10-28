package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type Limiter struct {
	client *goredis.Client
	limit  int64
	window time.Duration
}

func NewLimiter(client *goredis.Client, limit int, window time.Duration) *Limiter {
	return &Limiter{client: client, limit: int64(limit), window: window}
}

func (l *Limiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	now := time.Now()
	start := now.Truncate(l.window)
	redisKey := fmt.Sprintf("ratelimit:%s:%d", key, start.Unix())

	pipe := l.client.TxPipeline()
	hits := pipe.Incr(ctx, redisKey)
	pipe.ExpireNX(ctx, redisKey, l.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return true, 0, fmt.Errorf("count request: %w", err)
	}
	if hits.Val() > l.limit {
		return false, start.Add(l.window).Sub(now), nil
	}
	return true, 0, nil
}
