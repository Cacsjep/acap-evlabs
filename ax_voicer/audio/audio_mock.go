//go:build mock

package audio

import (
	"context"
	"sync"
)

// mockPlayer is used for host-side unit tests and `make dev-host`. It records
// the parameters of each Play call but does no real audio work.
type mockPlayer struct {
	mu    sync.Mutex
	calls []PlayParams
}

func New() Player { return &mockPlayer{} }

func (m *mockPlayer) Info(_ context.Context) Info {
	return Info{
		Implementation: "mock",
		HelperBuilt:    true,
		HelperVersion:  "mock-1",
		Nodes: []NodeInfo{
			{ID: 81, Name: "AudioDevice0Output0", Channels: 1, Rate: 48000, State: "idle", MediaClass: "Audio/Sink"},
		},
		OutputCount: 1,
	}
}

func (m *mockPlayer) Play(_ context.Context, p PlayParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, p)
	return nil
}
