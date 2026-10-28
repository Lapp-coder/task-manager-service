package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/lapp-coder/task-manager-service/internal/service"
)

type TokenParser interface {
	Parse(raw string) (int64, error)
}

type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, time.Duration, error)
}

type Deps struct {
	Auth           *service.Auth
	Teams          *service.Teams
	Tasks          *service.Tasks
	Tokens         TokenParser
	Limiter        RateLimiter
	Ready          func(context.Context) error
	Registry       *prometheus.Registry
	RequestTimeout time.Duration
	Log            *slog.Logger
}

type handler struct {
	auth  *service.Auth
	teams *service.Teams
	tasks *service.Tasks
	log   *slog.Logger
}

func NewRouter(d Deps) http.Handler {
	h := &handler{auth: d.Auth, teams: d.Teams, tasks: d.Tasks, log: d.Log}
	authed := authenticate(d.Tokens, h)

	api := http.NewServeMux()
	api.Handle("POST /api/v1/register", h.handle(h.register))
	api.Handle("POST /api/v1/login", h.handle(h.login))

	api.Handle("POST /api/v1/teams", authed(h.createTeam))
	api.Handle("GET /api/v1/teams", authed(h.listTeams))
	api.Handle("GET /api/v1/teams/{id}/members", authed(h.listMembers))
	api.Handle("POST /api/v1/teams/{id}/invite", authed(h.invite))
	api.Handle("PATCH /api/v1/teams/{id}/members/{user_id}", authed(h.changeRole))
	api.Handle("GET /api/v1/teams/{id}/stats", authed(h.teamStats))

	api.Handle("POST /api/v1/tasks", authed(h.createTask))
	api.Handle("GET /api/v1/tasks", authed(h.listTasks))
	api.Handle("GET /api/v1/tasks/{id}", authed(h.getTask))
	api.Handle("PUT /api/v1/tasks/{id}", authed(h.updateTask))
	api.Handle("GET /api/v1/tasks/{id}/history", authed(h.taskHistory))
	api.Handle("POST /api/v1/tasks/{id}/comments", authed(h.addComment))
	api.Handle("GET /api/v1/tasks/{id}/comments", authed(h.listComments))

	apiChain := chain(api,
		recoverPanics(d.Log),
		withRequestID,
		withTimeout(d.RequestTimeout),
		rateLimit(d.Limiter, h),
		observe(d.Log, newMetrics(d.Registry)),
	)

	root := http.NewServeMux()
	root.Handle("/api/", apiChain)
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	root.HandleFunc("GET /readyz", readiness(d.Ready, d.Log))
	root.Handle("GET /metrics", promhttp.HandlerFor(d.Registry, promhttp.HandlerOpts{}))
	root.HandleFunc("GET /swagger/", swaggerUI)
	root.HandleFunc("GET /swagger/openapi.yaml", openAPISpec)
	return root
}

func readiness(ready func(context.Context) error, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ready(r.Context()); err != nil {
			log.WarnContext(r.Context(), "readiness check", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
