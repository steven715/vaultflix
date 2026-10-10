package mock

import (
	"context"
	"fmt"
	"path/filepath"
)

// MediaFileResolver is a hand-written fake for the service-side file resolver
// (MediaSourceService.ResolveFile).
type MediaFileResolver struct {
	ResolveFileFunc func(ctx context.Context, sourceID, filePath string) (string, error)
}

func (m *MediaFileResolver) ResolveFile(ctx context.Context, sourceID, filePath string) (string, error) {
	if m.ResolveFileFunc == nil {
		return "", fmt.Errorf("mock: ResolveFileFunc not set")
	}
	return m.ResolveFileFunc(ctx, sourceID, filePath)
}

// ResolveUnder returns a resolver that places every file path under mount and
// never fails.
func ResolveUnder(mount string) *MediaFileResolver {
	return &MediaFileResolver{
		ResolveFileFunc: func(_ context.Context, _, filePath string) (string, error) {
			return filepath.Join(mount, filePath), nil
		},
	}
}

// ResolveFailing returns a resolver that fails every call with err.
func ResolveFailing(err error) *MediaFileResolver {
	return &MediaFileResolver{
		ResolveFileFunc: func(_ context.Context, _, _ string) (string, error) {
			return "", err
		},
	}
}
