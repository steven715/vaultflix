package service

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func TestValidateMountedFilePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		allowed bool
	}{
		{"file under mount", "/mnt/host/D/movie.mp4", true},
		{"mount root itself", "/mnt/host", true},
		{"sibling prefix collision", "/mnt/hostile/movie.mp4", false},
		{"outside mount", "/etc/passwd", false},
		{"traversal escaping mount", filepath.Join("/mnt/host/D", "../../etc/passwd"), false},
		{"in-bounds traversal", filepath.Join("/mnt/host/D/sub", "../movie.mp4"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMountedFilePath(tt.path)
			if tt.allowed && err != nil {
				t.Fatalf("expected %q allowed, got %v", tt.path, err)
			}
			if !tt.allowed && !errors.Is(err, model.ErrPathNotAllowed) {
				t.Fatalf("expected %q rejected with ErrPathNotAllowed, got %v", tt.path, err)
			}
		})
	}
}
