package domain

import "time"

type AccessToken struct {
	Value     string
	ExpiresAt time.Time
}
