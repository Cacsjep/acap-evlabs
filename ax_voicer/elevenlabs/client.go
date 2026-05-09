// Package elevenlabs is a tiny ElevenLabs REST client.
// Only the calls actually used by Voicer are implemented.
package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.elevenlabs.io"

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

type Voice struct {
	VoiceID  string            `json:"voice_id"`
	Name     string            `json:"name"`
	Category string            `json:"category"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type voicesResponse struct {
	Voices []Voice `json:"voices"`
}

type ModelLanguage struct {
	LanguageID string `json:"language_id"`
	Name       string `json:"name"`
}

type Model struct {
	ModelID                       string          `json:"model_id"`
	Name                          string          `json:"name"`
	Description                   string          `json:"description,omitempty"`
	CanDoTextToSpeech             bool            `json:"can_do_text_to_speech"`
	CanDoVoiceConversion          bool            `json:"can_do_voice_conversion"`
	RequiresAlphaAccess           bool            `json:"requires_alpha_access"`
	MaxCharactersRequestFreeUser  int             `json:"max_characters_request_free_user,omitempty"`
	MaxCharactersRequestSubUser   int             `json:"max_characters_request_subscribed_user,omitempty"`
	Languages                     []ModelLanguage `json:"languages,omitempty"`
}

// Subscription holds quota info, useful as a "is the API key valid" probe.
type Subscription struct {
	Tier                string `json:"tier"`
	CharacterCount      int    `json:"character_count"`
	CharacterLimit      int    `json:"character_limit"`
	NextCharacterCountResetUnix int64 `json:"next_character_count_reset_unix"`
	Status              string `json:"status"`
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader, accept string) (*http.Response, error) {
	url := strings.TrimRight(c.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("xi-api-key", c.APIKey)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ElevenLabs error: %v", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("ElevenLabs error: %s", parseAPIError(resp.StatusCode, b))
	}
	return resp, nil
}

// parseAPIError unwraps the ElevenLabs error envelope into a single readable
// sentence. Their typical shape is one of:
//
//	{"detail": {"status": "missing_permissions", "message": "..."}}
//	{"detail": "Plain text error"}
//	{"detail": [{"msg": "...", "type": "..."}]}
//	{"message": "..."}
//
// On parse failure we fall back to the HTTP status reason.
func parseAPIError(status int, body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return fmt.Sprintf("HTTP %d", status)
	}

	var env struct {
		Detail  json.RawMessage `json:"detail"`
		Message string          `json:"message"`
		Error   string          `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		// Not JSON, just return the truncated raw text.
		return truncate(string(body), 240)
	}

	if env.Message != "" {
		return env.Message
	}
	if env.Error != "" {
		return env.Error
	}
	if len(env.Detail) > 0 {
		// detail can be a string or an object; try string first.
		var s string
		if err := json.Unmarshal(env.Detail, &s); err == nil && s != "" {
			return s
		}
		var d struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(env.Detail, &d); err == nil && (d.Message != "" || d.Status != "") {
			if d.Message != "" && d.Status != "" {
				return fmt.Sprintf("%s (%s)", d.Message, d.Status)
			}
			if d.Message != "" {
				return d.Message
			}
			return d.Status
		}
		// Validation error array, like FastAPI returns.
		var arr []struct {
			Msg string `json:"msg"`
		}
		if err := json.Unmarshal(env.Detail, &arr); err == nil && len(arr) > 0 {
			return arr[0].Msg
		}
	}
	return fmt.Sprintf("HTTP %d", status)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (c *Client) ListVoices(ctx context.Context) ([]Voice, error) {
	resp, err := c.request(ctx, "GET", "/v1/voices", nil, "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v voicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return v.Voices, nil
}

func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	resp, err := c.request(ctx, "GET", "/v1/models", nil, "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var models []Model
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, err
	}
	return models, nil
}

func (c *Client) GetSubscription(ctx context.Context) (*Subscription, error) {
	resp, err := c.request(ctx, "GET", "/v1/user/subscription", nil, "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var s Subscription
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

type VoiceSettings struct {
	Stability        float64 `json:"stability"`
	SimilarityBoost  float64 `json:"similarity_boost"`
	Style            float64 `json:"style"`
	UseSpeakerBoost  bool    `json:"use_speaker_boost"`
}

type TTSRequest struct {
	Text          string         `json:"text"`
	ModelID       string         `json:"model_id"`
	VoiceSettings *VoiceSettings `json:"voice_settings,omitempty"`
}

// TTS performs a text-to-speech request and returns the raw audio bytes plus
// the response Content-Type. Output format is selected via the query string
// per ElevenLabs API; the caller picks pcm_*, mp3_* or ulaw_*.
func (c *Client) TTS(ctx context.Context, voiceID string, req TTSRequest, outputFormat string, optimizeStreamingLatency int) ([]byte, string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, "", err
	}
	path := fmt.Sprintf("/v1/text-to-speech/%s?output_format=%s&optimize_streaming_latency=%d", voiceID, outputFormat, optimizeStreamingLatency)
	resp, err := c.request(ctx, "POST", path, bytes.NewReader(body), "audio/*")
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	const max = 25 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > max {
		return nil, "", fmt.Errorf("elevenlabs response exceeded %d bytes", max)
	}
	return data, resp.Header.Get("Content-Type"), nil
}
