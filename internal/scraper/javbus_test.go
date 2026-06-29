package scraper

import (
	"os"
	"testing"
)

func TestParseJavBus_DASD626(t *testing.T) {
	html, err := os.ReadFile("testdata/javbus_dasd626.html")
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseJavBus(html, "DASD-626")
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "DASD-626" {
		t.Errorf("code = %q", got.Code)
	}
	if got.Title == "" {
		t.Error("title empty")
	}
	if got.Maker == "" {
		t.Error("maker empty")
	}
	if len(got.Actresses) == 0 {
		t.Error("expected at least one actress")
	}
	if len(got.Genres) == 0 {
		t.Error("expected genres")
	}
	if got.CoverURL == "" {
		t.Error("cover url empty")
	}
}
