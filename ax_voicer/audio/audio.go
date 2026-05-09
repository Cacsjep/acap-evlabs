// Package audio plays generated speech to the device's PipeWire output.
//
// libpipewire-0.3 is linked directly into the binary via cgo (pipewire.go +
// pipewire_helper.c). On host machines that lack the library, build with
// -tags=mock to swap in a pure-Go stub used for unit tests.
package audio

import "context"

// NodeInfo describes a PipeWire output node visible to the application.
type NodeInfo struct {
	ID         uint32 `json:"id"`
	Name       string `json:"name"`
	Channels   int    `json:"channels"`
	Rate       int    `json:"rate"`
	State      string `json:"state"`
	Driver     string `json:"driver,omitempty"`
	MediaClass string `json:"media_class,omitempty"`
}

// Info is the snapshot returned by Player.Info().
type Info struct {
	Implementation string     `json:"implementation"` // "pipewire", "mock"
	HelperPath     string     `json:"helper_path,omitempty"`
	HelperBuilt    bool       `json:"helper_built"`
	HelperVersion  string     `json:"helper_version,omitempty"`
	Nodes          []NodeInfo `json:"nodes"`
	OutputCount    int        `json:"output_count"`
	Error          string     `json:"error,omitempty"`
}

// PlayParams describes one playback request.
type PlayParams struct {
	// Format the buffer is encoded in. One of:
	//   "pcm_s16le_<rate>_<channels>" (e.g. pcm_s16le_44100_1)
	//   "mp3"
	Format string
	// Audio bytes ready for playback.
	Data []byte
	// Volume multiplier (0..4). 1.0 = unchanged.
	Volume float64
	// NodeNamePattern is a regex matched against PipeWire node names.
	NodeNamePattern string
}

// Player is the audio backend interface.
type Player interface {
	Info(ctx context.Context) Info
	Play(ctx context.Context, p PlayParams) error
}
