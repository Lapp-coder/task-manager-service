package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

const maxBodyBytes = 1 << 20

type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (h *handler) handle(fn handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			h.writeError(w, r, err)
		}
	})
}

type errorResponse struct {
	Error string `json:"error"`
}

var errorStatuses = []struct {
	err      error
	status   int
	detailed bool
}{
	{domain.ErrInvalidInput, http.StatusBadRequest, true},
	{domain.ErrUnauthorized, http.StatusUnauthorized, false},
	{domain.ErrInvalidCredentials, http.StatusUnauthorized, false},
	{domain.ErrForbidden, http.StatusForbidden, true},
	{domain.ErrUserNotFound, http.StatusNotFound, false},
	{domain.ErrTeamNotFound, http.StatusNotFound, false},
	{domain.ErrTaskNotFound, http.StatusNotFound, false},
	{domain.ErrMemberNotFound, http.StatusNotFound, false},
	{domain.ErrEmailTaken, http.StatusConflict, false},
	{domain.ErrAlreadyMember, http.StatusConflict, false},
	{domain.ErrVersionConflict, http.StatusConflict, false},
	{domain.ErrAssigneeNotMember, http.StatusUnprocessableEntity, false},
}

func (h *handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	for _, e := range errorStatuses {
		if !errors.Is(err, e.err) {
			continue
		}
		msg := e.err.Error()
		if e.detailed {
			msg = err.Error()
		}
		if e.status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", "Bearer")
		}
		writeJSON(w, e.status, errorResponse{Error: msg})
		return
	}
	h.log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: malformed request body: %s", domain.ErrInvalidInput, err.Error())
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("%w: request body must contain a single JSON object", domain.ErrInvalidInput)
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: %s must be a positive integer", domain.ErrInvalidInput, name)
	}
	return id, nil
}

func queryInt(q url.Values, name string) (int64, error) {
	raw := q.Get(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative integer", domain.ErrInvalidInput, name)
	}
	return n, nil
}

func pageRequest(q url.Values) (domain.PageRequest, error) {
	limit, err := queryInt(q, "limit")
	if err != nil {
		return domain.PageRequest{}, err
	}
	offset, err := queryInt(q, "offset")
	if err != nil {
		return domain.PageRequest{}, err
	}
	afterID, err := decodeCursor(q.Get("cursor"))
	if err != nil {
		return domain.PageRequest{}, err
	}
	return domain.NewPageRequest(int(limit), int(offset), afterID)
}

func encodeCursor(id int64) string {
	if id == 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

func decodeCursor(cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("%w: cursor is malformed", domain.ErrInvalidInput)
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: cursor is malformed", domain.ErrInvalidInput)
	}
	return id, nil
}
