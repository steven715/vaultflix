package avid

import "testing"

func TestExtractCode_Standard(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"plain", "DASD-626.mp4", "DASD-626", true},
		{"lowercase", "ssis-001.mkv", "SSIS-001", true},
		{"underscore", "MIDE_888.mp4", "MIDE-888", true},
		{"with resolution", "SSIS-123-1080p.mp4", "SSIS-123", true},
		{"website watermark", "hhd800.com@DASD-700.mp4", "DASD-700", true},
		{"chinese sub suffix", "STARS-256-C.mp4", "STARS-256", true},
		{"multi-disc", "ABP-999-cd2.mp4", "ABP-999", true},
		{"fc2 ppv", "FC2-PPV-1234567.mp4", "FC2-PPV-1234567", true},
		{"fc2 short", "FC2-1234567.mp4", "FC2-PPV-1234567", true},
		{"amateur numeric prefix luxu", "259LUXU-1234.mp4", "259LUXU-1234", true},
		{"amateur numeric prefix gana", "200gana-1888.mp4", "200GANA-1888", true},
		{"no code", "家庭聚會影片.mp4", "", false},
		{"random digits only", "20240101_backup.mp4", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ExtractCode(c.in)
			if ok != c.ok || got != c.want {
				t.Fatalf("ExtractCode(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestClean_StripsNoise(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hhd800.com@SSIS-001-1080p-C.mp4", "SSIS-001"},
		{"[www.example.com]ABP-123.mkv", "ABP-123"},
		{"MIDE-888-cd1.mp4", "MIDE-888"},
	}
	for _, c := range cases {
		if got := Clean(c.in); got != c.want {
			t.Errorf("Clean(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
