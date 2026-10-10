package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

// setupResolveFixture creates <tmp>/mnt/host/D/movie.mp4 and returns the
// injected mount prefix plus a repo serving one enabled Media Source at
// <tmp>/mnt/host/D with id "ms-1".
func setupResolveFixture(t *testing.T) (prefix, mount string, repo *mock.MediaSourceRepository) {
	t.Helper()
	prefix, _ = setupTempMount(t)
	mount = filepath.Join(prefix, "D")
	if err := os.MkdirAll(filepath.Join(mount, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mount, "movie.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	source := &model.MediaSource{ID: "ms-1", MountPath: mount, Enabled: true}
	repo = &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			if id != source.ID {
				return nil, model.ErrNotFound
			}
			return source, nil
		},
	}
	return prefix, mount, repo
}

func TestResolveFile_FileUnderMount(t *testing.T) {
	prefix, mount, repo := setupResolveFixture(t)
	svc := NewMediaSourceService(repo, prefix)

	got, err := svc.ResolveFile(context.Background(), "ms-1", "movie.mp4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(mount, "movie.mp4"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveFile_MediaSourceDisabled(t *testing.T) {
	prefix, _, repo := setupResolveFixture(t)
	source, _ := repo.FindByID(context.Background(), "ms-1")
	source.Enabled = false
	svc := NewMediaSourceService(repo, prefix)

	_, err := svc.ResolveFile(context.Background(), "ms-1", "movie.mp4")
	if !errors.Is(err, model.ErrMediaSourceDisabled) {
		t.Fatalf("expected ErrMediaSourceDisabled, got %v", err)
	}
}

func TestResolveFile_PathNotAllowed(t *testing.T) {
	tests := []struct {
		name      string
		mountPath func(prefix, mount string) string
		filePath  string
	}{
		{"file path escapes into sibling mount", nil, "../E/movie.mp4"},
		{"file path escapes mount prefix", nil, "../../../../etc/passwd"},
		{"sibling-prefix mount collision", func(_, mount string) string { return mount + "-extra" }, "../D/movie.mp4"},
		{"mount outside allowed prefix", func(_, _ string) string { return "/etc" }, "passwd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, mount, repo := setupResolveFixture(t)
			if tt.mountPath != nil {
				source, _ := repo.FindByID(context.Background(), "ms-1")
				source.MountPath = tt.mountPath(prefix, mount)
			}
			svc := NewMediaSourceService(repo, prefix)

			_, err := svc.ResolveFile(context.Background(), "ms-1", tt.filePath)
			if !errors.Is(err, model.ErrPathNotAllowed) {
				t.Fatalf("expected ErrPathNotAllowed, got %v", err)
			}
		})
	}
}

func TestResolveFile_InBoundsTraversal(t *testing.T) {
	prefix, mount, repo := setupResolveFixture(t)
	svc := NewMediaSourceService(repo, prefix)

	got, err := svc.ResolveFile(context.Background(), "ms-1", "sub/../movie.mp4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(mount, "movie.mp4"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveFile_FileNotExist(t *testing.T) {
	prefix, _, repo := setupResolveFixture(t)
	svc := NewMediaSourceService(repo, prefix)

	_, err := svc.ResolveFile(context.Background(), "ms-1", "missing.mp4")
	if !errors.Is(err, model.ErrPathNotExist) {
		t.Fatalf("expected ErrPathNotExist, got %v", err)
	}
}

func TestResolveFile_MediaSourceNotFound(t *testing.T) {
	prefix, _, repo := setupResolveFixture(t)
	svc := NewMediaSourceService(repo, prefix)

	_, err := svc.ResolveFile(context.Background(), "ms-missing", "movie.mp4")
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
