//go:build !mock

package audio

/*
#cgo pkg-config: libpipewire-0.3
#cgo LDFLAGS: -lm

#include <stdlib.h>
#include "pipewire_helper.h"
*/
import "C"

import (
	"context"
	"fmt"
	"strings"
	"unsafe"
)

const helperVersion = "0.1.0"

type pwPlayer struct{}

// New returns a Player backed by libpipewire-0.3 directly via cgo.
func New() Player { return &pwPlayer{} }

func (p *pwPlayer) Info(_ context.Context) Info {
	const maxNodes = 64
	var nodes [maxNodes]C.voicer_pw_node_t
	var version [32]C.char
	var errbuf [256]C.char

	n := C.voicer_pw_info(
		&nodes[0], C.int(maxNodes),
		&version[0], C.int(len(version)),
		&errbuf[0], C.int(len(errbuf)),
	)

	info := Info{
		Implementation: "pipewire",
		HelperBuilt:    true,
		HelperVersion:  C.GoString(&version[0]),
	}
	if n < 0 {
		info.Error = C.GoString(&errbuf[0])
		return info
	}
	for i := 0; i < int(n); i++ {
		nd := nodes[i]
		ni := NodeInfo{
			ID:         uint32(nd.id),
			Name:       C.GoString(&nd.name[0]),
			MediaClass: C.GoString(&nd.media_class[0]),
			Driver:     C.GoString(&nd.driver[0]),
			Channels:   int(nd.channels),
			Rate:       int(nd.rate),
		}
		info.Nodes = append(info.Nodes, ni)
		if isOutputNode(ni) {
			info.OutputCount++
		}
	}
	if info.HelperVersion == "" {
		info.HelperVersion = helperVersion
	}
	return info
}

func (p *pwPlayer) Play(_ context.Context, pp PlayParams) error {
	rate, channels, err := parsePCMFormat(pp.Format)
	if err != nil {
		return err
	}
	frameBytes := channels * 2
	if frameBytes <= 0 || len(pp.Data)%frameBytes != 0 {
		return fmt.Errorf("audio data %d bytes not aligned to s16 %d-channel frames",
			len(pp.Data), channels)
	}
	if len(pp.Data) == 0 {
		return fmt.Errorf("no audio data")
	}
	nFrames := len(pp.Data) / frameBytes

	cpattern := C.CString(pp.NodeNamePattern)
	defer C.free(unsafe.Pointer(cpattern))

	var errbuf [256]C.char
	samples := (*C.int16_t)(unsafe.Pointer(&pp.Data[0]))
	rc := C.voicer_pw_play(
		samples,
		C.size_t(nFrames),
		C.int(rate),
		C.int(channels),
		C.float(pp.Volume),
		cpattern,
		&errbuf[0],
		C.int(len(errbuf)),
	)
	if rc != 0 {
		return fmt.Errorf("pipewire: %s", C.GoString(&errbuf[0]))
	}
	return nil
}

func parsePCMFormat(s string) (rate, channels int, err error) {
	if !strings.HasPrefix(s, "pcm_s16le_") {
		return 0, 0, fmt.Errorf("unsupported audio format %q (want pcm_s16le_<rate>_<channels>)", s)
	}
	if _, err := fmt.Sscanf(s, "pcm_s16le_%d_%d", &rate, &channels); err != nil {
		return 0, 0, fmt.Errorf("cannot parse %q: %w", s, err)
	}
	if rate <= 0 || channels <= 0 || channels > 8 {
		return 0, 0, fmt.Errorf("invalid rate/channels in %q", s)
	}
	return rate, channels, nil
}

func isOutputNode(n NodeInfo) bool {
	cls := strings.ToLower(n.MediaClass)
	if strings.Contains(cls, "sink") || strings.Contains(cls, "audio/sink") {
		return true
	}
	return strings.Contains(strings.ToLower(n.Name), "output")
}
