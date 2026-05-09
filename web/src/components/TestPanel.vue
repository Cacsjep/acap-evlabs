<template>
  <v-card class="section-card pa-4">
    <div class="d-flex align-center mb-3">
      <v-icon color="amber" class="mr-2">mdi-test-tube</v-icon>
      <span class="text-h6 text-amber">Test &amp; Diagnostics</span>
    </div>

    <v-alert
      v-if="!connection.connected"
      type="info"
      variant="outlined"
      density="compact"
      border="start"
    >
      Connect ElevenLabs first to test synthesis and view voices.
    </v-alert>

    <template v-else>
      <div class="text-subtitle-2 mb-2">
        <v-icon color="amber" size="18" class="mr-1">mdi-microphone-message</v-icon>
        Synthesise &amp; play
      </div>

      <HintField
        label="Text"
        icon="mdi-format-text"
        :hint="`${(text || '').length} characters`"
      >
        <v-textarea
          v-model="text"
          rows="3"
          density="compact"
          hide-details
          placeholder="Hello from camera 1!"
        />
      </HintField>

      <HintField label="Voice override" help="Optional. Leave blank to use the saved voice. Only voices marked 'free' work on the free tier.">
        <v-select
          v-model="overrideVoice"
          :items="voiceOptions"
          density="compact"
          hide-details
          clearable
          placeholder="(use saved)"
        >
          <template #selection="{ item }">
            <span class="mr-2">{{ item.title }}</span>
            <v-chip
              size="x-small"
              :color="item.raw.isFree ? 'success' : 'warning'"
              variant="tonal"
              style="margin: 0;"
            >
              {{ item.raw.isFree ? 'free' : item.raw.category }}
            </v-chip>
          </template>
          <template #item="{ props: itemProps, item }">
            <v-list-item v-bind="itemProps" :title="item.title">
              <template #append>
                <v-chip
                  size="x-small"
                  :color="item.raw.isFree ? 'success' : 'warning'"
                  variant="tonal"
                  style="margin: 0;"
                >
                  {{ item.raw.isFree ? 'free' : item.raw.category }}
                </v-chip>
              </template>
            </v-list-item>
          </template>
        </v-select>
      </HintField>

      <div class="d-flex flex-wrap mt-2">
        <v-btn size="small" color="primary" prepend-icon="mdi-play" :disabled="!canPlay" :loading="loading.play" @click="play(false)">
          Play
        </v-btn>
        <v-btn size="small" variant="tonal" prepend-icon="mdi-flask" :loading="loading.play" :disabled="!canPlay" @click="play(true)">
          Dry run
        </v-btn>
        <v-btn size="small" variant="tonal" prepend-icon="mdi-download" :loading="loading.download" :disabled="!canPlay" @click="download">
          Download
        </v-btn>
      </div>

      <v-alert
        v-if="resultError"
        type="error"
        variant="outlined"
        density="compact"
        border="start"
        class="mt-3"
      >
        {{ resultError }}
      </v-alert>
      <v-alert
        v-else-if="resultOk"
        type="success"
        variant="outlined"
        density="compact"
        border="start"
        class="mt-3"
      >
        {{ resultOk }}
      </v-alert>
    </template>

    <v-divider class="my-3" />

    <div class="text-subtitle-2 mb-1">
      <v-icon color="amber" size="18" class="mr-1">mdi-api</v-icon>
      Public endpoint
    </div>
    <div class="hint mb-1">3rd party systems can post text directly to:</div>
    <code class="json-block d-block">POST {{ publicUrl }}
{ "text": "Intruder at gate 1" }</code>
    <div class="hint mt-1">Optional fields: voice_id, model_id, output_format, volume, dry_run.</div>
  </v-card>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { toast } from 'vue3-toastify'
import HintField from '@/components/HintField.vue'
import { ApiService, PLAYVOICE_PUBLIC_URL } from '@/services/apiService'

const props = defineProps({
  settings:   { type: Object, required: true },
  audio:      { type: Object, required: true },
  connection: { type: Object, required: true },
})
const emit = defineEmits(['use-voice'])

const loading = reactive({ play: false, download: false })
const text = ref('Hello from camera 1!')
const overrideVoice = ref('')
const resultOk = ref('')
const resultError = ref('')

const voiceOptions = computed(() => {
  const list = (props.connection.voices || []).map((v) => ({
    title: v.name || v.voice_id,
    value: v.voice_id,
    category: v.category || 'unknown',
    isFree: v.category === 'premade',
  }))
  list.sort((a, b) => {
    if (a.isFree !== b.isFree) return a.isFree ? -1 : 1
    return a.title.localeCompare(b.title)
  })
  return list
})

const publicUrl = computed(() => {
  if (PLAYVOICE_PUBLIC_URL.startsWith('http')) return PLAYVOICE_PUBLIC_URL
  return `${location.origin}${PLAYVOICE_PUBLIC_URL}`
})

const canPlay = computed(() => !!(text.value && text.value.trim()))

async function play(dry) {
  loading.play = true
  resultOk.value = ''
  resultError.value = ''
  try {
    const req = { text: text.value, dry_run: dry }
    if (overrideVoice.value) req.voice_id = overrideVoice.value
    const r = await ApiService.testPlay(req)
    if (r.played) {
      resultOk.value = `OK, played ${formatMs(r.duration_ms)}`
      toast.success('Played')
    } else if (dry) {
      resultOk.value = `OK, synthesised (dry run, ${formatMs(r.duration_ms)})`
    } else {
      resultOk.value = 'OK'
    }
  } catch (e) {
    resultError.value = e.message || 'Request failed'
  } finally { loading.play = false }
}

function formatMs(ms) {
  if (!ms || ms < 0) return ''
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)} s`
  return `${ms} ms`
}

async function download() {
  loading.download = true
  try {
    const req = {
      text: text.value,
      voice_id: overrideVoice.value || undefined,
      output_format: props.settings.output_format,
    }
    await ApiService.downloadSynth(req)
  } finally { loading.download = false }
}
</script>
