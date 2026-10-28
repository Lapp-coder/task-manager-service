package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

type Users struct {
	db *sql.DB
}

func NewUsers(db *sql.DB) *Users {
	return &Users{db: db}
}

func (r *Users) Create(ctx context.Context, u domain.User) (int64, error) {
	id, err := insert(ctx, conn(ctx, r.db),
		`INSERT INTO users (email, password_hash, name, created_at) VALUES (?, ?, ?, ?)`,
		u.Email, u.PasswordHash, u.Name, u.CreatedAt,
	)
	if isDuplicate(err) {
		return 0, domain.ErrEmailTaken
	}
	if err != nil {
		return 0, fmt.Errorf("insert user: %w", err)
	}
	return id, nil
}

func (r *Users) ByEmail(ctx context.Context, email string) (domain.User, error) {
	var u domain.User
	err := conn(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, created_at FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("select user: %w", err)
	}
	return u, nil
}
