package service

import "testing"

func TestAssetKeys_Formats(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"thumbnail", thumbnailKey("v1"), "thumbnails/v1.jpg"},
		{"preview", previewKey("v1"), "previews/v1.mp4"},
		{"cover scoped by Code and source", coverKey("DASD-626", "javbus"), "covers/DASD-626-javbus.jpg"},
		{"avatar scoped by name, Code and source", avatarKey("山田 花子", "DASD-626", "javbus"), "actresses/山田_花子-DASD-626-javbus.jpg"},
		{"avatar name loses path separators and dots", avatarKey("a/b.c", "X-1", "s"), "actresses/a_b_c-X-1-s.jpg"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

// Every Cover/Avatar key carries its kind's prefix: stagedKey relies on it to
// recognise keys in legacy Suggestions.
func TestAssetKeys_PrefixesMatchKeys(t *testing.T) {
	if k := coverKey("X-1", "s"); k[:len(coverKeyPrefix)] != coverKeyPrefix {
		t.Errorf("cover key %q lacks prefix %q", k, coverKeyPrefix)
	}
	if k := avatarKey("A", "X-1", "s"); k[:len(avatarKeyPrefix)] != avatarKeyPrefix {
		t.Errorf("avatar key %q lacks prefix %q", k, avatarKeyPrefix)
	}
}
