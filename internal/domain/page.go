package domain

import "fmt"

const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
)

type PageRequest struct {
	Limit   int
	Offset  int
	AfterID int64
}

func NewPageRequest(limit, offset int, afterID int64) (PageRequest, error) {
	switch {
	case limit == 0:
		limit = DefaultPageLimit
	case limit < 0 || limit > MaxPageLimit:
		return PageRequest{}, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidInput, MaxPageLimit)
	}
	if offset < 0 {
		return PageRequest{}, fmt.Errorf("%w: offset must not be negative", ErrInvalidInput)
	}
	if afterID < 0 {
		return PageRequest{}, fmt.Errorf("%w: cursor is malformed", ErrInvalidInput)
	}
	if afterID > 0 && offset > 0 {
		return PageRequest{}, fmt.Errorf("%w: cursor and offset are mutually exclusive", ErrInvalidInput)
	}
	return PageRequest{Limit: limit, Offset: offset, AfterID: afterID}, nil
}

type Page[T any] struct {
	Items      []T
	NextCursor int64
}
