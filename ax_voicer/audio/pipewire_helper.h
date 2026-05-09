/* libpipewire-0.3 wrapper for cgo. Compiled in-package by `import "C"` in
 * pipewire.go.
 */
#ifndef VOICER_PIPEWIRE_HELPER_H
#define VOICER_PIPEWIRE_HELPER_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define VOICER_PW_VERSION "0.1.0"

typedef struct {
    uint32_t id;
    char     name[128];
    char     media_class[64];
    char     driver[64];
    int      channels;
    int      rate;
} voicer_pw_node_t;

/* Enumerates PipeWire nodes. Returns the number of nodes written into
 * nodes_out (capped at max_nodes), or -1 on error.
 *
 * version_out is filled with VOICER_PW_VERSION on success.
 * err_out captures a human-readable error on failure.
 */
int voicer_pw_info(voicer_pw_node_t* nodes_out,
                   int               max_nodes,
                   char*             version_out,
                   int               version_size,
                   char*             err_out,
                   int               err_size);

/* Plays s16-little-endian PCM `samples` (n_frames * channels samples) to the
 * first PipeWire node whose name matches `pattern` (POSIX extended regex).
 * Blocks until playback drains. Returns 0 on success, -1 on error.
 */
int voicer_pw_play(const int16_t* samples,
                   size_t         n_frames,
                   int            rate,
                   int            channels,
                   float          volume,
                   const char*    pattern,
                   char*          err_out,
                   int            err_size);

#ifdef __cplusplus
}
#endif

#endif
