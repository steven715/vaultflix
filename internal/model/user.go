package model

import "time"

// StreamTokenScope is the "scope" claim value on tokens minted for <video>
// streaming. Such tokens are accepted only on the streaming route and only for
// the video they were issued for (enforced in the auth middleware).
const StreamTokenScope = "stream"

type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	DisabledAt   *time.Time `json:"disabled_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
