package model

import "time"

type MediaSource struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	MountPath string    `json:"mount_path"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
