package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lapp-coder/task-manager-service/internal/domain"
	"github.com/lapp-coder/task-manager-service/internal/service"
)

const (
	teamID      = 1
	otherTeamID = 2
	ownerID     = 1
	adminID     = 2
	creatorID   = 3
	assigneeID  = 4
	memberID    = 5
	outsiderID  = 6
	taskID      = 100
)

func ptr[T any](v T) *T { return &v }

type tasksEnv struct {
	svc   *service.Tasks
	tasks *fakeTasks
	cache *fakeCache
}

func newTasksEnv() tasksEnv {
	teams := newFakeTeams(
		domain.Member{TeamID: teamID, UserID: ownerID, Role: domain.RoleOwner},
		domain.Member{TeamID: teamID, UserID: adminID, Role: domain.RoleAdmin},
		domain.Member{TeamID: teamID, UserID: creatorID, Role: domain.RoleMember},
		domain.Member{TeamID: teamID, UserID: assigneeID, Role: domain.RoleMember},
		domain.Member{TeamID: teamID, UserID: memberID, Role: domain.RoleMember},
		domain.Member{TeamID: otherTeamID, UserID: outsiderID, Role: domain.RoleOwner},
	)
	tasks := newFakeTasks(domain.Task{
		ID:         taskID,
		TeamID:     teamID,
		Title:      "initial",
		Status:     domain.StatusTodo,
		CreatedBy:  creatorID,
		AssigneeID: assigneeID,
		CreatedAt:  time.Now().Add(-time.Hour),
		Version:    1,
	})
	cache := &fakeCache{}
	return tasksEnv{svc: service.NewTasks(fakeTx{}, tasks, teams, cache), tasks: tasks, cache: cache}
}

func TestTasksUpdatePermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actor   int64
		patch   domain.TaskPatch
		wantErr error
	}{
		{"owner changes title", ownerID, domain.TaskPatch{Title: ptr("new")}, nil},
		{"admin reassigns", adminID, domain.TaskPatch{AssigneeID: ptr[int64](memberID)}, nil},
		{"creator changes everything", creatorID, domain.TaskPatch{
			Title:       ptr("new"),
			Description: ptr("desc"),
			Status:      ptr(domain.StatusInProgress),
			AssigneeID:  ptr[int64](memberID),
		}, nil},
		{"creator unassigns", creatorID, domain.TaskPatch{AssigneeID: ptr[int64](0)}, nil},
		{"assignee changes status", assigneeID, domain.TaskPatch{Status: ptr(domain.StatusDone)}, nil},
		{"assignee cannot reassign", assigneeID, domain.TaskPatch{AssigneeID: ptr[int64](memberID)}, domain.ErrForbidden},
		{"assignee cannot rename", assigneeID, domain.TaskPatch{Title: ptr("new")}, domain.ErrForbidden},
		{"unrelated member is forbidden", memberID, domain.TaskPatch{Status: ptr(domain.StatusDone)}, domain.ErrForbidden},
		{"outsider does not see task", outsiderID, domain.TaskPatch{Status: ptr(domain.StatusDone)}, domain.ErrTaskNotFound},
		{"assignee must be team member", creatorID, domain.TaskPatch{AssigneeID: ptr[int64](outsiderID)}, domain.ErrAssigneeNotMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newTasksEnv()

			got, err := env.svc.Update(context.Background(), tt.actor, taskID, tt.patch, 0)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Empty(t, env.tasks.recorded())
				require.Empty(t, env.cache.invalidations())
				return
			}
			require.NoError(t, err)
			require.Equal(t, 2, got.Version)
			require.Len(t, env.tasks.recorded(), 1)
			require.Equal(t, []int64{teamID}, env.cache.invalidations())
		})
	}
}

func TestTasksUpdateRecordsHistoryFromCurrentState(t *testing.T) {
	t.Parallel()
	env := newTasksEnv()
	ctx := context.Background()

	_, err := env.svc.Update(ctx, assigneeID, taskID, domain.TaskPatch{Status: ptr(domain.StatusInProgress)}, 0)
	require.NoError(t, err)
	got, err := env.svc.Update(ctx, ownerID, taskID, domain.TaskPatch{Status: ptr(domain.StatusDone)}, 0)
	require.NoError(t, err)

	require.Equal(t, 3, got.Version)
	require.False(t, got.ClosedAt.IsZero())

	history := env.tasks.recorded()
	require.Len(t, history, 2)
	require.Equal(t, int64(ownerID), history[1].ChangedBy)
	require.Equal(t, &domain.Diff[domain.Status]{Old: domain.StatusInProgress, New: domain.StatusDone}, history[1].Changes.Status)
	require.Nil(t, history[1].Changes.Title)
}

func TestTasksUpdateVersionConflict(t *testing.T) {
	t.Parallel()
	env := newTasksEnv()

	_, err := env.svc.Update(context.Background(), ownerID, taskID, domain.TaskPatch{Title: ptr("new")}, 7)
	require.ErrorIs(t, err, domain.ErrVersionConflict)
	require.Empty(t, env.tasks.recorded())
}

func TestTasksUpdateWithoutChanges(t *testing.T) {
	t.Parallel()
	env := newTasksEnv()

	got, err := env.svc.Update(context.Background(), creatorID, taskID, domain.TaskPatch{Title: ptr("initial")}, 1)
	require.NoError(t, err)
	require.Equal(t, 1, got.Version)
	require.Empty(t, env.tasks.recorded())
	require.Empty(t, env.cache.invalidations())
}

func TestTasksCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actor   int64
		team    int64
		patch   domain.TaskPatch
		wantErr error
	}{
		{"member creates task", memberID, teamID, domain.TaskPatch{Title: ptr("t"), AssigneeID: ptr[int64](assigneeID)}, nil},
		{"outsider cannot create", outsiderID, teamID, domain.TaskPatch{Title: ptr("t")}, domain.ErrTeamNotFound},
		{"assignee from another team", memberID, teamID, domain.TaskPatch{Title: ptr("t"), AssigneeID: ptr[int64](outsiderID)}, domain.ErrAssigneeNotMember},
		{"title is required", memberID, teamID, domain.TaskPatch{}, domain.ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newTasksEnv()

			got, err := env.svc.Create(context.Background(), tt.actor, tt.team, tt.patch)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Empty(t, env.cache.invalidations())
				return
			}
			require.NoError(t, err)
			require.Equal(t, int64(tt.actor), got.CreatedBy)
			require.Equal(t, domain.StatusTodo, got.Status)

			history := env.tasks.recorded()
			require.Len(t, history, 1)
			require.Equal(t, got.ID, history[0].TaskID)
			require.Equal(t, []int64{teamID}, env.cache.invalidations())
		})
	}
}

func TestTasksVisibility(t *testing.T) {
	t.Parallel()
	env := newTasksEnv()
	ctx := context.Background()

	_, err := env.svc.Get(ctx, outsiderID, taskID)
	require.ErrorIs(t, err, domain.ErrTaskNotFound)

	_, err = env.svc.History(ctx, outsiderID, taskID, domain.PageRequest{Limit: 10})
	require.ErrorIs(t, err, domain.ErrTaskNotFound)

	_, err = env.svc.Comment(ctx, outsiderID, taskID, "hello")
	require.ErrorIs(t, err, domain.ErrTaskNotFound)

	_, err = env.svc.List(ctx, outsiderID, domain.TaskFilter{TeamID: teamID})
	require.ErrorIs(t, err, domain.ErrTeamNotFound)

	page, err := env.svc.List(ctx, memberID, domain.TaskFilter{TeamID: teamID})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)

	_, err = env.svc.Comment(ctx, memberID, taskID, "hello")
	require.NoError(t, err)
}
