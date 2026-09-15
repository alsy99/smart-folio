package config

import (
	"os"
	"testing"
	"time"
)

func TestStringFallback(t *testing.T) {
	t.Setenv("APERTURE_TEST_STR", "")
	if got := String("APERTURE_TEST_STR", "def"); got != "def" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("APERTURE_TEST_STR", "  live  ")
	if got := String("APERTURE_TEST_STR", "def"); got != "live" {
		t.Fatalf("got %q", got)
	}
}

func TestBool(t *testing.T) {
	t.Setenv("APERTURE_TEST_BOOL", "true")
	if !Bool("APERTURE_TEST_BOOL") {
		t.Fatal("expected true")
	}
	os.Unsetenv("APERTURE_TEST_BOOL")
	if Bool("APERTURE_TEST_BOOL") {
		t.Fatal("expected false")
	}
	if !BoolDefault("APERTURE_MISSING_LLM", true) {
		t.Fatal("default true")
	}
	t.Setenv("APERTURE_MISSING_LLM", "false")
	if BoolDefault("APERTURE_MISSING_LLM", true) {
		t.Fatal("explicit false")
	}
}

func TestDurationSeconds(t *testing.T) {
	t.Setenv("APERTURE_TEST_DUR", "900")
	if got := Duration("APERTURE_TEST_DUR", time.Second); got != 900*time.Second {
		t.Fatalf("got %s", got)
	}
}
