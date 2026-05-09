package elevenlabs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListVoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/voices" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("xi-api-key") != "k" {
			t.Errorf("missing api key header")
		}
		_, _ = io.WriteString(w, `{"voices":[{"voice_id":"v1","name":"Rachel","category":"premade"}]}`)
	}))
	defer srv.Close()

	c := New("k")
	c.BaseURL = srv.URL
	v, err := c.ListVoices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 || v[0].VoiceID != "v1" {
		t.Fatalf("got %+v", v)
	}
}

func TestTTS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/text-to-speech/v1") {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("output_format") != "pcm_44100" {
			t.Errorf("format: %s", r.URL.Query().Get("output_format"))
		}
		w.Header().Set("Content-Type", "audio/x-pcm")
		_, _ = w.Write([]byte("AUDIO"))
	}))
	defer srv.Close()

	c := New("k")
	c.BaseURL = srv.URL
	data, ct, err := c.TTS(context.Background(), "v1", TTSRequest{Text: "hi"}, "pcm_44100", 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "AUDIO" || ct != "audio/x-pcm" {
		t.Fatalf("got %q %q", string(data), ct)
	}
}

func TestErrorPropagated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"detail":{"status":"unauthorized"}}`)
	}))
	defer srv.Close()
	c := New("bad")
	c.BaseURL = srv.URL
	if _, err := c.ListVoices(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
