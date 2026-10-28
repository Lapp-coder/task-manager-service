package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type MemberRepository interface {
	Member(ctx context.Context, teamID, userID int64) (domain.Member, error)
}

func member(ctx context.Context, repo MemberRepository, teamID, userID int64, hidden error) (domain.Member, error) {
	m, err := repo.Member(ctx, teamID, userID)
	if errors.Is(err, domain.ErrMemberNotFound) {
		return domain.Member{}, hidden
	}
	if err != nil {
		return domain.Member{}, fmt.Errorf("get member of team %d: %w", teamID, err)
	}
	return m, nil
}

func now() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}
