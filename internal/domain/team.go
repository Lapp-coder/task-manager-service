package domain

import (
	"fmt"
	"time"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

func ParseInvitableRole(s string) (Role, error) {
	switch r := Role(s); r {
	case RoleAdmin, RoleMember:
		return r, nil
	default:
		return "", fmt.Errorf("%w: role must be admin or member", ErrInvalidInput)
	}
}

func (r Role) managesTeam() bool {
	return r == RoleOwner || r == RoleAdmin
}

type Team struct {
	ID        int64
	Name      string
	CreatedBy int64
	CreatedAt time.Time
}

type Membership struct {
	Team Team
	Role Role
}

type Member struct {
	TeamID   int64
	UserID   int64
	Role     Role
	JoinedAt time.Time
}

type TeamMember struct {
	Member
	Email string
	Name  string
}

func (m Member) CanInvite() error {
	if !m.Role.managesTeam() {
		return fmt.Errorf("%w: only owner or admin can invite", ErrForbidden)
	}
	return nil
}

func (m Member) CanViewStats() error {
	if !m.Role.managesTeam() {
		return fmt.Errorf("%w: only owner or admin can view team stats", ErrForbidden)
	}
	return nil
}

func (m Member) CanChangeRole(target Member) error {
	if m.Role != RoleOwner {
		return fmt.Errorf("%w: only owner can change roles", ErrForbidden)
	}
	if target.Role == RoleOwner {
		return fmt.Errorf("%w: owner role cannot be changed", ErrForbidden)
	}
	return nil
}
