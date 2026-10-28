package http

import (
	"fmt"
	"net/http"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

func (h *handler) createTask(w http.ResponseWriter, r *http.Request) error {
	var req createTaskRequest
	if err := decode(w, r, &req); err != nil {
		return err
	}
	if req.TeamID <= 0 {
		return fmt.Errorf("%w: team_id must be a positive integer", domain.ErrInvalidInput)
	}
	patch, err := req.patch()
	if err != nil {
		return err
	}
	t, err := h.tasks.Create(r.Context(), actorID(r.Context()), req.TeamID, patch)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, taskResponseOf(t))
	return nil
}

func (h *handler) listTasks(w http.ResponseWriter, r *http.Request) error {
	f, err := taskFilter(r)
	if err != nil {
		return err
	}
	tasks, err := h.tasks.List(r.Context(), actorID(r.Context()), f)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, pageResponseOf(tasks, taskResponseOf))
	return nil
}

func taskFilter(r *http.Request) (domain.TaskFilter, error) {
	q := r.URL.Query()
	teamID, err := queryInt(q, "team_id")
	if err != nil {
		return domain.TaskFilter{}, err
	}
	if teamID == 0 {
		return domain.TaskFilter{}, fmt.Errorf("%w: team_id is required", domain.ErrInvalidInput)
	}
	assigneeID, err := queryInt(q, "assignee_id")
	if err != nil {
		return domain.TaskFilter{}, err
	}
	page, err := pageRequest(q)
	if err != nil {
		return domain.TaskFilter{}, err
	}
	f := domain.TaskFilter{TeamID: teamID, AssigneeID: assigneeID, Page: page}
	if raw := q.Get("status"); raw != "" {
		if f.Status, err = domain.ParseStatus(raw); err != nil {
			return domain.TaskFilter{}, err
		}
	}
	return f, nil
}

func (h *handler) getTask(w http.ResponseWriter, r *http.Request) error {
	taskID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	t, err := h.tasks.Get(r.Context(), actorID(r.Context()), taskID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, taskResponseOf(t))
	return nil
}

func (h *handler) updateTask(w http.ResponseWriter, r *http.Request) error {
	taskID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var req updateTaskRequest
	if err := decode(w, r, &req); err != nil {
		return err
	}
	patch, err := req.patch()
	if err != nil {
		return err
	}
	t, err := h.tasks.Update(r.Context(), actorID(r.Context()), taskID, patch, req.Version)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, taskResponseOf(t))
	return nil
}

func (h *handler) taskHistory(w http.ResponseWriter, r *http.Request) error {
	taskID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	page, err := pageRequest(r.URL.Query())
	if err != nil {
		return err
	}
	history, err := h.tasks.History(r.Context(), actorID(r.Context()), taskID, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, pageResponseOf(history, historyResponseOf))
	return nil
}

func (h *handler) addComment(w http.ResponseWriter, r *http.Request) error {
	taskID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var req commentRequest
	if err := decode(w, r, &req); err != nil {
		return err
	}
	c, err := h.tasks.Comment(r.Context(), actorID(r.Context()), taskID, req.Content)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, commentResponseOf(c))
	return nil
}

func (h *handler) listComments(w http.ResponseWriter, r *http.Request) error {
	taskID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	page, err := pageRequest(r.URL.Query())
	if err != nil {
		return err
	}
	comments, err := h.tasks.Comments(r.Context(), actorID(r.Context()), taskID, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, pageResponseOf(comments, commentResponseOf))
	return nil
}
