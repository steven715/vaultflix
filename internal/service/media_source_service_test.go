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

// helper: create a temp dir structure for path validation tests.
// Returns the mount prefix (e.g., "/tmp/xxx/mnt/host/") and a valid subdir path.
func setupTempMount(t *testing.T) (prefix string, validPath string) {
	t.Helper()
	base := t.TempDir()
	mountHost := filepath.Join(base, "mnt", "host")
	subDir := filepath.Join(mountHost, "Videos")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	// prefix must end with separator to match filepath.Clean behavior
	return mountHost + string(filepath.Separator), subDir
}

func TestCreateMediaSource_Success(t *testing.T) {
	prefix, validPath := setupTempMount(t)

	repo := &mock.MediaSourceRepository{
		CreateFunc: func(ctx context.Context, source *model.MediaSource) error {
			source.ID = "ms-1"
			return nil
		},
	}
	svc := NewMediaSourceService(repo, prefix)

	source := &model.MediaSource{Label: "Videos", MountPath: validPath}
	err := svc.Create(context.Background(), source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source.ID != "ms-1" {
		t.Errorf("expected id ms-1, got %s", source.ID)
	}
}

func TestCreateMediaSource_PathOutsidePrefix(t *testing.T) {
	prefix, _ := setupTempMount(t)

	svc := NewMediaSourceService(&mock.MediaSourceRepository{}, prefix)

	source := &model.MediaSource{Label: "Bad", MountPath: "/etc/passwd"}
	err := svc.Create(context.Background(), source)
	if !errors.Is(err, model.ErrPathNotAllowed) {
		t.Fatalf("expected ErrPathNotAllowed, got %v", err)
	}
}

func TestCreateMediaSource_PathContainsDotDot(t *testing.T) {
	prefix, validPath := setupTempMount(t)

	svc := NewMediaSourceService(&mock.MediaSourceRepository{}, prefix)

	// path with .. that resolves outside prefix
	dotdotPath := validPath + "/../../../etc"
	source := &model.MediaSource{Label: "Bad", MountPath: dotdotPath}
	err := svc.Create(context.Background(), source)
	if !errors.Is(err, model.ErrPathNotAllowed) {
		t.Fatalf("expected ErrPathNotAllowed, got %v", err)
	}
}

func TestCreateMediaSource_PathNotExist(t *testing.T) {
	prefix, _ := setupTempMount(t)

	svc := NewMediaSourceService(&mock.MediaSourceRepository{}, prefix)

	source := &model.MediaSource{Label: "Gone", MountPath: filepath.Join(prefix, "nonexistent")}
	err := svc.Create(context.Background(), source)
	if !errors.Is(err, model.ErrPathNotExist) {
		t.Fatalf("expected ErrPathNotExist, got %v", err)
	}
}

func TestCreateMediaSource_PathNotDirectory(t *testing.T) {
	prefix, validPath := setupTempMount(t)

	// create a file inside the valid dir
	filePath := filepath.Join(validPath, "video.mp4")
	if err := os.WriteFile(filePath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	svc := NewMediaSourceService(&mock.MediaSourceRepository{}, prefix)

	source := &model.MediaSource{Label: "File", MountPath: filePath}
	err := svc.Create(context.Background(), source)
	if !errors.Is(err, model.ErrPathNotAllowed) {
		t.Fatalf("expected ErrPathNotAllowed, got %v", err)
	}
}

func TestCreateMediaSource_DuplicatePath(t *testing.T) {
	prefix, validPath := setupTempMount(t)

	repo := &mock.MediaSourceRepository{
		CreateFunc: func(ctx context.Context, source *model.MediaSource) error {
			return model.ErrAlreadyExists
		},
	}
	svc := NewMediaSourceService(repo, prefix)

	source := &model.MediaSource{Label: "Dup", MountPath: validPath}
	err := svc.Create(context.Background(), source)
	if !errors.Is(err, model.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestUpdateMediaSource_Success(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return &model.MediaSource{ID: id, Label: "Old", MountPath: "/mnt/host/X", Enabled: true}, nil
		},
		UpdateFunc: func(ctx context.Context, source *model.MediaSource) error {
			return nil
		},
	}
	svc := NewMediaSourceService(repo, "/mnt/host/")

	err := svc.Update(context.Background(), "ms-1", "New Label", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateMediaSource_NotFound(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return nil, model.ErrNotFound
		},
	}
	svc := NewMediaSourceService(repo, "/mnt/host/")

	err := svc.Update(context.Background(), "nope", "Label", true)
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteMediaSource_Success(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}
	svc := NewMediaSourceService(repo, "/mnt/host/")

	err := svc.Delete(context.Background(), "ms-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteMediaSource_NotFound(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			return model.ErrNotFound
		},
	}
	svc := NewMediaSourceService(repo, "/mnt/host/")

	err := svc.Delete(context.Background(), "nope")
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
