package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

type Teams struct {
	db *sql.DB
}

func NewTeams(db *sql.DB) *Teams {
	return &Teams{db: db}
}

func (r *Teams) Create(ctx context.Context, t domain.Team) (int64, error) {
	id, err := insert(ctx, conn(ctx, r.db),
		`INSERT INTO teams (name, created_by, created_at) VALUES (?, ?, ?)`,
		t.Name, t.CreatedBy, t.CreatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("insert team: %w", err)
	}
	return id, nil
}

func (r *Teams) AddMember(ctx context.Context, m domain.Member) error {
	_, err := conn(ctx, r.db).ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role, joined_at) VALUES (?, ?, ?, ?)`,
		m.TeamID, m.UserID, string(m.Role), m.JoinedAt,
	)
	if isDuplicate(err) {
		return domain.ErrAlreadyMember
	}
	if err != nil {
		return fmt.Errorf("insert team member: %w", err)
	}
	return nil
}

func (r *Teams) UpdateRole(ctx context.Context, m domain.Member) error {
	res, err := conn(ctx, r.db).ExecContext(ctx,
		`UPDATE team_members SET role = ? WHERE team_id = ? AND user_id = ?`,
		string(m.Role), m.TeamID, m.UserID,
	)
	if err != nil {
		return fmt.Errorf("update team member: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return domain.ErrMemberNotFound
	}
	return nil
}

func (r *Teams) Member(ctx context.Context, teamID, userID int64) (domain.Member, error) {
	var m domain.Member
	err := conn(ctx, r.db).QueryRowContext(ctx,
		`SELECT team_id, user_id, role, joined_at FROM team_members WHERE team_id = ? AND user_id = ?`,
		teamID, userID,
	).Scan(&m.TeamID, &m.UserID, &m.Role, &m.JoinedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Member{}, domain.ErrMemberNotFound
	}
	if err != nil {
		return domain.Member{}, fmt.Errorf("select team member: %w", err)
	}
	return m, nil
}

func (r *Teams) ListByUser(ctx context.Context, userID int64) ([]domain.Membership, error) {
	return queryAll(ctx, conn(ctx, r.db), scanMembership, `
		SELECT t.id, t.name, t.created_by, t.created_at, m.role
		FROM team_members m
		JOIN teams t ON t.id = m.team_id
		WHERE m.user_id = ?
		ORDER BY t.id`,
		userID,
	)
}

func (r *Teams) Members(ctx context.Context, teamID int64) ([]domain.TeamMember, error) {
	return queryAll(ctx, conn(ctx, r.db), scanTeamMember, `
		SELECT m.team_id, m.user_id, m.role, m.joined_at, u.email, u.name
		FROM team_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.team_id = ?
		ORDER BY FIELD(m.role, 'owner', 'admin', 'member'), m.joined_at, m.user_id`,
		teamID,
	)
}

func scanMembership(s scanner) (domain.Membership, error) {
	var m domain.Membership
	err := s.Scan(&m.Team.ID, &m.Team.Name, &m.Team.CreatedBy, &m.Team.CreatedAt, &m.Role)
	return m, err
}

func scanTeamMember(s scanner) (domain.TeamMember, error) {
	var m domain.TeamMember
	err := s.Scan(&m.TeamID, &m.UserID, &m.Role, &m.JoinedAt, &m.Email, &m.Name)
	return m, err
}
