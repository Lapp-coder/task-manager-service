package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

type TaskRepository interface {
	Create(ctx context.Context, t domain.Task) (int64, error)
	ByID(ctx context.Context, id int64) (domain.Task, error)
	ByIDForUpdate(ctx context.Context, id int64) (domain.Task, error)
	Update(ctx context.Context, t domain.Task, prevVersion int) error
	List(ctx context.Context, f domain.TaskFilter) (domain.Page[domain.Task], error)
	AddHistory(ctx context.Context, h domain.HistoryEntry) error
	History(ctx context.Context, taskID int64, p domain.PageRequest) (domain.Page[domain.HistoryEntry], error)
	AddComment(ctx context.Context, c domain.Comment) (int64, error)
	Comments(ctx context.Context, taskID int64, p domain.PageRequest) (domain.Page[domain.Comment], error)
}

type TaskCache interface {
	Tasks(ctx context.Context, f domain.TaskFilter, load func(context.Context) (domain.Page[domain.Task], error)) (domain.Page[domain.Task], error)
	Invalidate(ctx context.Context, teamID int64)
}

type Tasks struct {
	tx      TxManager
	tasks   TaskRepository
	members MemberRepository
	cache   TaskCache
}

func NewTasks(tx TxManager, tasks TaskRepository, members MemberRepository, cache TaskCache) *Tasks {
	return &Tasks{tx: tx, tasks: tasks, members: members, cache: cache}
}

func (s *Tasks) Create(ctx context.Context, actorID, teamID int64, p domain.TaskPatch) (domain.Task, error) {
	task, err := domain.NewTask(teamID, actorID, p, now())
	if err != nil {
		return domain.Task{}, err
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := member(ctx, s.members, teamID, actorID, domain.ErrTeamNotFound); err != nil {
			return err
		}
		if err := s.ensureAssignable(ctx, task); err != nil {
			return err
		}
		id, err := s.tasks.Create(ctx, task)
		if err != nil {
			return fmt.Errorf("create task: %w", err)
		}
		task.ID = id
		return s.record(ctx, actorID, domain.Task{}, task)
	})
	if err != nil {
		return domain.Task{}, err
	}

	s.cache.Invalidate(ctx, teamID)
	return task, nil
}

func (s *Tasks) Update(ctx context.Context, actorID, taskID int64, p domain.TaskPatch, expectedVersion int) (domain.Task, error) {
	var (
		task    domain.Task
		changed bool
	)
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		current, err := s.access(ctx, actorID, taskID, s.tasks.ByIDForUpdate)
		if err != nil {
			return err
		}
		if expectedVersion != 0 && expectedVersion != current.task.Version {
			return domain.ErrVersionConflict
		}
		next, err := current.task.Apply(p, now())
		if err != nil {
			return err
		}
		changes := domain.Changes(current.task, next)
		if err := current.actor.CanEditTask(current.task, changes); err != nil {
			return err
		}
		if changes.Empty() {
			task = current.task
			return nil
		}
		if changes.AssigneeID != nil {
			if err := s.ensureAssignable(ctx, next); err != nil {
				return err
			}
		}

		next.Version = current.task.Version + 1
		if err := s.tasks.Update(ctx, next, current.task.Version); err != nil {
			return fmt.Errorf("update task %d: %w", taskID, err)
		}
		task, changed = next, true
		return s.record(ctx, actorID, current.task, next)
	})
	if err != nil {
		return domain.Task{}, err
	}

	if changed {
		s.cache.Invalidate(ctx, task.TeamID)
	}
	return task, nil
}

func (s *Tasks) Get(ctx context.Context, actorID, taskID int64) (domain.Task, error) {
	v, err := s.access(ctx, actorID, taskID, s.tasks.ByID)
	if err != nil {
		return domain.Task{}, err
	}
	return v.task, nil
}

func (s *Tasks) List(ctx context.Context, actorID int64, f domain.TaskFilter) (domain.Page[domain.Task], error) {
	if _, err := member(ctx, s.members, f.TeamID, actorID, domain.ErrTeamNotFound); err != nil {
		return domain.Page[domain.Task]{}, err
	}
	page, err := s.cache.Tasks(ctx, f, func(ctx context.Context) (domain.Page[domain.Task], error) {
		return s.tasks.List(ctx, f)
	})
	if err != nil {
		return domain.Page[domain.Task]{}, fmt.Errorf("list tasks of team %d: %w", f.TeamID, err)
	}
	return page, nil
}

func (s *Tasks) History(ctx context.Context, actorID, taskID int64, p domain.PageRequest) (domain.Page[domain.HistoryEntry], error) {
	if _, err := s.access(ctx, actorID, taskID, s.tasks.ByID); err != nil {
		return domain.Page[domain.HistoryEntry]{}, err
	}
	history, err := s.tasks.History(ctx, taskID, p)
	if err != nil {
		return domain.Page[domain.HistoryEntry]{}, fmt.Errorf("history of task %d: %w", taskID, err)
	}
	return history, nil
}

func (s *Tasks) Comment(ctx context.Context, actorID, taskID int64, content string) (domain.Comment, error) {
	c, err := domain.NewComment(taskID, actorID, content, now())
	if err != nil {
		return domain.Comment{}, err
	}
	if _, err := s.access(ctx, actorID, taskID, s.tasks.ByID); err != nil {
		return domain.Comment{}, err
	}
	if c.ID, err = s.tasks.AddComment(ctx, c); err != nil {
		return domain.Comment{}, fmt.Errorf("comment task %d: %w", taskID, err)
	}
	return c, nil
}

func (s *Tasks) Comments(ctx context.Context, actorID, taskID int64, p domain.PageRequest) (domain.Page[domain.Comment], error) {
	if _, err := s.access(ctx, actorID, taskID, s.tasks.ByID); err != nil {
		return domain.Page[domain.Comment]{}, err
	}
	comments, err := s.tasks.Comments(ctx, taskID, p)
	if err != nil {
		return domain.Page[domain.Comment]{}, fmt.Errorf("comments of task %d: %w", taskID, err)
	}
	return comments, nil
}

type taskAccess struct {
	task  domain.Task
	actor domain.Member
}

func (s *Tasks) access(ctx context.Context, actorID, taskID int64, load func(context.Context, int64) (domain.Task, error)) (taskAccess, error) {
	t, err := load(ctx, taskID)
	if errors.Is(err, domain.ErrTaskNotFound) {
		return taskAccess{}, domain.ErrTaskNotFound
	}
	if err != nil {
		return taskAccess{}, fmt.Errorf("get task %d: %w", taskID, err)
	}
	m, err := member(ctx, s.members, t.TeamID, actorID, domain.ErrTaskNotFound)
	if err != nil {
		return taskAccess{}, err
	}
	return taskAccess{task: t, actor: m}, nil
}

func (s *Tasks) ensureAssignable(ctx context.Context, t domain.Task) error {
	if t.AssigneeID == 0 {
		return nil
	}
	_, err := member(ctx, s.members, t.TeamID, t.AssigneeID, domain.ErrAssigneeNotMember)
	return err
}

func (s *Tasks) record(ctx context.Context, actorID int64, before, after domain.Task) error {
	h := domain.HistoryEntry{
		TaskID:    after.ID,
		ChangedBy: actorID,
		Changes:   domain.Changes(before, after),
		CreatedAt: after.UpdatedAt,
	}
	if err := s.tasks.AddHistory(ctx, h); err != nil {
		return fmt.Errorf("record history of task %d: %w", after.ID, err)
	}
	return nil
}
