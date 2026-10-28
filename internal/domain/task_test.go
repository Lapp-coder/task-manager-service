package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

const (
	teamID     = 1
	creatorID  = 10
	assigneeID = 20
	strangerID = 30
)

func ptr[T any](v T) *T { return &v }

func TestMemberCanEditTask(t *testing.T) {
	t.Parallel()

	task := domain.Task{ID: 100, TeamID: teamID, CreatedBy: creatorID, AssigneeID: assigneeID, Status: domain.StatusTodo}
	statusOnly := domain.TaskChanges{Status: &domain.Diff[domain.Status]{Old: domain.StatusTodo, New: domain.StatusDone}}
	titleChange := domain.TaskChanges{Title: &domain.Diff[string]{Old: "a", New: "b"}}
	reassign := domain.TaskChanges{AssigneeID: &domain.Diff[int64]{Old: assigneeID, New: strangerID}}

	tests := []struct {
		name    string
		member  domain.Member
		changes domain.TaskChanges
		wantErr error
	}{
		{"owner edits any field", member(strangerID, domain.RoleOwner), titleChange, nil},
		{"admin reassigns foreign task", member(strangerID, domain.RoleAdmin), reassign, nil},
		{"creator changes title", member(creatorID, domain.RoleMember), titleChange, nil},
		{"creator reassigns", member(creatorID, domain.RoleMember), reassign, nil},
		{"assignee changes status", member(assigneeID, domain.RoleMember), statusOnly, nil},
		{"assignee cannot change title", member(assigneeID, domain.RoleMember), titleChange, domain.ErrForbidden},
		{"assignee cannot reassign", member(assigneeID, domain.RoleMember), reassign, domain.ErrForbidden},
		{"unrelated member cannot change status", member(strangerID, domain.RoleMember), statusOnly, domain.ErrForbidden},
		{"admin of another team sees nothing", domain.Member{TeamID: 2, UserID: strangerID, Role: domain.RoleAdmin}, statusOnly, domain.ErrTaskNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, tt.member.CanEditTask(task, tt.changes), tt.wantErr)
		})
	}
}

func TestMemberTeamPermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role      domain.Role
		canManage bool
	}{
		{domain.RoleOwner, true},
		{domain.RoleAdmin, true},
		{domain.RoleMember, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			t.Parallel()
			m := member(creatorID, tt.role)
			if tt.canManage {
				require.NoError(t, m.CanInvite())
				require.NoError(t, m.CanViewStats())
				return
			}
			require.ErrorIs(t, m.CanInvite(), domain.ErrForbidden)
			require.ErrorIs(t, m.CanViewStats(), domain.ErrForbidden)
		})
	}
}

func TestMemberCanChangeRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actor   domain.Role
		target  domain.Role
		wantErr error
	}{
		{"owner promotes member", domain.RoleOwner, domain.RoleMember, nil},
		{"owner demotes admin", domain.RoleOwner, domain.RoleAdmin, nil},
		{"owner role is immutable", domain.RoleOwner, domain.RoleOwner, domain.ErrForbidden},
		{"admin cannot change owner", domain.RoleAdmin, domain.RoleOwner, domain.ErrForbidden},
		{"admin cannot change member", domain.RoleAdmin, domain.RoleMember, domain.ErrForbidden},
		{"member cannot change roles", domain.RoleMember, domain.RoleMember, domain.ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := member(creatorID, tt.actor).CanChangeRole(member(assigneeID, tt.target))
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestParseInvitableRole(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"admin", "member"} {
		role, err := domain.ParseInvitableRole(raw)
		require.NoError(t, err)
		require.Equal(t, domain.Role(raw), role)
	}
	for _, raw := range []string{"owner", "", "root"} {
		_, err := domain.ParseInvitableRole(raw)
		require.ErrorIs(t, err, domain.ErrInvalidInput)
	}
}

func TestTaskApplyStatusTransitions(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	closed := created.Add(time.Hour)
	later := closed.Add(time.Hour)

	tests := []struct {
		name       string
		task       domain.Task
		status     domain.Status
		wantClosed time.Time
	}{
		{"closing sets closed_at", domain.Task{Status: domain.StatusInProgress}, domain.StatusDone, later},
		{"repeated done keeps closed_at", domain.Task{Status: domain.StatusDone, ClosedAt: closed}, domain.StatusDone, closed},
		{"reopening clears closed_at", domain.Task{Status: domain.StatusDone, ClosedAt: closed}, domain.StatusTodo, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.task.Apply(domain.TaskPatch{Status: ptr(tt.status)}, later)
			require.NoError(t, err)
			require.Equal(t, tt.status, got.Status)
			require.Equal(t, tt.wantClosed, got.ClosedAt)
		})
	}
}

func TestNewTaskValidation(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name  string
		patch domain.TaskPatch
	}{
		{"title is required", domain.TaskPatch{}},
		{"blank title", domain.TaskPatch{Title: ptr("   ")}},
		{"too long description", domain.TaskPatch{Title: ptr("ok"), Description: ptr(string(make([]rune, 10001)))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := domain.NewTask(teamID, creatorID, tt.patch, now)
			require.ErrorIs(t, err, domain.ErrInvalidInput)
		})
	}
}

func TestChanges(t *testing.T) {
	t.Parallel()

	before := domain.Task{Title: "a", Description: "d", Status: domain.StatusTodo, AssigneeID: assigneeID}
	after := before
	after.Status = domain.StatusDone
	after.AssigneeID = 0

	got := domain.Changes(before, after)
	require.Equal(t, domain.TaskChanges{
		Status:     &domain.Diff[domain.Status]{Old: domain.StatusTodo, New: domain.StatusDone},
		AssigneeID: &domain.Diff[int64]{Old: assigneeID, New: 0},
	}, got)
	require.True(t, domain.Changes(before, before).Empty())
}

func member(userID int64, role domain.Role) domain.Member {
	return domain.Member{TeamID: teamID, UserID: userID, Role: role}
}
