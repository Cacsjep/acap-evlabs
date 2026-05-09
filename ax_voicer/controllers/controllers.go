// Package controllers contains the HTTP handlers for the Voicer ACAP.
package controllers

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Cacsjep/voicer/ax_voicer/audio"
	"github.com/Cacsjep/voicer/ax_voicer/elevenlabs"
	"github.com/Cacsjep/voicer/ax_voicer/settings"
	"github.com/gofiber/fiber/v2"
	"github.com/hajimehoshi/go-mp3"
)

// Logger is the small subset of the goxis syslog interface we depend on.
// Keeps the controllers easy to unit-test.
type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

type API struct {
	Store  *settings.Store
	Player audio.Player
	Log    Logger

	// Optional override for unit tests.
	NewElevenLabs func(apiKey string) *elevenlabs.Client
}

func (a *API) newClient(apiKey string) *elevenlabs.Client {
	if a.NewElevenLabs != nil {
		return a.NewElevenLabs(apiKey)
	}
	return elevenlabs.New(apiKey)
}

func errorJSON(c *fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg})
}

/* ---------- settings ---------- */

func (a *API) GetSettings(c *fiber.Ctx) error {
	return c.JSON(settings.Redact(a.Store.Get()))
}

func (a *API) UpdateSettings(c *fiber.Ctx) error {
	cur := a.Store.Get()
	body := map[string]json.RawMessage{}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return errorJSON(c, 400, "invalid json: "+err.Error())
	}

	// Apply provided fields onto current settings, ignoring redacted secrets so
	// the UI doesn't accidentally clobber the API key when displaying ********.
	merged := cur
	mergeBytes, _ := json.Marshal(merged)
	intoMap := map[string]json.RawMessage{}
	_ = json.Unmarshal(mergeBytes, &intoMap)
	for k, v := range body {
		if k == "api_key" && string(v) == `"********"` {
			continue
		}
		intoMap[k] = v
	}
	out, _ := json.Marshal(intoMap)
	if err := json.Unmarshal(out, &merged); err != nil {
		return errorJSON(c, 400, "invalid settings: "+err.Error())
	}
	saved, err := a.Store.Update(merged)
	if err != nil {
		return errorJSON(c, 400, err.Error())
	}
	return c.JSON(settings.Redact(saved))
}

/* ---------- audio info ---------- */

func (a *API) GetAudioInfo(c *fiber.Ctx) error {
	info := a.Player.Info(c.Context())
	return c.JSON(info)
}

/* ---------- ElevenLabs probes ---------- */

func (a *API) TestAPIKey(c *fiber.Ctx) error {
	cur := a.Store.Get()
	if cur.APIKey == "" {
		return errorJSON(c, 400, "api_key is empty")
	}
	cl := a.newClient(cur.APIKey)
	ctx, cancel := context.WithTimeout(c.Context(), 15*time.Second)
	defer cancel()
	sub, err := cl.GetSubscription(ctx)
	if err != nil {
		return errorJSON(c, 502, err.Error())
	}

	// Auto-pick the right output format for the detected tier. Pro and
	// above can use pcm_* (no MP3 decode round trip on the camera). Free
	// tier needs mp3_*. The runPlay format_fallback is still in place as
	// a safety net for the in-between tiers (starter/creator) that also
	// block PCM.
	if newFmt := preferredFormatForTier(sub.Tier, cur.OutputFormat); newFmt != cur.OutputFormat {
		if a.Log != nil {
			a.Log.Infof("voicer: tier=%s, switching output_format %s -> %s", sub.Tier, cur.OutputFormat, newFmt)
		}
		cur.OutputFormat = newFmt
		if _, err := a.Store.Update(cur); err != nil && a.Log != nil {
			a.Log.Errorf("voicer: failed to persist format for tier %s: %v", sub.Tier, err)
		}
	}

	return c.JSON(fiber.Map{
		"ok":            true,
		"subscription":  sub,
		"settings":      settings.Redact(a.Store.Get()),
	})
}

// preferredFormatForTier returns the output_format we want to use for a given
// ElevenLabs subscription tier. Empty / unknown tiers keep the existing
// format unchanged so we never make things worse on a tier we don't know.
func preferredFormatForTier(tier, current string) string {
	t := strings.ToLower(strings.TrimSpace(tier))
	switch t {
	case "":
		return current
	case "free":
		// Free tier: only mp3_* / ulaw_* allowed.
		if strings.HasPrefix(current, "mp3_") {
			return current
		}
		return "mp3_44100_128"
	default:
		// Anything paid: prefer pcm_44100. If runPlay's format_fallback
		// later discovers this tier still blocks PCM (e.g. starter /
		// creator), it will auto-migrate to mp3.
		if current == "pcm_44100" {
			return current
		}
		return "pcm_44100"
	}
}

func (a *API) ListVoices(c *fiber.Ctx) error {
	cur := a.Store.Get()
	if cur.APIKey == "" {
		return errorJSON(c, 400, "api_key is empty")
	}
	cl := a.newClient(cur.APIKey)
	ctx, cancel := context.WithTimeout(c.Context(), 15*time.Second)
	defer cancel()
	v, err := cl.ListVoices(ctx)
	if err != nil {
		return errorJSON(c, 502, err.Error())
	}
	return c.JSON(fiber.Map{"voices": v})
}

func (a *API) ListModels(c *fiber.Ctx) error {
	cur := a.Store.Get()
	if cur.APIKey == "" {
		return errorJSON(c, 400, "api_key is empty")
	}
	cl := a.newClient(cur.APIKey)
	ctx, cancel := context.WithTimeout(c.Context(), 15*time.Second)
	defer cancel()
	models, err := cl.ListModels(ctx)
	if err != nil {
		return errorJSON(c, 502, err.Error())
	}
	// Only return models that can do TTS, the only thing Voicer uses.
	out := make([]elevenlabs.Model, 0, len(models))
	for _, m := range models {
		if m.CanDoTextToSpeech {
			out = append(out, m)
		}
	}
	return c.JSON(fiber.Map{"models": out})
}

/* ---------- speaker test (sawtooth, no ElevenLabs) ---------- */

type toneRequest struct {
	DurationMs int     `json:"duration_ms"`
	FreqHz     int     `json:"freq_hz"`
	Volume     float64 `json:"volume"`
	Waveform   string  `json:"waveform"` // "saw" (default) or "sine"
}

func (a *API) TestTone(c *fiber.Ctx) error {
	var req toneRequest
	_ = json.Unmarshal(c.Body(), &req) // body is optional
	if req.DurationMs <= 0 {
		req.DurationMs = 800
	}
	if req.DurationMs > 5000 {
		req.DurationMs = 5000
	}
	if req.FreqHz <= 0 {
		req.FreqHz = 440
	}
	if req.Volume <= 0 || req.Volume > 1 {
		req.Volume = 0.5
	}
	const rate = 44100
	pcm := generateTone(req.Waveform, req.DurationMs, req.FreqHz, rate, req.Volume)

	cur := a.Store.Get()
	ctx, cancel := context.WithTimeout(c.Context(), 15*time.Second)
	defer cancel()
	if err := a.Player.Play(ctx, audio.PlayParams{
		Format:          fmt.Sprintf("pcm_s16le_%d_1", rate),
		Data:            pcm,
		Volume:          1.0, // amplitude already applied
		NodeNamePattern: nodePattern(cur.AudioNode),
	}); err != nil {
		if a.Log != nil {
			a.Log.Errorf("voicer: tone test failed: %v", err)
		}
		return errorJSON(c, 500, err.Error())
	}
	return c.JSON(fiber.Map{
		"ok":          true,
		"duration_ms": req.DurationMs,
		"freq_hz":     req.FreqHz,
		"waveform":    waveformName(req.Waveform),
	})
}

func waveformName(s string) string {
	switch strings.ToLower(s) {
	case "sine":
		return "sine"
	default:
		return "saw"
	}
}

func generateTone(waveform string, durationMs, freqHz, rate int, amplitude float64) []byte {
	nFrames := durationMs * rate / 1000
	out := make([]byte, nFrames*2)
	period := float64(rate) / float64(freqHz)
	useSine := strings.ToLower(waveform) == "sine"
	for i := 0; i < nFrames; i++ {
		var v float64
		if useSine {
			v = math.Sin(2*math.Pi*float64(i)/period) * amplitude
		} else {
			phase := float64(i) / period
			phase -= math.Floor(phase)
			v = (phase*2.0 - 1.0) * amplitude
		}
		// Short fade-in/out (5 ms) to avoid clicks.
		fade := rate / 200
		if i < fade {
			v *= float64(i) / float64(fade)
		} else if i > nFrames-fade {
			v *= float64(nFrames-i) / float64(fade)
		}
		s := int16(v * 32767)
		out[i*2] = byte(s & 0xff)
		out[i*2+1] = byte((s >> 8) & 0xff)
	}
	return out
}

/* ---------- play / test ---------- */

type playRequest struct {
	Text         string  `json:"text"`
	VoiceID      string  `json:"voice_id"`
	ModelID      string  `json:"model_id"`
	OutputFormat string  `json:"output_format"`
	Volume       float64 `json:"volume"`
	DryRun       bool    `json:"dry_run"`
}

func (a *API) Test(c *fiber.Ctx) error {
	return a.runPlay(c, true)
}

// PlayVoice is the public 3rd-party endpoint exposed at
// <cam>/local/voicer/playvoice. It mirrors Test(), but is mounted outside the
// /api group so it is not caught by the SPA fallback. Access control is
// handled by the camera's reverse proxy (apiPath access in manifest.json).
func (a *API) PlayVoice(c *fiber.Ctx) error {
	return a.runPlay(c, false)
}

func (a *API) runPlay(c *fiber.Ctx, isTest bool) error {
	var req playRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return errorJSON(c, 400, "invalid json: "+err.Error())
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		return errorJSON(c, 400, "text is required")
	}

	cur := a.Store.Get()
	if len(req.Text) > settings.MaxTextChars {
		return errorJSON(c, 400, fmt.Sprintf("text exceeds %d chars", settings.MaxTextChars))
	}
	if cur.APIKey == "" {
		return errorJSON(c, 400, "api_key is not configured")
	}

	voiceID := firstNonEmpty(req.VoiceID, cur.VoiceID)
	modelID := firstNonEmpty(req.ModelID, cur.ModelID)
	outFmt := firstNonEmpty(req.OutputFormat, cur.OutputFormat)

	volume := req.Volume
	if volume == 0 {
		volume = cur.Volume
	}

	cl := a.newClient(cur.APIKey)
	ctx, cancel := context.WithTimeout(c.Context(), 45*time.Second)
	defer cancel()

	if a.Log != nil {
		a.Log.Infof("voicer: synthesise %d chars voice=%s model=%s fmt=%s", len(req.Text), voiceID, modelID, outFmt)
	}

	t0 := time.Now()
	ttsReq := elevenlabs.TTSRequest{
		Text:    req.Text,
		ModelID: modelID,
		VoiceSettings: &elevenlabs.VoiceSettings{
			Stability:       cur.Stability,
			SimilarityBoost: cur.SimilarityBoost,
			Style:           cur.Style,
			UseSpeakerBoost: cur.UseSpeakerBoost,
		},
	}
	audioBytes, contentType, err := cl.TTS(ctx, voiceID, ttsReq, outFmt, cur.OptimizeStreaming)

	// Auto-migrate when the tier does not allow PCM. Free-tier accounts only
	// get mp3 / ulaw; the old default of pcm_44100 trips a 403 otherwise. We
	// retry once with mp3_44100_128 and persist the format so the next call
	// goes through directly.
	formatFallback := false
	if err != nil && isOutputFormatNotAllowed(err) && outFmt != "mp3_44100_128" {
		if a.Log != nil {
			a.Log.Infof("voicer: format %s not allowed for this tier, falling back to mp3_44100_128", outFmt)
		}
		outFmt = "mp3_44100_128"
		formatFallback = true
		audioBytes, contentType, err = cl.TTS(ctx, voiceID, ttsReq, outFmt, cur.OptimizeStreaming)
		if err == nil {
			cur.OutputFormat = outFmt
			if _, perr := a.Store.Update(cur); perr != nil && a.Log != nil {
				a.Log.Errorf("voicer: persisting format fallback failed: %v", perr)
			}
		}
	}
	if err != nil {
		return errorJSON(c, 502, err.Error())
	}

	pcm, rate, channels, err := toPCM(outFmt, audioBytes)
	if err != nil {
		if a.Log != nil {
			a.Log.Errorf("voicer: decode %s failed: %v", outFmt, err)
		}
		return errorJSON(c, 500, "decode "+outFmt+": "+err.Error())
	}

	playFmt := fmt.Sprintf("pcm_s16le_%d_%d", rate, channels)
	resp := fiber.Map{
		"ok":              true,
		"bytes":           len(audioBytes),
		"pcm_bytes":       len(pcm),
		"content_type":    contentType,
		"sample_rate":     rate,
		"channels":        channels,
		"duration_ms":     pcmDurationMs(len(pcm), rate, channels, 2),
		"elapsed_ms":      time.Since(t0).Milliseconds(),
		"voice_id":        voiceID,
		"model_id":        modelID,
		"output_format":   outFmt,
		"format_fallback": formatFallback,
		"played":          false,
		"is_test":         isTest,
	}

	if req.DryRun {
		return c.JSON(resp)
	}

	if err := a.Player.Play(ctx, audio.PlayParams{
		Format:          playFmt,
		Data:            pcm,
		Volume:          volume,
		NodeNamePattern: nodePattern(cur.AudioNode),
	}); err != nil {
		if a.Log != nil {
			a.Log.Errorf("voicer: playback failed: %v", err)
		}
		return errorJSON(c, 500, err.Error())
	}

	resp["played"] = true
	return c.JSON(resp)
}

// toPCM decodes the audio Bytes returned by ElevenLabs into 16-bit
// little-endian PCM ready for the audio player. Supports the formats Voicer
// actually requests:
//
//	pcm_<rate>     - raw mono s16le, no decoding needed
//	mp3_*          - decoded with go-mp3, output is interleaved s16le stereo
//
// Other formats (ulaw_8000) would need a decoder too but Voicer never asks
// for them in normal operation; the caller gets a clear error if they do.
func toPCM(outFmt string, data []byte) (pcm []byte, rate, channels int, err error) {
	switch {
	case strings.HasPrefix(outFmt, "pcm_"):
		r, err := pcmRate(outFmt)
		if err != nil {
			return nil, 0, 0, err
		}
		return data, r, 1, nil

	case strings.HasPrefix(outFmt, "mp3_"):
		dec, err := mp3.NewDecoder(bytes.NewReader(data))
		if err != nil {
			return nil, 0, 0, fmt.Errorf("mp3 decoder init: %w", err)
		}
		out, err := io.ReadAll(dec)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("mp3 decode: %w", err)
		}
		// go-mp3 always emits interleaved s16le stereo.
		return out, dec.SampleRate(), 2, nil

	default:
		return nil, 0, 0, fmt.Errorf("output format %q is not supported by the player; pick pcm_* or mp3_*", outFmt)
	}
}

func pcmRate(outFmt string) (int, error) {
	rate, err := strconv.Atoi(strings.TrimPrefix(outFmt, "pcm_"))
	if err != nil {
		return 0, fmt.Errorf("cannot parse rate from %q", outFmt)
	}
	return rate, nil
}

func pcmDurationMs(bytes int, rate int, channels int, bytesPerSample int) int {
	if rate <= 0 || channels <= 0 || bytesPerSample <= 0 {
		return 0
	}
	frames := bytes / (channels * bytesPerSample)
	return int(int64(frames) * 1000 / int64(rate))
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// isOutputFormatNotAllowed matches the ElevenLabs 403 returned when the
// requested output_format is gated behind a paid tier. Substring match on the
// stable API status code is more durable than parsing the human message.
func isOutputFormatNotAllowed(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "output_format_not_allowed")
}

// nodePattern builds a POSIX extended regex from a chosen PipeWire node
// name. Empty falls back to the default Axis output regex so the app keeps
// working before the user has clicked Save.
func nodePattern(node string) string {
	node = strings.TrimSpace(node)
	if node == "" {
		return "^AudioDevice[0-9]+Output[0-9]+$"
	}
	// Anchor and escape so special chars in the name (e.g. dots, dashes)
	// match literally.
	var b strings.Builder
	b.WriteByte('^')
	for _, r := range node {
		switch r {
		case '\\', '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '|', '^', '$':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('$')
	return b.String()
}

/* ---------- raw audio download (debug helper) ---------- */

// SynthDownload returns the raw audio bytes from ElevenLabs without playing
// them. Useful for offline inspection in the browser ("download the WAV").
func (a *API) SynthDownload(c *fiber.Ctx) error {
	var req playRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return errorJSON(c, 400, "invalid json: "+err.Error())
	}
	cur := a.Store.Get()
	if cur.APIKey == "" {
		return errorJSON(c, 400, "api_key is not configured")
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		return errorJSON(c, 400, "text is required")
	}
	if len(req.Text) > settings.MaxTextChars {
		return errorJSON(c, 400, fmt.Sprintf("text exceeds %d chars", settings.MaxTextChars))
	}
	voiceID := firstNonEmpty(req.VoiceID, cur.VoiceID)
	modelID := firstNonEmpty(req.ModelID, cur.ModelID)
	outFmt := firstNonEmpty(req.OutputFormat, cur.OutputFormat)
	cl := a.newClient(cur.APIKey)
	ctx, cancel := context.WithTimeout(c.Context(), 45*time.Second)
	defer cancel()
	pcm, _, err := cl.TTS(ctx, voiceID, elevenlabs.TTSRequest{
		Text:    req.Text,
		ModelID: modelID,
	}, outFmt, cur.OptimizeStreaming)
	if err != nil {
		return errorJSON(c, 502, err.Error())
	}
	if strings.HasPrefix(outFmt, "pcm_") {
		// Wrap raw PCM in a WAV header so browsers can play it.
		rate, err := pcmRate(outFmt)
		if err != nil {
			return errorJSON(c, 400, err.Error())
		}
		c.Set(fiber.HeaderContentType, "audio/wav")
		c.Set(fiber.HeaderContentDisposition, `attachment; filename="voicer.wav"`)
		return writeWAV(c.Context().Response.BodyWriter(), pcm, rate, 1, 16)
	}
	c.Set(fiber.HeaderContentType, "audio/mpeg")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="voicer.mp3"`)
	_, err = c.Write(pcm)
	return err
}

func writeWAV(w io.Writer, pcm []byte, rate, channels, bitsPerSample int) error {
	byteRate := rate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8
	dataSize := uint32(len(pcm))
	riffSize := 36 + dataSize

	header := make([]byte, 0, 44)
	header = append(header, []byte("RIFF")...)
	header = appendU32(header, riffSize)
	header = append(header, []byte("WAVEfmt ")...)
	header = appendU32(header, 16) // PCM fmt size
	header = appendU16(header, 1)  // PCM
	header = appendU16(header, uint16(channels))
	header = appendU32(header, uint32(rate))
	header = appendU32(header, uint32(byteRate))
	header = appendU16(header, uint16(blockAlign))
	header = appendU16(header, uint16(bitsPerSample))
	header = append(header, []byte("data")...)
	header = appendU32(header, dataSize)

	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(pcm)
	return err
}

func appendU32(b []byte, v uint32) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	return append(b, buf[:]...)
}

func appendU16(b []byte, v uint16) []byte {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	return append(b, buf[:]...)
}

/* ---------- health ---------- */

func (a *API) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"ok":     true,
		"app":    "voicer",
		"time":   time.Now().UTC().Format(time.RFC3339),
		"player": a.Player.Info(c.Context()).Implementation,
	})
}
