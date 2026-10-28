package token

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

const issuer = "task-manager-service"

type JWT struct {
	secret []byte
	ttl    time.Duration
	parser *jwt.Parser
}

func NewJWT(secret string, ttl time.Duration) *JWT {
	return &JWT{
		secret: []byte(secret),
		ttl:    ttl,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(issuer),
			jwt.WithExpirationRequired(),
		),
	}
}

func (j *JWT) Issue(userID int64) (domain.AccessToken, error) {
	issuedAt := time.Now()
	expiresAt := issuedAt.Add(j.ttl)
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   strconv.FormatInt(userID, 10),
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.secret)
	if err != nil {
		return domain.AccessToken{}, fmt.Errorf("sign token: %w", err)
	}
	return domain.AccessToken{Value: signed, ExpiresAt: expiresAt}, nil
}

func (j *JWT) Parse(raw string) (int64, error) {
	var claims jwt.RegisteredClaims
	if _, err := j.parser.ParseWithClaims(raw, &claims, j.key); err != nil {
		return 0, errors.Join(domain.ErrUnauthorized, err)
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return 0, fmt.Errorf("%w: malformed subject", domain.ErrUnauthorized)
	}
	return userID, nil
}

func (j *JWT) key(*jwt.Token) (any, error) {
	return j.secret, nil
}
