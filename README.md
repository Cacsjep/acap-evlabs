# Voicer

ACAP application that synthesises speech via
[ElevenLabs](https://elevenlabs.io) and plays it on the camera through
PipeWire, based on the official Axis
[`audio-playback`](https://github.com/AxisCommunications/acap-native-sdk-examples/tree/main/audio-playback)
example.

```
+--------------+    POST /playvoice    +-----------------------+
|  3rd-party   | --------------------> |  Voicer (Fiber/Go)    |
|  system      |                       |   - ElevenLabs HTTPS  |
+--------------+                       |   - libpipewire (cgo) | --> speaker
                                       +-----------------------+
```

Targets AXIS OS 12 cameras (PipeWire is only available on that family).

## Layout

| Path                                    | What                                                                       |
| --------------------------------------- | -------------------------------------------------------------------------- |
| `ax_voicer/`                            | Go ACAP backend (Fiber, no DB; settings persisted as JSON).                |
| `ax_voicer/audio/pipewire_helper.{c,h}` | cgo wrapper for libpipewire-0.3 (info + play).                             |
| `web/`                                  | Vue 3 + Vuetify single-page UI (Audio / ElevenLabs / Test).                |
| `Makefile`                              | Top-level entry points (device + host targets).                            |
| `scripts/test_playvoice.py`             | No-deps smoke test for the public `/playvoice` endpoint.                   |
| `.github/workflows/`                    | CI: tests on every push, .eap release on `v*` tags for aarch64 + armv7hf.  |

## Build pipeline

```sh
make web-install    # one-time: npm install in web/
make build          # vite build, then goxisbuilder -> .eap -> install on $IP
```

What `make build` does:

1. `npm --prefix web run build` writes the bundled SPA into
   `ax_voicer/html/`.
2. `goxisbuilder -appdir ./ax_voicer -files "html" ...` packages the Go
   binary (cgo libpipewire) and the SPA into a `.eap`, then installs it on
   `$IP` via the Axis device CGI.

Defaults match the `msf` repo (`IP=10.0.0.48 PWD=pass SDK=12.5.0`).
Override on the command line, e.g. `make build IP=192.168.1.50`.

## Host-side dev (no cross-compile env)

The whole Go + Vue stack can be exercised on Windows / macOS / Linux without
docker. Audio playback is stubbed out via the `mock` build tag, but every
other code path (settings, ElevenLabs HTTP, validation, the SPA fallback) is
real.

```sh
make host-test    # go test -tags=host,mock ./...   (also what CI runs)
make host         # run the API on :8889 (mock audio)
make web          # vite dev server on :3001, proxies api -> camera at $IP
```

Build tags used in this project:

| Tag            | Where                                         | Purpose                                      |
| -------------- | --------------------------------------------- | -------------------------------------------- |
| (none)         | camera build                                  | Real `goxis` + cgo libpipewire.              |
| `host`         | `main_host.go`                                | Plain Fiber entry, no Axis SDK imports.      |
| `mock`         | `audio/audio_mock.go`, `audio/pipewire_helper.c` | Skip cgo libpipewire; pure-Go stub player.   |
| `host,mock`    | unit tests on dev machines                    | Combine the two so `go test ./...` works.    |

## Public 3rd-party endpoint

```
POST http://<cam>/local/voicer/playvoice
Content-Type: application/json

{
  "text":          "Intruder detected at gate 1",
  "voice_id":      "21m00Tcm4TlvDq8ikWAM",   // optional, falls back to saved
  "model_id":      "eleven_multilingual_v2", // optional
  "output_format": "mp3_44100_128",          // optional; default depends on tier
  "volume":        1.0,                       // optional, 0..4
  "dry_run":       false                      // synthesise without playing
}
```

Access control is delegated to the camera's reverse proxy
(`access: anonymous` in `manifest.json`); the app itself does not
authenticate.

`scripts/test_playvoice.py` is a stdlib-only Python smoke test:

```sh
python scripts/test_playvoice.py http://10.0.0.48 "Hello"
```

## ElevenLabs tier behaviour

Free-tier accounts cannot use `pcm_*` formats or library voices via the API.
Voicer handles both transparently:

* **Free tier**: format defaults to `mp3_44100_128`. The backend decodes the
  MP3 to PCM via [go-mp3](https://github.com/hajimehoshi/go-mp3) before
  handing it to PipeWire.
* **Pro and above**: format defaults to `pcm_44100`, skipping the decode
  round trip.
* **Mid tiers (starter / creator)** that also block PCM are caught by a
  runtime fallback in `runPlay`: the first call returns
  `output_format_not_allowed`, the controller retries with
  `mp3_44100_128`, and persists that as the new saved format so the next
  call goes through directly.
* **Library voices** (`category != "premade"`) are flagged in the voice
  dropdown with an amber category chip; only `free` (premade) voices work
  on the free API tier. The selector sorts free voices to the top.

The API key needs the following permissions on the ElevenLabs key page
(Profile, API Keys, Edit):

* Models, read
* User, read
* Text to speech
* Voices, read

## Settings UI

Three-column layout, no expansion panels.

* **Audio**: output device dropdown (auto-picked from PipeWire sinks),
  volume slider, speaker test (saw / sine, 100 to 2000 Hz, 100 to 3000 ms).
* **ElevenLabs**: collapsed to just an API-key field plus Connect button
  until authenticated. After connect: voice picker with `free` / `cloned` /
  `professional` category chips, model picker, voice settings (stability,
  similarity, style, speaker boost). Subscription tier and character
  usage are shown next to the panel title.
* **Test & Diagnostics**: synthesise & play (with optional voice override),
  dry run, download. Result is a single outlined alert: green "OK" with
  duration, or red with the ElevenLabs error message.

## CI / CD

[`.github/workflows/test.yaml`](.github/workflows/test.yaml) runs on every
push and pull request:

* `go vet -tags=host,mock ./...`
* `go test -tags=host,mock -race ./...`
* `npm ci && npm run build` for the frontend

[`.github/workflows/build.yaml`](.github/workflows/build.yaml) runs on tag
push (`v*`):

* Builds .eaps for `aarch64` and `armv7hf` in parallel via goxisbuilder
  inside the Axis SDK 12.5.0 docker image.
* Uploads each .eap as a workflow artifact.
* Creates a GitHub Release with both .eaps and a single zip bundle
  attached, plus auto-generated release notes.

To cut a release:

```sh
git tag v0.1.0
git push origin v0.1.0
```
