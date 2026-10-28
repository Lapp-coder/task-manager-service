package domain

import "errors"

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrForbidden          = errors.New("forbidden")
	ErrEmailTaken         = errors.New("email already registered")
	ErrUserNotFound       = errors.New("user not found")
	ErrTeamNotFound       = errors.New("team not found")
	ErrTaskNotFound       = errors.New("task not found")
	ErrMemberNotFound     = errors.New("member not found")
	ErrAlreadyMember      = errors.New("user is already a team member")
	ErrAssigneeNotMember  = errors.New("assignee is not a team member")
	ErrVersionConflict    = errors.New("task was modified concurrently")
)
