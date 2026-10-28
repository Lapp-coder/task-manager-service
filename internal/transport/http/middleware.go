package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/lapp-coder/task-manager-service/internal/domain"
	"github.com/lapp-coder/task-manager-service/internal/logging"
)

const (
	requestIDHeader = "X-Request-ID"
	maxRequestIDLen = 64
)

type middleware func(http.Handler) http.Handler

func chain(h http.Handler, mws ...middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func recoverPanics(log *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					log.ErrorContext(r.Context(), "panic", "value", fmt.Sprint(v), "stack", string(debug.Stack()))
					writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" || len(id) > maxRequestIDLen {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
	})
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func withTimeout(d time.Duration) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func rateLimit(l RateLimiter, h *handler) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter, err := l.Allow(r.Context(), "ip:"+clientIP(r))
			if err != nil {
				h.log.WarnContext(r.Context(), "rate limiter unavailable", "error", err)
			}
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
				writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "too many requests"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type metrics struct {
	duration *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer) metrics {
	m := metrics{duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by route and status.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route", "status"})}
	reg.MustRegister(m.duration)
	return m
}

func observe(log *slog.Logger, m metrics) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			elapsed := time.Since(start)

			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			m.duration.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Observe(elapsed.Seconds())
			log.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"route", route,
				"status", rec.status,
				"duration_ms", elapsed.Milliseconds(),
				"remote_ip", clientIP(r),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

type userIDKey struct{}

func actorID(ctx context.Context) int64 {
	id, _ := ctx.Value(userIDKey{}).(int64)
	return id
}

func authenticate(tokens TokenParser, h *handler) func(handlerFunc) http.Handler {
	return func(next handlerFunc) http.Handler {
		return h.handle(func(w http.ResponseWriter, r *http.Request) error {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || raw == "" {
				return domain.ErrUnauthorized
			}
			userID, err := tokens.Parse(raw)
			if err != nil {
				return err
			}
			return next(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, userID)))
		})
	}
}
