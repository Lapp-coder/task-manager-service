package service

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, u domain.User) (int64, error)
	ByEmail(ctx context.Context, email string) (domain.User, error)
}

type TokenIssuer interface {
	Issue(userID int64) (domain.AccessToken, error)
}

type Auth struct {
	users  UserRepository
	tokens TokenIssuer
}

func NewAuth(users UserRepository, tokens TokenIssuer) *Auth {
	return &Auth{users: users, tokens: tokens}
}

type Registration struct {
	Email    string
	Password string
	Name     string
}

func (a *Auth) Register(ctx context.Context, r Registration) (domain.User, error) {
	email, err := domain.NormalizeEmail(r.Email)
	if err != nil {
		return domain.User{}, err
	}
	name, err := domain.NormalizeName("name", r.Name)
	if err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidatePassword(r.Password); err != nil {
		return domain.User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(r.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}

	u := domain.User{Email: email, Name: name, PasswordHash: string(hash), CreatedAt: now()}
	if u.ID, err = a.users.Create(ctx, u); err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (a *Auth) Login(ctx context.Context, email, password string) (domain.AccessToken, error) {
	email, err := domain.NormalizeEmail(email)
	if err != nil {
		return domain.AccessToken{}, domain.ErrInvalidCredentials
	}
	u, err := a.users.ByEmail(ctx, email)
	if errors.Is(err, domain.ErrUserNotFound) {
		return domain.AccessToken{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return domain.AccessToken{}, fmt.Errorf("get user by email: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return domain.AccessToken{}, domain.ErrInvalidCredentials
	}

	token, err := a.tokens.Issue(u.ID)
	if err != nil {
		return domain.AccessToken{}, fmt.Errorf("issue token: %w", err)
	}
	return token, nil
}
