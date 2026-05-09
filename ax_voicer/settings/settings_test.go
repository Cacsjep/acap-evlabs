package settings

import (
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	s, err := NewStore(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	cur := s.Get()
	if cur.OutputFormat == "" {
		t.Fatalf("expected defaults, got %+v", cur)
	}

	cur.APIKey = "key"
	cur.Volume = 0.7
	if _, err := s.Update(cur); err != nil {
		t.Fatalf("update: %v", err)
	}

	s2, err := NewStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if s2.Get().APIKey != "key" || s2.Get().Volume != 0.7 {
		t.Fatalf("not persisted: %+v", s2.Get())
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		mut  func(s *Settings)
		ok   bool
	}{
		{"defaults", func(*Settings) {}, true},
		{"bad stability", func(s *Settings) { s.Stability = 2 }, false},
		{"bad similarity", func(s *Settings) { s.SimilarityBoost = -1 }, false},
		{"bad volume", func(s *Settings) { s.Volume = -0.1 }, false},
		{"bad optimize", func(s *Settings) { s.OptimizeStreaming = 9 }, false},
		{"empty audio node ok", func(s *Settings) { s.AudioNode = "" }, true},
		{"unknown format", func(s *Settings) { s.OutputFormat = "wav_blah" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Defaults()
			tc.mut(&d)
			err := Validate(&d)
			if tc.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestRedact(t *testing.T) {
	d := Defaults()
	d.APIKey = "abc"
	r := Redact(d)
	if r.APIKey == "abc" {
		t.Fatalf("not redacted: %+v", r)
	}
}
