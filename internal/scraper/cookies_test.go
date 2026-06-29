package scraper

import (
	"reflect"
	"testing"
)

func TestParseCookieHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{
			name:  "typical javbus age-gate cookies",
			input: "age=verified; existmag=all",
			want:  map[string]string{"age": "verified", "existmag": "all"},
		},
		{
			name:  "empty string returns empty map",
			input: "",
			want:  map[string]string{},
		},
		{
			name:  "value containing equals sign splits on first equals only",
			input: "token=a=b",
			want:  map[string]string{"token": "a=b"},
		},
		{
			name:  "whitespace around name and value is trimmed",
			input: "  foo = bar ;  baz = qux ",
			want:  map[string]string{"foo": "bar", "baz": "qux"},
		},
		{
			name:  "stray semicolons and empty pairs are skipped",
			input: "a=1;;b=2;",
			want:  map[string]string{"a": "1", "b": "2"},
		},
		{
			name:  "pair without equals sign is skipped",
			input: "good=ok; malformed; other=fine",
			want:  map[string]string{"good": "ok", "other": "fine"},
		},
		{
			name:  "single pair no semicolon",
			input: "session=xyz",
			want:  map[string]string{"session": "xyz"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCookieHeader(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseCookieHeader(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
