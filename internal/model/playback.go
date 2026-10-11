package model

// PlaybackSource is where a Video's bytes are read from, and how they play.
// Exactly one of Path and LegacyObjectKey is set.
type PlaybackSource struct {
	// Path is the validated on-disk file of a Video on a Media Source.
	Path string
	// LegacyObjectKey is the MinIO object of a Video stored before Media
	// Sources existed.
	LegacyObjectKey string
	PlayMode        PlayMode
	MimeType        string
}

// IsLegacy reports whether the bytes are a MinIO object rather than a file.
func (p *PlaybackSource) IsLegacy() bool {
	return p.LegacyObjectKey != ""
}
