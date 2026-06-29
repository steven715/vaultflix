package scraper

import (
	"errors"
	"os"
	"testing"

	"github.com/steven/vaultflix/internal/model"
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

func TestParseJavBus_AgeGate_ReturnsBlocked(t *testing.T) {
	ageGateHTML := []byte(`<!DOCTYPE html><html><head><title>Age Verification</title></head><body><p>You must verify your age.</p></body></html>`)
	_, err := parseJavBus(ageGateHTML, "DASD-626")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, model.ErrScrapeBlocked) {
		t.Errorf("expected ErrScrapeBlocked, got %v", err)
	}
}
