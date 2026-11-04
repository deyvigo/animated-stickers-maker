package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func env(m map[string]string) LookupFunc {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestDefaults(t *testing.T) {
	s, err := LoadFrom("does-not-exist", env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if s.Port != 8000 || s.AnimatedMaxBytes != 500*1024 || s.JobTTL != 6*time.Hour {
		t.Fatalf("unexpected defaults: %+v", s)
	}
}

func TestEnvOverridesFileAndParsesHosts(t *testing.T) {
	dotenv := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(dotenv, []byte("# c\nPORT=9000\nJOB_TTL_HOURS=1.5\nALLOWED_HOSTS=\"a.com, B.com\"\n"), 0o644)

	s, err := LoadFrom(dotenv, env(map[string]string{"PORT": "9100"}))
	if err != nil {
		t.Fatal(err)
	}
	if s.Port != 9100 {
		t.Errorf("env should win over .env, got port %d", s.Port)
	}
	if s.JobTTL != 90*time.Minute {
		t.Errorf("JobTTL = %v", s.JobTTL)
	}
	if len(s.AllowedHosts) != 2 || s.AllowedHosts[1] != "b.com" {
		t.Errorf("AllowedHosts = %v", s.AllowedHosts)
	}
}

func TestInvalidValue(t *testing.T) {
	if _, err := LoadFrom("x", env(map[string]string{"PORT": "abc"})); err == nil {
		t.Fatal("expected error")
	}
}

func TestPaddingFill(t *testing.T) {
	s, err := LoadFrom("x", env(nil))
	if err != nil || s.PaddingFill != "blur" {
		t.Fatalf("default = %q, %v", s.PaddingFill, err)
	}
	s, err = LoadFrom("x", env(map[string]string{"PADDING_FILL": "transparent"}))
	if err != nil || s.PaddingFill != "transparent" {
		t.Fatalf("override = %q, %v", s.PaddingFill, err)
	}
	if _, err := LoadFrom("x", env(map[string]string{"PADDING_FILL": "pink"})); err == nil {
		t.Fatal("invalid value must be rejected")
	}
}
