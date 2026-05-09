//go:build mock

package controllers

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Cacsjep/voicer/ax_voicer/audio"
	"github.com/Cacsjep/voicer/ax_voicer/elevenlabs"
	"github.com/Cacsjep/voicer/ax_voicer/settings"
	"github.com/gofiber/fiber/v2"
)

//go:embed testdata/sine_440hz_500ms.mp3
var fixtureSineMP3 []byte

// recordingPlayer captures Play() calls so tests can assert on the bytes the
// controller hands to the audio backend after format decoding.
type recordingPlayer struct {
	mu    sync.Mutex
	calls []audio.PlayParams
}

func (r *recordingPlayer) Info(_ context.Context) audio.Info {
	return audio.Info{Implementation: "recording", HelperBuilt: true}
}

func (r *recordingPlayer) Play(_ context.Context, p audio.PlayParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, p)
	return nil
}

func (r *recordingPlayer) lastCall() (audio.PlayParams, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return audio.PlayParams{}, false
	}
	return r.calls[len(r.calls)-1], true
}

type testLogger struct{ t *testing.T }

func (l *testLogger) Infof(f string, a ...interface{})  { l.t.Logf("INFO: "+f, a...) }
func (l *testLogger) Errorf(f string, a ...interface{}) { l.t.Logf("ERR : "+f, a...) }

func newTestAPI(t *testing.T, base string) (*fiber.App, *API) {
	t.Helper()
	store, err := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	cur := store.Get()
	cur.APIKey = "secret"
	if _, err := store.Update(cur); err != nil {
		t.Fatal(err)
	}
	api := &API{
		Store:  store,
		Player: audio.New(),
		Log:    &testLogger{t: t},
		NewElevenLabs: func(apiKey string) *elevenlabs.Client {
			c := elevenlabs.New(apiKey)
			c.BaseURL = base
			return c
		},
	}
	app := fiber.New()
	app.Get("/api/settings", api.GetSettings)
	app.Post("/api/settings", api.UpdateSettings)
	app.Get("/api/audio/info", api.GetAudioInfo)
	app.Post("/api/test/play", api.Test)
	app.Post("/playvoice", api.PlayVoice)
	return app, api
}

func TestSettingsRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	app, _ := newTestAPI(t, srv.URL)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/settings", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte(`"********"`)) {
		t.Fatalf("expected redacted api_key in %s", body)
	}
}

func TestSettingsUpdatePreservesRedactedKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	app, api := newTestAPI(t, srv.URL)

	// Submit redacted value: api key should NOT be overwritten.
	body := `{"api_key":"********","voice_id":"new-voice"}`
	req := httptest.NewRequest("POST", "/api/settings", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if _, err := app.Test(req); err != nil {
		t.Fatal(err)
	}
	if api.Store.Get().APIKey != "secret" {
		t.Fatalf("api key clobbered: %q", api.Store.Get().APIKey)
	}
	if api.Store.Get().VoiceID != "new-voice" {
		t.Fatalf("voice not updated: %q", api.Store.Get().VoiceID)
	}
}

func TestAudioInfoMock(t *testing.T) {
	app, _ := newTestAPI(t, "")
	resp, err := app.Test(httptest.NewRequest("GET", "/api/audio/info", nil))
	if err != nil {
		t.Fatal(err)
	}
	var got audio.Info
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Implementation != "mock" || got.OutputCount == 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestPlayVoiceFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-pcm")
		// 1 second of silence at 44100 Hz mono s16 = 88200 bytes
		_, _ = w.Write(make([]byte, 88200))
	}))
	defer srv.Close()
	app, _ := newTestAPI(t, srv.URL)

	body := `{"text":"hello","output_format":"pcm_44100"}`
	req := httptest.NewRequest("POST", "/playvoice", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5_000)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["played"] != true {
		t.Fatalf("expected played=true, got %v", got)
	}
	if int(got["bytes"].(float64)) != 88200 {
		t.Fatalf("unexpected bytes: %v", got["bytes"])
	}
}

func TestPreferredFormatForTier(t *testing.T) {
	cases := []struct {
		tier    string
		current string
		want    string
	}{
		// Free tier: force mp3_* if anything else is set.
		{"free", "pcm_44100", "mp3_44100_128"},
		{"free", "mp3_44100_128", "mp3_44100_128"},
		{"free", "mp3_44100_64", "mp3_44100_64"}, // keep mp3 variants
		{"FREE", "pcm_44100", "mp3_44100_128"},   // case insensitive

		// Pro / paid: prefer pcm_44100. The format_fallback will
		// down-shift if PCM still isn't allowed.
		{"pro", "mp3_44100_128", "pcm_44100"},
		{"creator", "mp3_44100_128", "pcm_44100"},
		{"scale", "pcm_44100", "pcm_44100"},

		// Unknown / empty tier: leave the user's choice alone.
		{"", "pcm_44100", "pcm_44100"},
		{"", "mp3_44100_128", "mp3_44100_128"},
	}
	for _, tc := range cases {
		got := preferredFormatForTier(tc.tier, tc.current)
		if got != tc.want {
			t.Errorf("preferredFormatForTier(%q, %q) = %q, want %q",
				tc.tier, tc.current, got, tc.want)
		}
	}
}

// TestTestAPIKeyAutoPicksFormat is the integration-level cousin of the
// preferredFormatForTier table test: it walks the full /api/test/api_key
// flow and verifies that the saved settings.OutputFormat is updated based on
// the subscription tier returned by ElevenLabs.
func TestTestAPIKeyAutoPicksFormat(t *testing.T) {
	cases := []struct {
		name    string
		tier    string
		startAt string
		want    string
	}{
		{"free downgrades pcm to mp3", "free", "pcm_44100", "mp3_44100_128"},
		{"pro upgrades mp3 to pcm", "pro", "mp3_44100_128", "pcm_44100"},
		{"unknown leaves it alone", "", "mp3_44100_128", "mp3_44100_128"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/user/subscription" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				_, _ = io.WriteString(w, `{"tier":"`+tc.tier+`","character_count":0,"character_limit":10000,"status":"active"}`)
			}))
			defer srv.Close()

			store, err := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			cur := store.Get()
			cur.APIKey = "secret"
			cur.OutputFormat = tc.startAt
			if _, err := store.Update(cur); err != nil {
				t.Fatal(err)
			}

			api := &API{
				Store:  store,
				Player: audio.New(),
				Log:    &testLogger{t: t},
				NewElevenLabs: func(apiKey string) *elevenlabs.Client {
					c := elevenlabs.New(apiKey)
					c.BaseURL = srv.URL
					return c
				},
			}
			app := fiber.New()
			app.Post("/api/test/api_key", api.TestAPIKey)

			resp, err := app.Test(httptest.NewRequest("POST", "/api/test/api_key", nil))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("status %d body %s", resp.StatusCode, b)
			}
			if got := api.Store.Get().OutputFormat; got != tc.want {
				t.Errorf("settings.OutputFormat: got %q, want %q", got, tc.want)
			}

			// Response body should also include the (redacted) updated settings
			// so the frontend can sync without an extra GET /api/settings call.
			var got map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&got)
			s, ok := got["settings"].(map[string]any)
			if !ok {
				t.Fatalf("response missing settings field: %v", got)
			}
			if s["output_format"] != tc.want {
				t.Errorf("response settings.output_format: got %v, want %q", s["output_format"], tc.want)
			}
		})
	}
}

func TestNodePatternEscaping(t *testing.T) {
	cases := map[string]string{
		"":                    "^AudioDevice[0-9]+Output[0-9]+$",
		"AudioDevice0Output0": "^AudioDevice0Output0$",
		"my.node":             `^my\.node$`,
		"a+b":                 `^a\+b$`,
	}
	for in, want := range cases {
		if got := nodePattern(in); got != want {
			t.Errorf("nodePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRejectsUnsupportedFormatForPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("X"))
	}))
	defer srv.Close()
	app, _ := newTestAPI(t, srv.URL)

	// ulaw is accepted by /v1/voices, but the player has no decoder for it.
	body := `{"text":"hi","output_format":"ulaw_8000"}`
	req := httptest.NewRequest("POST", "/playvoice", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 500 {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

// Quick sanity check on context plumbing.
func TestContextNotLeaked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = ctx
}

// TestPlayVoiceFreeTierFallback simulates the 403 ElevenLabs returns when
// the saved format is pcm_* but the account is on the free tier. The
// controller should retry once with mp3_44100_128, persist that as the new
// saved format, and play the decoded audio.
func TestPlayVoiceFreeTierFallback(t *testing.T) {
	if len(fixtureSineMP3) == 0 {
		t.Fatal("MP3 fixture is empty")
	}

	var seenFormats []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmtParam := r.URL.Query().Get("output_format")
		seenFormats = append(seenFormats, fmtParam)
		if strings.HasPrefix(fmtParam, "pcm_") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"detail":{"type":"authorization_error","code":"subscription_required","message":"Output format not allowed.","status":"output_format_not_allowed"}}`)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(fixtureSineMP3)
	}))
	defer srv.Close()

	store, err := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	cur := store.Get()
	cur.APIKey = "secret"
	cur.OutputFormat = "pcm_44100" // simulates a leftover from the old default
	if _, err := store.Update(cur); err != nil {
		t.Fatal(err)
	}

	rec := &recordingPlayer{}
	api := &API{
		Store:  store,
		Player: rec,
		Log:    &testLogger{t: t},
		NewElevenLabs: func(apiKey string) *elevenlabs.Client {
			c := elevenlabs.New(apiKey)
			c.BaseURL = srv.URL
			return c
		},
	}
	app := fiber.New()
	app.Post("/playvoice", api.PlayVoice)

	body := `{"text":"hi"}`
	req := httptest.NewRequest("POST", "/playvoice", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5_000)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}

	// First attempt should be the saved pcm_44100, second should be the
	// mp3 fallback.
	if len(seenFormats) != 2 {
		t.Fatalf("expected 2 ElevenLabs calls (pcm fail then mp3), got %v", seenFormats)
	}
	if seenFormats[0] != "pcm_44100" || seenFormats[1] != "mp3_44100_128" {
		t.Errorf("call sequence: got %v, want [pcm_44100 mp3_44100_128]", seenFormats)
	}

	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["played"] != true {
		t.Errorf("played: got %v, want true", got["played"])
	}
	if got["format_fallback"] != true {
		t.Errorf("format_fallback: got %v, want true", got["format_fallback"])
	}
	if got["output_format"] != "mp3_44100_128" {
		t.Errorf("output_format in response: got %v", got["output_format"])
	}

	// And the format must now be persisted so the next call goes straight
	// to mp3 without round-tripping the 403.
	if api.Store.Get().OutputFormat != "mp3_44100_128" {
		t.Errorf("settings not migrated: %q", api.Store.Get().OutputFormat)
	}
}

// TestPlayVoiceMP3DecodeIntegration runs the full /playvoice flow end-to-end
// with output_format=mp3_44100_128 (the new default). The httptest server
// returns a real MP3 fixture (0.5 s sine wave at 44.1 kHz mono); the
// controller must:
//
//  1. Forward the request to ElevenLabs with the right URL params.
//  2. Decode the MP3 bytes into PCM via go-mp3.
//  3. Hand the PCM to the player as pcm_s16le_44100_2 (go-mp3 always emits
//     interleaved stereo).
//  4. Report sample_rate / channels / pcm_bytes / duration_ms in the JSON
//     response.
//
// This is the path that runs in production today. Without this test the
// only coverage on the MP3 branch is "did anyone get woken up at 3am".
func TestPlayVoiceMP3DecodeIntegration(t *testing.T) {
	if len(fixtureSineMP3) == 0 {
		t.Fatal("MP3 fixture is empty; check //go:embed path")
	}

	var (
		gotPath   string
		gotFormat string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotFormat = r.URL.Query().Get("output_format")
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(fixtureSineMP3)
	}))
	defer srv.Close()

	store, err := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	cur := store.Get()
	cur.APIKey = "secret"
	if _, err := store.Update(cur); err != nil {
		t.Fatal(err)
	}

	rec := &recordingPlayer{}
	api := &API{
		Store:  store,
		Player: rec,
		Log:    &testLogger{t: t},
		NewElevenLabs: func(apiKey string) *elevenlabs.Client {
			c := elevenlabs.New(apiKey)
			c.BaseURL = srv.URL
			return c
		},
	}
	app := fiber.New()
	app.Post("/playvoice", api.PlayVoice)

	body := `{"text":"hello","output_format":"mp3_44100_128"}`
	req := httptest.NewRequest("POST", "/playvoice", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5_000)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// 1) URL routing on the ElevenLabs side.
	if !strings.HasPrefix(gotPath, "/v1/text-to-speech/") {
		t.Errorf("ElevenLabs path: got %q, want /v1/text-to-speech/...", gotPath)
	}
	if gotFormat != "mp3_44100_128" {
		t.Errorf("output_format query: got %q, want mp3_44100_128", gotFormat)
	}

	// 2) JSON response carries the decoded format details.
	if got["played"] != true {
		t.Errorf("played: got %v, want true", got["played"])
	}
	if int(got["bytes"].(float64)) != len(fixtureSineMP3) {
		t.Errorf("bytes: got %v, want %d (raw MP3 length)", got["bytes"], len(fixtureSineMP3))
	}
	if got["sample_rate"].(float64) != 44100 {
		t.Errorf("sample_rate: got %v, want 44100", got["sample_rate"])
	}
	if got["channels"].(float64) != 2 {
		t.Errorf("channels: got %v, want 2 (go-mp3 always emits stereo)", got["channels"])
	}
	pcmBytes := int(got["pcm_bytes"].(float64))
	// 0.5 s at 44.1 kHz stereo s16le = 0.5 * 44100 * 2 * 2 = 88200 bytes of
	// signal. MP3 encoders prepend a 576+ sample delay frame and may pad the
	// tail; go-mp3 emits everything, so the decoded buffer is somewhat
	// larger than the source duration. We accept anything from 0.5 s up to
	// 0.7 s of decoded audio.
	const minBytes = 88_000
	const maxBytes = 124_000
	if pcmBytes < minBytes || pcmBytes > maxBytes {
		t.Errorf("pcm_bytes: got %d, want %d..%d", pcmBytes, minBytes, maxBytes)
	}
	dur := int(got["duration_ms"].(float64))
	if dur < 480 || dur > 720 {
		t.Errorf("duration_ms: got %d, want 480..720", dur)
	}

	// 3) The player got the decoded PCM with the right format string.
	call, ok := rec.lastCall()
	if !ok {
		t.Fatal("player never invoked")
	}
	if call.Format != "pcm_s16le_44100_2" {
		t.Errorf("player format: got %q, want pcm_s16le_44100_2", call.Format)
	}
	if len(call.Data) != pcmBytes {
		t.Errorf("player.Data len: got %d, want %d (matches pcm_bytes in response)", len(call.Data), pcmBytes)
	}
	if call.NodeNamePattern != "^AudioDevice[0-9]+Output[0-9]+$" {
		t.Errorf("default node pattern: got %q", call.NodeNamePattern)
	}

	// 4) Sanity check the PCM is not all zeros: the fixture is a sine wave,
	// so the absolute peak must be well above the noise floor. This catches
	// cases where the decoder silently swallowed the input.
	var peak int16
	for i := 0; i+1 < len(call.Data); i += 2 {
		s := int16(call.Data[i]) | int16(call.Data[i+1])<<8
		if s < 0 {
			s = -s
		}
		if s > peak {
			peak = s
		}
	}
	// Fixture is encoded at -1.5 dBFS so peak should be well above 25k. A
	// successful decode of this sine wave never lands below 20000.
	if peak < 20000 {
		t.Errorf("decoded PCM peak amplitude %d looks too low; decoder may have dropped frames", peak)
	}
}
