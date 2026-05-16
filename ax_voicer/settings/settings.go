package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// MaxTextChars is the hard cap on text length accepted by /playvoice and the
// test endpoint. Kept internal because the app runs behind the camera's
// authenticated reverse proxy; an attacker on the same network is the wrong
// threat model to plan around here.
const MaxTextChars = 2000

// Settings holds all user-configurable values for the Voicer app.
// They are persisted as JSON next to the binary.
type Settings struct {
	// ElevenLabs
	APIKey            string  `json:"api_key"`
	VoiceID           string  `json:"voice_id"`
	ModelID           string  `json:"model_id"`
	OutputFormat      string  `json:"output_format"` // pcm_16000, pcm_22050, pcm_24000, pcm_44100, mp3_44100_128, ...
	Stability         float64 `json:"stability"`
	SimilarityBoost   float64 `json:"similarity_boost"`
	Style             float64 `json:"style"`
	UseSpeakerBoost   bool    `json:"use_speaker_boost"`
	OptimizeStreaming int     `json:"optimize_streaming_latency"`

	// Audio
	AudioNode string  `json:"audio_node"` // chosen PipeWire node name (e.g. AudioDevice0Output0); empty = first match
	Volume    float64 `json:"volume"`     // 0.0 - 4.0 multiplier applied before playback
}

// Defaults returns the bundled default values.
func Defaults() Settings {
	return Settings{
		VoiceID:         "21m00Tcm4TlvDq8ikWAM", // Rachel, public sample voice
		ModelID:         "eleven_multilingual_v2",
		OutputFormat:    "mp3_44100_128", // works on every tier; pcm_* requires Pro
		Stability:       0.5,
		SimilarityBoost: 0.75,
		Style:           0.0,
		UseSpeakerBoost: true,
		AudioNode:       "",
		Volume:          1.0,
	}
}

// Store is a thread-safe wrapper around the on-disk Settings JSON file.
type Store struct {
	path string
	mu   sync.RWMutex
	cur  Settings
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path, cur: Defaults()}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s.persistLocked()
		}
		return fmt.Errorf("read settings: %w", err)
	}
	var cur Settings
	if err := json.Unmarshal(data, &cur); err != nil {
		return fmt.Errorf("parse settings: %w", err)
	}
	s.merge(&cur)
	s.cur = cur
	return nil
}

// merge fills in any zero values with defaults so newly added fields work
// after upgrades.
func (s *Store) merge(into *Settings) {
	d := Defaults()
	if into.VoiceID == "" {
		into.VoiceID = d.VoiceID
	}
	if into.ModelID == "" {
		into.ModelID = d.ModelID
	}
	if into.OutputFormat == "" {
		into.OutputFormat = d.OutputFormat
	}
	// Volume is intentionally not backfilled: 0 is a legitimate "mute"
	// value the user can save. Fresh installs start from Defaults()
	// before load() is ever called, so a brand-new store still gets
	// Volume=1.0.
}

func (s *Store) persistLocked() error {
	data, err := json.MarshalIndent(s.cur, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Get returns a copy of the current settings.
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Update validates and persists new settings.
func (s *Store) Update(next Settings) (Settings, error) {
	if err := Validate(&next); err != nil {
		return Settings{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = next
	if err := s.persistLocked(); err != nil {
		return Settings{}, err
	}
	return s.cur, nil
}

// Validate normalises and rejects bad values.
func Validate(s *Settings) error {
	if s.Stability < 0 || s.Stability > 1 {
		return fmt.Errorf("stability must be 0..1")
	}
	if s.SimilarityBoost < 0 || s.SimilarityBoost > 1 {
		return fmt.Errorf("similarity_boost must be 0..1")
	}
	if s.Style < 0 || s.Style > 1 {
		return fmt.Errorf("style must be 0..1")
	}
	if s.Volume < 0 || s.Volume > 4 {
		return fmt.Errorf("volume must be 0..4")
	}
	if s.OptimizeStreaming < 0 || s.OptimizeStreaming > 4 {
		return fmt.Errorf("optimize_streaming_latency must be 0..4")
	}
	switch s.OutputFormat {
	case "pcm_16000", "pcm_22050", "pcm_24000", "pcm_44100", "pcm_48000",
		"mp3_44100_128", "mp3_22050_32", "mp3_44100_64", "mp3_44100_96",
		"ulaw_8000":
	default:
		return fmt.Errorf("unsupported output_format: %s", s.OutputFormat)
	}
	return nil
}

// Redact returns a copy with secrets blanked for client display.
func Redact(s Settings) Settings {
	if s.APIKey != "" {
		s.APIKey = "********"
	}
	return s
}
