package service

import (
	"path/filepath"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// validateMountedFilePath ensures a constructed media file path stays within
// AllowedMountPrefix before it is handed to ffmpeg/ffprobe. This is
// defence-in-depth: media-source mount paths are validated at creation time,
// but the joined (mount_path + db file_path) result is re-checked here so a
// traversal component in either part cannot escape the allowed mount.
//
// The input is expected to be a filepath.Join result (already Clean'd), so any
// ".." is collapsed before the prefix check; an escape therefore lands outside
// the prefix and is rejected. Returns model.ErrPathNotAllowed on violation.
func validateMountedFilePath(path string) error {
	cleaned := filepath.Clean(path)
	base := filepath.Clean(AllowedMountPrefix)
	if cleaned != base && !strings.HasPrefix(cleaned, base+string(filepath.Separator)) {
		return model.ErrPathNotAllowed
	}
	return nil
}
