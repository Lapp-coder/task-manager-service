package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxTitleLen       = 255
	maxDescriptionLen = 10000
)

type Status string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

var Statuses = []Status{StatusTodo, StatusInProgress, StatusDone}

func ParseStatus(s string) (Status, error) {
	switch st := Status(s); st {
	case StatusTodo, StatusInProgress, StatusDone:
		return st, nil
	default:
		return "", fmt.Errorf("%w: status must be one of todo, in_progress, done", ErrInvalidInput)
	}
}

type Task struct {
	ID          int64
	TeamID      int64
	Title       string
	Description string
	Status      Status
	CreatedBy   int64
	AssigneeID  int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    time.Time
	Version     int
}

type TaskPatch struct {
	Title       *string
	Description *string
	Status      *Status
	AssigneeID  *int64
}

func NewTask(teamID, createdBy int64, p TaskPatch, now time.Time) (Task, error) {
	if p.Title == nil {
		return Task{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	t := Task{
		TeamID:    teamID,
		CreatedBy: createdBy,
		Status:    StatusTodo,
		CreatedAt: now,
		Version:   1,
	}
	return t.Apply(p, now)
}

func (t Task) Apply(p TaskPatch, now time.Time) (Task, error) {
	if p.Title != nil {
		title := strings.TrimSpace(*p.Title)
		if title == "" || utf8.RuneCountInString(title) > maxTitleLen {
			return Task{}, fmt.Errorf("%w: title must be 1 to %d characters long", ErrInvalidInput, maxTitleLen)
		}
		t.Title = title
	}
	if p.Description != nil {
		if utf8.RuneCountInString(*p.Description) > maxDescriptionLen {
			return Task{}, fmt.Errorf("%w: description must be at most %d characters long", ErrInvalidInput, maxDescriptionLen)
		}
		t.Description = *p.Description
	}
	if p.AssigneeID != nil {
		t.AssigneeID = *p.AssigneeID
	}
	if p.Status != nil {
		t.setStatus(*p.Status, now)
	}
	t.UpdatedAt = now
	return t, nil
}

func (t *Task) setStatus(s Status, now time.Time) {
	switch {
	case s != StatusDone:
		t.ClosedAt = time.Time{}
	case t.Status != StatusDone:
		t.ClosedAt = now
	}
	t.Status = s
}

type Diff[T comparable] struct {
	Old T
	New T
}

func diff[T comparable](before, after T) *Diff[T] {
	if before == after {
		return nil
	}
	return &Diff[T]{Old: before, New: after}
}

type TaskChanges struct {
	Title       *Diff[string]
	Description *Diff[string]
	Status      *Diff[Status]
	AssigneeID  *Diff[int64]
}

func Changes(before, after Task) TaskChanges {
	return TaskChanges{
		Title:       diff(before.Title, after.Title),
		Description: diff(before.Description, after.Description),
		Status:      diff(before.Status, after.Status),
		AssigneeID:  diff(before.AssigneeID, after.AssigneeID),
	}
}

func (c TaskChanges) Empty() bool {
	return c.Title == nil && c.Description == nil && c.Status == nil && c.AssigneeID == nil
}

func (m Member) CanEditTask(t Task, c TaskChanges) error {
	switch {
	case m.TeamID != t.TeamID:
		return ErrTaskNotFound
	case m.Role.managesTeam(), t.CreatedBy == m.UserID:
		return nil
	case t.AssigneeID == m.UserID:
		if c.Title != nil || c.Description != nil || c.AssigneeID != nil {
			return fmt.Errorf("%w: assignee can change only task status", ErrForbidden)
		}
		return nil
	default:
		return fmt.Errorf("%w: only task creator, assignee, team owner or admin can edit the task", ErrForbidden)
	}
}

type HistoryEntry struct {
	ID        int64
	TaskID    int64
	ChangedBy int64
	Changes   TaskChanges
	CreatedAt time.Time
}

type TaskFilter struct {
	TeamID     int64
	Status     Status
	AssigneeID int64
	Page       PageRequest
}
