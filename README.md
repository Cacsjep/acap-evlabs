# Voicer

ACAP application that synthesises speech via [ElevenLabs](https://elevenlabs.io)
and plays it on the camera through [PipeWire](https://pipewire.org), based on
the official Axis [`audio-playback`](https://github.com/AxisCommunications/acap-native-sdk-examples/tree/main/audio-playback)
example.

```
┌──────────────┐   POST /playvoice    ┌──────────────────────┐
│ 3rd-party    │ ──────────────────▶ │ Voicer (Fiber/Go)    │
│ system       │                      │  ├─ ElevenLabs HTTPS │
└──────────────┘                      │  └─ exec voicer_audio│ ──▶ PipeWire ──▶ 🔊
                                      └──────────────────────┘
```

## Layout

| Path                                | What                                                                  |
| ----------------------------------- | --------------------------------------------------------------------- |
| `ax_voicer/`                        | Go ACAP backend (Fiber, no DB; settings persisted as JSON).           |
| `ax_voicer/voicer_audio/`           | Tiny C helper linking `libpipewire-0.3` for actual playback.          |
| `ax_voicer/Dockerfile.helper`       | One-off Docker build for `voicer_audio` using the Axis SDK image.     |
| `web/`                              | Vue 3 + Vuetify single-page UI (Settings / Test panel).               |
| `Makefile`                          | Top-level entry points (device + host targets).                       |
| `scripts/test_playvoice.py`         | Smoke test for the public `/playvoice` endpoint.                      |

## Build pipeline

The build has two cross-compile steps: **Go** (handled by `goxisbuilder`) and
**C** (libpipewire). They both need the Axis SDK Docker image.

```sh
make audio-helper   # builds ax_voicer/voicer_audio (one-shot, when .c changes)
make web-install    # one-time: npm install
make build          # web → vite build, then goxisbuilder → .eap → install on $IP
```

`make build` does:
1. `cd web && npm run build` → emits `ax_voicer/html/`
2. `goxisbuilder -appdir ./ax_voicer -files "html" ...`
   packages the Go binary (with cgo libpipewire) and the SPA into a `.eap`.

Defaults match the `msf` repo (`IP=10.0.0.48 PWD=1qay2wsx SDK=12.5.0`).
Override on the command line, e.g. `make build IP=192.168.1.50`.

## Host-side dev (no cross-compile env)

The whole Go + Vue stack can be exercised on Windows / macOS / Linux without
docker. Audio playback is stubbed out, but every other code path (settings,
ElevenLabs HTTP, validation, the SPA fallback) is real.

```sh
make host-test                # go test ./... with -tags=host,mock
make host VOICER_KEY=sk_…     # run the API on :8889 (mock pipewire)
make web                      # vite dev server on :3001 (proxies api → camera)
```

The host server uses the same routes the camera does:
- `GET  /local/voicer/voicer/api/settings`
- `POST /local/voicer/voicer/api/test/play`
- `POST /local/voicer/voicer/playvoice`
- …

## Public 3rd-party endpoint

```
POST http://<cam>/local/voicer/playvoice
Content-Type: application/json
X-Voicer-Key: <optional shared secret if configured>

{
  "text": "Intruder detected at gate 1",
  "voice_id": "21m00Tcm4TlvDq8ikWAM",   // optional: overrides saved
  "model_id": "eleven_multilingual_v2", // optional
  "output_format": "pcm_44100",         // must be pcm_* to play
  "volume": 1.0,                         // optional (0..2)
  "dry_run": false                       // synthesise without playing
}
```

`scripts/test_playvoice.py` is a no-deps Python smoke test that hits this
endpoint from outside the camera.

## Settings UI

Mirrors the look and feel of `G:\msf`: Vuetify, dark theme, hint texts on every
input, validation, single Save button. The Test panel surfaces:

- ElevenLabs subscription probe (verifies the API key without spending characters)
- ElevenLabs voices table (one-click "Copy ID" into the override)
- PipeWire node listing from `voicer_audio info` (id / name / media_class /
  channels / rate) so missing or mis-named outputs are visible
- Synthesise & play with optional voice/format overrides and a dry-run button
- Download WAV/MP3 of the generated audio for offline inspection
