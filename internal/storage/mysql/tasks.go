package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

const taskColumns = `id, team_id, title, description, status, created_by, assignee_id, created_at, updated_at, closed_at, version`

type Tasks struct {
	db *sql.DB
}

func NewTasks(db *sql.DB) *Tasks {
	return &Tasks{db: db}
}

func (r *Tasks) Create(ctx context.Context, t domain.Task) (int64, error) {
	id, err := insert(ctx, conn(ctx, r.db), `
		INSERT INTO tasks (team_id, title, description, status, created_by, assignee_id, created_at, updated_at, closed_at, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.TeamID, t.Title, t.Description, string(t.Status), t.CreatedBy, nullID(t.AssigneeID),
		t.CreatedAt, t.UpdatedAt, nullTime(t.ClosedAt), t.Version,
	)
	if err != nil {
		return 0, fmt.Errorf("insert task: %w", err)
	}
	return id, nil
}

func (r *Tasks) ByID(ctx context.Context, id int64) (domain.Task, error) {
	return r.byID(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
}

func (r *Tasks) ByIDForUpdate(ctx context.Context, id int64) (domain.Task, error) {
	return r.byID(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ? FOR UPDATE`, id)
}

func (r *Tasks) byID(ctx context.Context, query string, id int64) (domain.Task, error) {
	t, err := scanTask(conn(ctx, r.db).QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, domain.ErrTaskNotFound
	}
	if err != nil {
		return domain.Task{}, fmt.Errorf("select task: %w", err)
	}
	return t, nil
}

func (r *Tasks) Update(ctx context.Context, t domain.Task, prevVersion int) error {
	res, err := conn(ctx, r.db).ExecContext(ctx, `
		UPDATE tasks
		SET title = ?, description = ?, status = ?, assignee_id = ?, updated_at = ?, closed_at = ?, version = ?
		WHERE id = ? AND version = ?`,
		t.Title, t.Description, string(t.Status), nullID(t.AssigneeID), t.UpdatedAt, nullTime(t.ClosedAt), t.Version,
		t.ID, prevVersion,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Tasks) List(ctx context.Context, f domain.TaskFilter) (domain.Page[domain.Task], error) {
	where := []string{"team_id = ?"}
	args := []any{f.TeamID}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(f.Status))
	}
	if f.AssigneeID != 0 {
		where = append(where, "assignee_id = ?")
		args = append(args, f.AssigneeID)
	}
	if f.Page.AfterID != 0 {
		where = append(where, "id < ?")
		args = append(args, f.Page.AfterID)
	}
	args = append(args, f.Page.Limit+1, f.Page.Offset)

	query := `SELECT ` + taskColumns + ` FROM tasks WHERE ` + strings.Join(where, " AND ") + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	tasks, err := queryAll(ctx, conn(ctx, r.db), scanTask, query, args...)
	if err != nil {
		return domain.Page[domain.Task]{}, fmt.Errorf("select tasks: %w", err)
	}
	return page(tasks, f.Page, func(t domain.Task) int64 { return t.ID }), nil
}

func (r *Tasks) AddHistory(ctx context.Context, h domain.HistoryEntry) error {
	changes, err := json.Marshal(changesRecordOf(h.Changes))
	if err != nil {
		return fmt.Errorf("marshal changes: %w", err)
	}
	_, err = conn(ctx, r.db).ExecContext(ctx,
		`INSERT INTO task_history (task_id, changed_by, changes, created_at) VALUES (?, ?, ?, ?)`,
		h.TaskID, h.ChangedBy, string(changes), h.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert task history: %w", err)
	}
	return nil
}

func (r *Tasks) History(ctx context.Context, taskID int64, p domain.PageRequest) (domain.Page[domain.HistoryEntry], error) {
	entries, err := queryAll(ctx, conn(ctx, r.db), scanHistoryEntry, `
		SELECT id, task_id, changed_by, changes, created_at
		FROM task_history
		WHERE task_id = ? AND id > ?
		ORDER BY id
		LIMIT ? OFFSET ?`,
		taskID, p.AfterID, p.Limit+1, p.Offset,
	)
	if err != nil {
		return domain.Page[domain.HistoryEntry]{}, fmt.Errorf("select task history: %w", err)
	}
	return page(entries, p, func(h domain.HistoryEntry) int64 { return h.ID }), nil
}

func (r *Tasks) AddComment(ctx context.Context, c domain.Comment) (int64, error) {
	id, err := insert(ctx, conn(ctx, r.db),
		`INSERT INTO task_comments (task_id, user_id, content, created_at) VALUES (?, ?, ?, ?)`,
		c.TaskID, c.UserID, c.Content, c.CreatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("insert task comment: %w", err)
	}
	return id, nil
}

func (r *Tasks) Comments(ctx context.Context, taskID int64, p domain.PageRequest) (domain.Page[domain.Comment], error) {
	comments, err := queryAll(ctx, conn(ctx, r.db), scanComment, `
		SELECT id, task_id, user_id, content, created_at
		FROM task_comments
		WHERE task_id = ? AND id > ?
		ORDER BY id
		LIMIT ? OFFSET ?`,
		taskID, p.AfterID, p.Limit+1, p.Offset,
	)
	if err != nil {
		return domain.Page[domain.Comment]{}, fmt.Errorf("select task comments: %w", err)
	}
	return page(comments, p, func(c domain.Comment) int64 { return c.ID }), nil
}

func scanTask(s scanner) (domain.Task, error) {
	var (
		t        domain.Task
		assignee sql.NullInt64
		closedAt sql.NullTime
	)
	err := s.Scan(&t.ID, &t.TeamID, &t.Title, &t.Description, &t.Status, &t.CreatedBy, &assignee,
		&t.CreatedAt, &t.UpdatedAt, &closedAt, &t.Version)
	t.AssigneeID = assignee.Int64
	t.ClosedAt = closedAt.Time
	return t, err
}

func scanHistoryEntry(s scanner) (domain.HistoryEntry, error) {
	var (
		h       domain.HistoryEntry
		changes []byte
	)
	if err := s.Scan(&h.ID, &h.TaskID, &h.ChangedBy, &changes, &h.CreatedAt); err != nil {
		return domain.HistoryEntry{}, err
	}
	var rec changesRecord
	if err := json.Unmarshal(changes, &rec); err != nil {
		return domain.HistoryEntry{}, fmt.Errorf("unmarshal changes of history entry %d: %w", h.ID, err)
	}
	h.Changes = rec.domain()
	return h, nil
}

func scanComment(s scanner) (domain.Comment, error) {
	var c domain.Comment
	err := s.Scan(&c.ID, &c.TaskID, &c.UserID, &c.Content, &c.CreatedAt)
	return c, err
}

type diffRecord[T comparable] struct {
	Old T `json:"old"`
	New T `json:"new"`
}

func diffRecordOf[T comparable](d *domain.Diff[T]) *diffRecord[T] {
	if d == nil {
		return nil
	}
	return &diffRecord[T]{Old: d.Old, New: d.New}
}

func (r *diffRecord[T]) domain() *domain.Diff[T] {
	if r == nil {
		return nil
	}
	return &domain.Diff[T]{Old: r.Old, New: r.New}
}

type changesRecord struct {
	Title       *diffRecord[string]        `json:"title,omitempty"`
	Description *diffRecord[string]        `json:"description,omitempty"`
	Status      *diffRecord[domain.Status] `json:"status,omitempty"`
	AssigneeID  *diffRecord[int64]         `json:"assignee_id,omitempty"`
}

func changesRecordOf(c domain.TaskChanges) changesRecord {
	return changesRecord{
		Title:       diffRecordOf(c.Title),
		Description: diffRecordOf(c.Description),
		Status:      diffRecordOf(c.Status),
		AssigneeID:  diffRecordOf(c.AssigneeID),
	}
}

func (r changesRecord) domain() domain.TaskChanges {
	return domain.TaskChanges{
		Title:       r.Title.domain(),
		Description: r.Description.domain(),
		Status:      r.Status.domain(),
		AssigneeID:  r.AssigneeID.domain(),
	}
}
