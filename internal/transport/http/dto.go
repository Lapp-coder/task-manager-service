package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func userResponseOf(u domain.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Name: u.Name, CreatedAt: u.CreatedAt}
}

type tokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type createTeamRequest struct {
	Name string `json:"name"`
}

type teamResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	Role      string    `json:"role"`
}

func teamResponseOf(t domain.Team, role domain.Role) teamResponse {
	return teamResponse{ID: t.ID, Name: t.Name, CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt, Role: string(role)}
}

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type changeRoleRequest struct {
	Role string `json:"role"`
}

type memberResponse struct {
	TeamID   int64     `json:"team_id"`
	UserID   int64     `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
	Email    string    `json:"email,omitempty"`
	Name     string    `json:"name,omitempty"`
}

func memberResponseOf(m domain.TeamMember) memberResponse {
	return memberResponse{
		TeamID:   m.TeamID,
		UserID:   m.UserID,
		Role:     string(m.Role),
		JoinedAt: m.JoinedAt,
		Email:    m.Email,
		Name:     m.Name,
	}
}

type createTaskRequest struct {
	TeamID      int64   `json:"team_id"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	AssigneeID  *int64  `json:"assignee_id"`
}

func (r createTaskRequest) patch() (domain.TaskPatch, error) {
	return taskPatch(r.Title, r.Description, r.Status, nullableID{set: r.AssigneeID != nil, id: r.AssigneeID})
}

type updateTaskRequest struct {
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	Status      *string    `json:"status"`
	AssigneeID  nullableID `json:"assignee_id"`
	Version     int        `json:"version"`
}

func (r updateTaskRequest) patch() (domain.TaskPatch, error) {
	if r.Version < 0 {
		return domain.TaskPatch{}, fmt.Errorf("%w: version must not be negative", domain.ErrInvalidInput)
	}
	return taskPatch(r.Title, r.Description, r.Status, r.AssigneeID)
}

func taskPatch(title, description, status *string, assignee nullableID) (domain.TaskPatch, error) {
	p := domain.TaskPatch{Title: title, Description: description}
	if status != nil {
		s, err := domain.ParseStatus(*status)
		if err != nil {
			return domain.TaskPatch{}, err
		}
		p.Status = &s
	}
	if assignee.set {
		var id int64
		if assignee.id != nil {
			if *assignee.id <= 0 {
				return domain.TaskPatch{}, fmt.Errorf("%w: assignee_id must be a positive integer or null", domain.ErrInvalidInput)
			}
			id = *assignee.id
		}
		p.AssigneeID = &id
	}
	return p, nil
}

type nullableID struct {
	set bool
	id  *int64
}

func (n *nullableID) UnmarshalJSON(data []byte) error {
	n.set = true
	if bytes.Equal(data, []byte("null")) {
		n.id = nil
		return nil
	}
	if err := json.Unmarshal(data, &n.id); err != nil {
		return fmt.Errorf("assignee_id: %w", err)
	}
	return nil
}

type taskResponse struct {
	ID          int64      `json:"id"`
	TeamID      int64      `json:"team_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	CreatedBy   int64      `json:"created_by"`
	AssigneeID  *int64     `json:"assignee_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	Version     int        `json:"version"`
}

func taskResponseOf(t domain.Task) taskResponse {
	resp := taskResponse{
		ID:          t.ID,
		TeamID:      t.TeamID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		CreatedBy:   t.CreatedBy,
		AssigneeID:  optionalID(t.AssigneeID),
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		Version:     t.Version,
	}
	if !t.ClosedAt.IsZero() {
		resp.ClosedAt = &t.ClosedAt
	}
	return resp
}

func optionalID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

type pageResponse[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func pageResponseOf[D, T any](p domain.Page[D], convert func(D) T) pageResponse[T] {
	items := make([]T, len(p.Items))
	for i, item := range p.Items {
		items[i] = convert(item)
	}
	return pageResponse[T]{Items: items, NextCursor: encodeCursor(p.NextCursor)}
}

type diffResponse[T any] struct {
	Old T `json:"old"`
	New T `json:"new"`
}

func diffResponseOf[D comparable, T any](d *domain.Diff[D], convert func(D) T) *diffResponse[T] {
	if d == nil {
		return nil
	}
	return &diffResponse[T]{Old: convert(d.Old), New: convert(d.New)}
}

func same[T any](v T) T { return v }

type changesResponse struct {
	Title       *diffResponse[string] `json:"title,omitempty"`
	Description *diffResponse[string] `json:"description,omitempty"`
	Status      *diffResponse[string] `json:"status,omitempty"`
	AssigneeID  *diffResponse[*int64] `json:"assignee_id,omitempty"`
}

type historyResponse struct {
	ID        int64           `json:"id"`
	TaskID    int64           `json:"task_id"`
	ChangedBy int64           `json:"changed_by"`
	Changes   changesResponse `json:"changes"`
	CreatedAt time.Time       `json:"created_at"`
}

func historyResponseOf(h domain.HistoryEntry) historyResponse {
	return historyResponse{
		ID:        h.ID,
		TaskID:    h.TaskID,
		ChangedBy: h.ChangedBy,
		CreatedAt: h.CreatedAt,
		Changes: changesResponse{
			Title:       diffResponseOf(h.Changes.Title, same[string]),
			Description: diffResponseOf(h.Changes.Description, same[string]),
			Status:      diffResponseOf(h.Changes.Status, func(s domain.Status) string { return string(s) }),
			AssigneeID:  diffResponseOf(h.Changes.AssigneeID, optionalID),
		},
	}
}

type commentRequest struct {
	Content string `json:"content"`
}

type commentResponse struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	UserID    int64     `json:"user_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func commentResponseOf(c domain.Comment) commentResponse {
	return commentResponse{ID: c.ID, TaskID: c.TaskID, UserID: c.UserID, Content: c.Content, CreatedAt: c.CreatedAt}
}

type statsResponse struct {
	TeamID              int64              `json:"team_id"`
	TasksByStatus       map[string]int     `json:"tasks_by_status"`
	TopAssignees        []assigneeResponse `json:"top_assignees"`
	AvgCloseTimeSeconds *float64           `json:"avg_close_time_seconds"`
	CommentsTotal       int                `json:"comments_total"`
}

type assigneeResponse struct {
	UserID      int64  `json:"user_id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	ClosedTasks int    `json:"closed_tasks"`
}

func statsResponseOf(s domain.TeamStats) statsResponse {
	resp := statsResponse{
		TeamID:        s.TeamID,
		TasksByStatus: make(map[string]int, len(s.TasksByStatus)),
		TopAssignees:  make([]assigneeResponse, len(s.TopAssignees)),
		CommentsTotal: s.CommentsTotal,
	}
	for status, n := range s.TasksByStatus {
		resp.TasksByStatus[string(status)] = n
	}
	for i, a := range s.TopAssignees {
		resp.TopAssignees[i] = assigneeResponse{UserID: a.UserID, Name: a.Name, Email: a.Email, ClosedTasks: a.ClosedTasks}
	}
	if s.AvgCloseTime != nil {
		seconds := s.AvgCloseTime.Seconds()
		resp.AvgCloseTimeSeconds = &seconds
	}
	return resp
}
