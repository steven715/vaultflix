package service

import (
	"fmt"
	"strings"
)

// Object key prefixes of the images Enrichment stages; stagedKey also uses
// them to recognise keys in legacy Suggestions.
const (
	coverKeyPrefix  = "covers/"
	avatarKeyPrefix = "actresses/"
)

// The object key of every asset kind lives here, so a key format is decided
// once and is testable on its own.

// thumbnailKey is the key of a Video's Thumbnail.
func thumbnailKey(videoID string) string {
	return fmt.Sprintf("thumbnails/%s.jpg", videoID)
}

// previewKey is the key of a Video's Preview.
func previewKey(videoID string) string {
	return fmt.Sprintf("previews/%s.mp4", videoID)
}

// coverKey is the key of a Cover staged from source for Code. It is scoped
// by Code and source so staging never overwrites an accepted Cover (ADR-0010).
func coverKey(code, source string) string {
	return fmt.Sprintf("%s%s-%s.jpg", coverKeyPrefix, code, source)
}

// avatarKey is the key of a Performer's Avatar staged from source for Code.
// Scoped like coverKey: a bare per-name key would overwrite the Avatar an
// existing Performer points to before the Suggestion is accepted.
func avatarKey(performerName, code, source string) string {
	return fmt.Sprintf("%s%s-%s-%s.jpg", avatarKeyPrefix, sanitizeName(performerName), code, source)
}

// sanitizeName replaces characters that are unsafe in object key paths with
// underscores.
func sanitizeName(name string) string {
	return strings.NewReplacer("/", "_", " ", "_", ".", "_").Replace(name)
}
