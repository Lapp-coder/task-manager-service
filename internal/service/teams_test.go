package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lapp-coder/task-manager-service/internal/domain"
	"github.com/lapp-coder/task-manager-service/internal/service"
)

const newcomerEmail = "newcomer@example.com"

func newTeamsService() *service.Teams {
	teams := newFakeTeams(
		domain.Member{TeamID: teamID, UserID: ownerID, Role: domain.RoleOwner},
		domain.Member{TeamID: teamID, UserID: adminID, Role: domain.RoleAdmin},
		domain.Member{TeamID: teamID, UserID: memberID, Role: domain.RoleMember},
	)
	users := fakeUsers{
		newcomerEmail:        {ID: 42, Email: newcomerEmail, Name: "Newcomer"},
		"member@example.com": {ID: memberID, Email: "member@example.com"},
	}
	return service.NewTeams(fakeTx{}, teams, users, fakeStats{})
}

func TestTeamsInvite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actor   int64
		email   string
		wantErr error
	}{
		{"owner invites", ownerID, newcomerEmail, nil},
		{"admin invites", adminID, newcomerEmail, nil},
		{"member cannot invite", memberID, newcomerEmail, domain.ErrForbidden},
		{"outsider sees no team", outsiderID, newcomerEmail, domain.ErrTeamNotFound},
		{"unknown user", ownerID, "ghost@example.com", domain.ErrUserNotFound},
		{"existing member", ownerID, "member@example.com", domain.ErrAlreadyMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, err := newTeamsService().Invite(context.Background(), tt.actor, teamID, tt.email, domain.RoleMember)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, domain.RoleMember, m.Role)
			require.Equal(t, int64(42), m.UserID)
		})
	}
}

func TestTeamsChangeRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actor   int64
		target  int64
		wantErr error
	}{
		{"owner promotes member", ownerID, memberID, nil},
		{"owner cannot change own role", ownerID, ownerID, domain.ErrForbidden},
		{"admin cannot change owner role", adminID, ownerID, domain.ErrForbidden},
		{"admin cannot change roles", adminID, memberID, domain.ErrForbidden},
		{"unknown target", ownerID, outsiderID, domain.ErrMemberNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, err := newTeamsService().ChangeRole(context.Background(), tt.actor, teamID, tt.target, domain.RoleAdmin)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, domain.RoleAdmin, m.Role)
		})
	}
}

func TestTeamsStatsAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actor   int64
		wantErr error
	}{
		{"owner", ownerID, nil},
		{"admin", adminID, nil},
		{"member", memberID, domain.ErrForbidden},
		{"outsider", outsiderID, domain.ErrTeamNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stats, err := newTeamsService().Stats(context.Background(), tt.actor, teamID)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, int64(teamID), stats.TeamID)
		})
	}
}
