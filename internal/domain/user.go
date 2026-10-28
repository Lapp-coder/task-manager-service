package domain

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	minPasswordLen = 8
	maxPasswordLen = 72
	maxNameLen     = 255
)

type User struct {
	ID           int64
	Email        string
	Name         string
	PasswordHash string
	CreatedAt    time.Time
}

func NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", fmt.Errorf("%w: email is malformed", ErrInvalidInput)
	}
	return email, nil
}

func ValidatePassword(password string) error {
	if n := len(password); n < minPasswordLen || n > maxPasswordLen {
		return fmt.Errorf("%w: password must be %d to %d bytes long", ErrInvalidInput, minPasswordLen, maxPasswordLen)
	}
	return nil
}

func NormalizeName(field, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return "", fmt.Errorf("%w: %s must be 1 to %d characters long", ErrInvalidInput, field, maxNameLen)
	}
	return name, nil
}
