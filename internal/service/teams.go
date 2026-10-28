package service

import (
	"context"
	"fmt"
	"time"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

type TeamRepository interface {
	MemberRepository
	Create(ctx context.Context, t domain.Team) (int64, error)
	AddMember(ctx context.Context, m domain.Member) error
	UpdateRole(ctx context.Context, m domain.Member) error
	ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error)
	Members(ctx context.Context, teamID int64) ([]domain.TeamMember, error)
}

type UserFinder interface {
	ByEmail(ctx context.Context, email string) (domain.User, error)
}

type StatsRepository interface {
	TeamStats(ctx context.Context, teamID int64, now time.Time) (domain.TeamStats, error)
}

type Teams struct {
	tx    TxManager
	teams TeamRepository
	users UserFinder
	stats StatsRepository
}

func NewTeams(tx TxManager, teams TeamRepository, users UserFinder, stats StatsRepository) *Teams {
	return &Teams{tx: tx, teams: teams, users: users, stats: stats}
}

func (s *Teams) Create(ctx context.Context, actorID int64, name string) (domain.Team, error) {
	name, err := domain.NormalizeName("team name", name)
	if err != nil {
		return domain.Team{}, err
	}

	t := domain.Team{Name: name, CreatedBy: actorID, CreatedAt: now()}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		id, err := s.teams.Create(ctx, t)
		if err != nil {
			return fmt.Errorf("create team: %w", err)
		}
		t.ID = id
		owner := domain.Member{TeamID: t.ID, UserID: actorID, Role: domain.RoleOwner, JoinedAt: t.CreatedAt}
		if err := s.teams.AddMember(ctx, owner); err != nil {
			return fmt.Errorf("add team owner: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Team{}, err
	}
	return t, nil
}

func (s *Teams) List(ctx context.Context, actorID int64) ([]domain.Membership, error) {
	teams, err := s.teams.ListByUser(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("list teams of user %d: %w", actorID, err)
	}
	return teams, nil
}

func (s *Teams) Members(ctx context.Context, actorID, teamID int64) ([]domain.TeamMember, error) {
	if _, err := member(ctx, s.teams, teamID, actorID, domain.ErrTeamNotFound); err != nil {
		return nil, err
	}
	members, err := s.teams.Members(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("list members of team %d: %w", teamID, err)
	}
	return members, nil
}

func (s *Teams) Invite(ctx context.Context, actorID, teamID int64, email string, role domain.Role) (domain.TeamMember, error) {
	actor, err := member(ctx, s.teams, teamID, actorID, domain.ErrTeamNotFound)
	if err != nil {
		return domain.TeamMember{}, err
	}
	if err := actor.CanInvite(); err != nil {
		return domain.TeamMember{}, err
	}
	if email, err = domain.NormalizeEmail(email); err != nil {
		return domain.TeamMember{}, err
	}
	u, err := s.users.ByEmail(ctx, email)
	if err != nil {
		return domain.TeamMember{}, fmt.Errorf("get user by email: %w", err)
	}

	m := domain.Member{TeamID: teamID, UserID: u.ID, Role: role, JoinedAt: now()}
	if err := s.teams.AddMember(ctx, m); err != nil {
		return domain.TeamMember{}, fmt.Errorf("add member to team %d: %w", teamID, err)
	}
	return domain.TeamMember{Member: m, Email: u.Email, Name: u.Name}, nil
}

func (s *Teams) ChangeRole(ctx context.Context, actorID, teamID, userID int64, role domain.Role) (domain.Member, error) {
	var target domain.Member
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		actor, err := member(ctx, s.teams, teamID, actorID, domain.ErrTeamNotFound)
		if err != nil {
			return err
		}
		if target, err = member(ctx, s.teams, teamID, userID, domain.ErrMemberNotFound); err != nil {
			return err
		}
		if err := actor.CanChangeRole(target); err != nil {
			return err
		}
		target.Role = role
		if err := s.teams.UpdateRole(ctx, target); err != nil {
			return fmt.Errorf("update role: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Member{}, err
	}
	return target, nil
}

func (s *Teams) Stats(ctx context.Context, actorID, teamID int64) (domain.TeamStats, error) {
	actor, err := member(ctx, s.teams, teamID, actorID, domain.ErrTeamNotFound)
	if err != nil {
		return domain.TeamStats{}, err
	}
	if err := actor.CanViewStats(); err != nil {
		return domain.TeamStats{}, err
	}
	stats, err := s.stats.TeamStats(ctx, teamID, now())
	if err != nil {
		return domain.TeamStats{}, fmt.Errorf("team %d stats: %w", teamID, err)
	}
	return stats, nil
}
