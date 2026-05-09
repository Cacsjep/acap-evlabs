// Build constraint here keeps `go test -tags=mock` happy on host machines that
// have no libpipewire-0.3.
//go:build !mock
// +build !mock

/* libpipewire-0.3 wrapper. Linked into the Voicer Go binary via cgo.
 *
 * Heavily based on the audio-playback ACAP example from
 * https://github.com/AxisCommunications/acap-native-sdk-examples
 */
#include "pipewire_helper.h"

#include <regex.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wtype-limits"
#include <pipewire/pipewire.h>
#include <spa/param/audio/format-utils.h>
#include <spa/utils/dict.h>
#include <spa/utils/result.h>
#pragma GCC diagnostic pop

static void set_err(char* err_out, int err_size, const char* fmt, ...) {
    if (!err_out || err_size <= 0) return;
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(err_out, (size_t)err_size, fmt, ap);
    va_end(ap);
}

/* ------------------------- info ------------------------- */

struct info_state {
    struct pw_main_loop* loop;
    voicer_pw_node_t*    nodes;
    int                  count;
    int                  max;
    int                  sync_seq;
};

static void info_registry_global(void*                 data,
                                 uint32_t              id,
                                 uint32_t              permissions,
                                 const char*           type,
                                 uint32_t              version,
                                 const struct spa_dict* props) {
    (void)permissions;
    (void)version;
    if (!spa_streq(type, PW_TYPE_INTERFACE_Node)) return;
    struct info_state* s = data;
    if (s->count >= s->max) return;
    voicer_pw_node_t* n = &s->nodes[s->count++];
    memset(n, 0, sizeof(*n));
    n->id            = id;
    const char* name = spa_dict_lookup(props, PW_KEY_NODE_NAME);
    const char* cls  = spa_dict_lookup(props, PW_KEY_MEDIA_CLASS);
    const char* drv  = spa_dict_lookup(props, "node.driver");
    const char* chs  = spa_dict_lookup(props, "audio.channels");
    const char* rate = spa_dict_lookup(props, "audio.rate");
    if (name) strncpy(n->name, name, sizeof(n->name) - 1);
    if (cls) strncpy(n->media_class, cls, sizeof(n->media_class) - 1);
    if (drv) strncpy(n->driver, drv, sizeof(n->driver) - 1);
    if (chs) n->channels = atoi(chs);
    if (rate) n->rate = atoi(rate);
}

static const struct pw_registry_events info_registry_events = {
    PW_VERSION_REGISTRY_EVENTS,
    .global = info_registry_global,
};

static void info_core_done(void* data, uint32_t id, int seq) {
    struct info_state* s = data;
    if (id == PW_ID_CORE && seq == s->sync_seq) pw_main_loop_quit(s->loop);
}

static const struct pw_core_events info_core_events = {
    PW_VERSION_CORE_EVENTS,
    .done = info_core_done,
};

int voicer_pw_info(voicer_pw_node_t* nodes_out,
                   int               max_nodes,
                   char*             version_out,
                   int               version_size,
                   char*             err_out,
                   int               err_size) {
    struct info_state s     = {0};
    s.nodes                 = nodes_out;
    s.max                   = max_nodes;
    struct pw_context* ctx  = NULL;
    struct pw_core*    core = NULL;
    struct pw_registry* reg = NULL;
    struct spa_hook reg_listener  = {0};
    struct spa_hook core_listener = {0};

    pw_init(NULL, NULL);
    s.loop = pw_main_loop_new(NULL);
    if (!s.loop) {
        set_err(err_out, err_size, "pw_main_loop_new failed");
        return -1;
    }
    ctx = pw_context_new(pw_main_loop_get_loop(s.loop), NULL, 0);
    if (!ctx) {
        set_err(err_out, err_size, "pw_context_new failed");
        goto fail;
    }
    core = pw_context_connect(ctx, NULL, 0);
    if (!core) {
        set_err(err_out, err_size, "pw_context_connect failed (is the app in the pipewire group?)");
        goto fail;
    }
    pw_core_add_listener(core, &core_listener, &info_core_events, &s);
    reg = pw_core_get_registry(core, PW_VERSION_REGISTRY, 0);
    pw_registry_add_listener(reg, &reg_listener, &info_registry_events, &s);
    s.sync_seq = pw_core_sync(core, PW_ID_CORE, 0);

    pw_main_loop_run(s.loop);

    spa_hook_remove(&reg_listener);
    spa_hook_remove(&core_listener);
    pw_proxy_destroy((struct pw_proxy*)reg);
    pw_core_disconnect(core);
    pw_context_destroy(ctx);
    pw_main_loop_destroy(s.loop);
    pw_deinit();

    if (version_out && version_size > 0) {
        snprintf(version_out, (size_t)version_size, "%s", VOICER_PW_VERSION);
    }
    return s.count;

fail:
    if (core) pw_core_disconnect(core);
    if (ctx) pw_context_destroy(ctx);
    if (s.loop) pw_main_loop_destroy(s.loop);
    pw_deinit();
    return -1;
}

/* ------------------------- play ------------------------- */

struct play_state {
    struct pw_main_loop* loop;
    struct pw_stream*    stream;
    const int16_t*       samples;
    size_t               total_frames;
    size_t               cursor;
    int                  channels;
    int                  rate;
    float                volume;
    int                  finished;
    char                 target[64];
    char*                err_out;
    int                  err_size;
};

static void play_on_state_changed(void*               data,
                                  enum pw_stream_state old,
                                  enum pw_stream_state state,
                                  const char*         error) {
    (void)old;
    struct play_state* st = data;
    if (state == PW_STREAM_STATE_ERROR) {
        set_err(st->err_out, st->err_size, "stream error: %s", error ? error : "?");
        st->finished = 1;
        pw_main_loop_quit(st->loop);
    }
}

static void play_on_drained(void* data) {
    struct play_state* st = data;
    st->finished          = 1;
    pw_main_loop_quit(st->loop);
}

static void play_on_process(void* data) {
    struct play_state* st = data;
    struct pw_buffer*  b  = pw_stream_dequeue_buffer(st->stream);
    if (!b) return;
    struct spa_buffer* buf = b->buffer;
    int16_t*           dst = buf->datas[0].data;
    if (!dst) {
        pw_stream_queue_buffer(st->stream, b);
        return;
    }
    uint32_t cap_frames =
        buf->datas[0].maxsize / (sizeof(int16_t) * (uint32_t)st->channels);
    uint32_t remaining = (uint32_t)(st->total_frames - st->cursor);
    uint32_t frames    = remaining < cap_frames ? remaining : cap_frames;

    if (frames > 0) {
        const int16_t* src = st->samples + (st->cursor * (size_t)st->channels);
        if (st->volume == 1.0f) {
            memcpy(dst, src, frames * sizeof(int16_t) * (size_t)st->channels);
        } else {
            uint32_t total_samples = frames * (uint32_t)st->channels;
            for (uint32_t i = 0; i < total_samples; i++) {
                int v = (int)((float)src[i] * st->volume);
                if (v > 32767) v = 32767;
                if (v < -32768) v = -32768;
                dst[i] = (int16_t)v;
            }
        }
        st->cursor += frames;
    }

    buf->datas[0].chunk->offset = 0;
    buf->datas[0].chunk->stride = (int32_t)(sizeof(int16_t) * (size_t)st->channels);
    buf->datas[0].chunk->size   = frames * sizeof(int16_t) * (uint32_t)st->channels;
    pw_stream_queue_buffer(st->stream, b);

    if (st->cursor >= st->total_frames) {
        pw_stream_flush(st->stream, true);
    }
}

static const struct pw_stream_events play_stream_events = {
    PW_VERSION_STREAM_EVENTS,
    .state_changed = play_on_state_changed,
    .process       = play_on_process,
    .drained       = play_on_drained,
};

struct play_registry_args {
    struct play_state* st;
    regex_t*           node_regex;
    int                connected;
};

static void play_registry_global(void*                  data,
                                 uint32_t               id,
                                 uint32_t               permissions,
                                 const char*            type,
                                 uint32_t               version,
                                 const struct spa_dict* props) {
    (void)permissions;
    (void)version;
    (void)id;
    if (!spa_streq(type, PW_TYPE_INTERFACE_Node)) return;
    struct play_registry_args* a = data;
    if (a->connected) return;
    const char* name = spa_dict_lookup(props, PW_KEY_NODE_NAME);
    if (!name) return;
    if (regexec(a->node_regex, name, 0, NULL, 0) != 0) return;

    strncpy(a->st->target, name, sizeof(a->st->target) - 1);

    struct pw_properties* sprops =
        pw_properties_new(PW_KEY_MEDIA_TYPE, "Audio",
                          PW_KEY_MEDIA_CATEGORY, "Playback",
                          PW_KEY_TARGET_OBJECT, name,
                          NULL);

    a->st->stream = pw_stream_new_simple(pw_main_loop_get_loop(a->st->loop),
                                         "voicer", sprops,
                                         &play_stream_events, a->st);
    if (!a->st->stream) {
        set_err(a->st->err_out, a->st->err_size, "pw_stream_new_simple failed");
        pw_main_loop_quit(a->st->loop);
        return;
    }

    uint8_t bbuf[1024];
    struct spa_pod_builder b = SPA_POD_BUILDER_INIT(bbuf, sizeof(bbuf));
    struct spa_audio_info_raw info = SPA_AUDIO_INFO_RAW_INIT(
        .format   = SPA_AUDIO_FORMAT_S16_LE,
        .channels = (uint32_t)a->st->channels,
        .rate     = (uint32_t)a->st->rate);
    const struct spa_pod* params[1] = {
        spa_format_audio_raw_build(&b, SPA_PARAM_EnumFormat, &info)};

    int res = pw_stream_connect(a->st->stream,
                                PW_DIRECTION_OUTPUT,
                                PW_ID_ANY,
                                PW_STREAM_FLAG_AUTOCONNECT |
                                    PW_STREAM_FLAG_MAP_BUFFERS |
                                    PW_STREAM_FLAG_RT_PROCESS,
                                params, 1);
    if (res < 0) {
        set_err(a->st->err_out, a->st->err_size,
                "pw_stream_connect failed: %s", spa_strerror(res));
        pw_main_loop_quit(a->st->loop);
        return;
    }
    a->connected = 1;
}

static const struct pw_registry_events play_registry_events = {
    PW_VERSION_REGISTRY_EVENTS,
    .global = play_registry_global,
};

int voicer_pw_play(const int16_t* samples,
                   size_t         n_frames,
                   int            rate,
                   int            channels,
                   float          volume,
                   const char*    pattern,
                   char*          err_out,
                   int            err_size) {
    if (!samples || n_frames == 0) {
        set_err(err_out, err_size, "no samples");
        return -1;
    }
    if (rate <= 0 || channels <= 0 || channels > 8) {
        set_err(err_out, err_size, "bad rate/channels (%d / %d)", rate, channels);
        return -1;
    }
    if (!pattern || !*pattern) pattern = "^AudioDevice[0-9]+Output[0-9]+$";

    regex_t reg;
    if (regcomp(&reg, pattern, REG_EXTENDED | REG_NOSUB) != 0) {
        set_err(err_out, err_size, "bad regex: %s", pattern);
        return -1;
    }

    struct play_state st = {0};
    st.samples           = samples;
    st.total_frames      = n_frames;
    st.channels          = channels;
    st.rate              = rate;
    st.volume            = volume <= 0 ? 1.0f : volume;
    st.err_out           = err_out;
    st.err_size          = err_size;

    struct play_registry_args ra = {.st = &st, .node_regex = &reg};

    pw_init(NULL, NULL);
    st.loop = pw_main_loop_new(NULL);
    if (!st.loop) {
        set_err(err_out, err_size, "pw_main_loop_new failed");
        regfree(&reg);
        return -1;
    }
    struct pw_context* ctx  = pw_context_new(pw_main_loop_get_loop(st.loop), NULL, 0);
    struct pw_core*    core = ctx ? pw_context_connect(ctx, NULL, 0) : NULL;
    if (!core) {
        set_err(err_out, err_size, "pw_context_connect failed");
        if (ctx) pw_context_destroy(ctx);
        pw_main_loop_destroy(st.loop);
        regfree(&reg);
        pw_deinit();
        return -1;
    }

    struct pw_registry* reg_obj  = pw_core_get_registry(core, PW_VERSION_REGISTRY, 0);
    struct spa_hook     listener = {0};
    pw_registry_add_listener(reg_obj, &listener, &play_registry_events, &ra);

    pw_main_loop_run(st.loop);

    if (st.stream) pw_stream_destroy(st.stream);
    spa_hook_remove(&listener);
    pw_proxy_destroy((struct pw_proxy*)reg_obj);
    pw_core_disconnect(core);
    pw_context_destroy(ctx);
    pw_main_loop_destroy(st.loop);
    pw_deinit();
    regfree(&reg);

    if (!ra.connected) {
        set_err(err_out, err_size,
                "no PipeWire node matched %s (check Test panel → Audio Devices)",
                pattern);
        return -1;
    }
    return st.finished ? 0 : -1;
}
