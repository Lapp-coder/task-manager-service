package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

const invalidateTimeout = time.Second

type TaskCache struct {
	client *goredis.Client
	ttl    time.Duration
	log    *slog.Logger
}

func NewTaskCache(client *goredis.Client, ttl time.Duration, log *slog.Logger) *TaskCache {
	return &TaskCache{client: client, ttl: ttl, log: log}
}

func (c *TaskCache) Tasks(
	ctx context.Context,
	f domain.TaskFilter,
	load func(context.Context) (domain.Page[domain.Task], error),
) (domain.Page[domain.Task], error) {
	key, err := c.listKey(ctx, f)
	if err != nil {
		c.log.WarnContext(ctx, "tasks cache unavailable", "team_id", f.TeamID, "error", err)
		return load(ctx)
	}

	cached, err := c.client.Get(ctx, key).Bytes()
	switch {
	case err == nil:
		var p taskPage
		if err := json.Unmarshal(cached, &p); err == nil {
			return p.domain(), nil
		}
	case !errors.Is(err, goredis.Nil):
		c.log.WarnContext(ctx, "read tasks cache", "key", key, "error", err)
	}

	page, err := load(ctx)
	if err != nil {
		return domain.Page[domain.Task]{}, err
	}
	c.store(ctx, key, page)
	return page, nil
}

func (c *TaskCache) Invalidate(ctx context.Context, teamID int64) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), invalidateTimeout)
	defer cancel()
	if err := c.client.Incr(ctx, generationKey(teamID)).Err(); err != nil {
		c.log.ErrorContext(ctx, "invalidate tasks cache", "team_id", teamID, "error", err)
	}
}

func (c *TaskCache) store(ctx context.Context, key string, page domain.Page[domain.Task]) {
	data, err := json.Marshal(taskPageOf(page))
	if err != nil {
		c.log.ErrorContext(ctx, "marshal tasks page", "error", err)
		return
	}
	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		c.log.WarnContext(ctx, "write tasks cache", "key", key, "error", err)
	}
}

func (c *TaskCache) listKey(ctx context.Context, f domain.TaskFilter) (string, error) {
	gen, err := c.client.Get(ctx, generationKey(f.TeamID)).Int64()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return "", fmt.Errorf("get cache generation: %w", err)
	}
	return fmt.Sprintf("tasks:team:%d:gen:%d:status:%s:assignee:%d:limit:%d:offset:%d:after:%d",
		f.TeamID, gen, f.Status, f.AssigneeID, f.Page.Limit, f.Page.Offset, f.Page.AfterID), nil
}

func generationKey(teamID int64) string {
	return fmt.Sprintf("tasks:team:%d:gen", teamID)
}

type taskPage struct {
	Items      []task `json:"items"`
	NextCursor int64  `json:"next_cursor"`
}

type task struct {
	ID          int64         `json:"id"`
	TeamID      int64         `json:"team_id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Status      domain.Status `json:"status"`
	CreatedBy   int64         `json:"created_by"`
	AssigneeID  int64         `json:"assignee_id"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	ClosedAt    time.Time     `json:"closed_at"`
	Version     int           `json:"version"`
}

func taskPageOf(p domain.Page[domain.Task]) taskPage {
	items := make([]task, len(p.Items))
	for i, t := range p.Items {
		items[i] = task(t)
	}
	return taskPage{Items: items, NextCursor: p.NextCursor}
}

func (p taskPage) domain() domain.Page[domain.Task] {
	items := make([]domain.Task, len(p.Items))
	for i, t := range p.Items {
		items[i] = domain.Task(t)
	}
	return domain.Page[domain.Task]{Items: items, NextCursor: p.NextCursor}
}
