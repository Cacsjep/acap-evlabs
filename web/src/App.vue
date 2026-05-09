<template>
  <v-app>
    <v-app-bar density="compact" color="surface" elevation="2">
      <v-icon class="ml-3 mr-2" color="primary">mdi-microphone-message</v-icon>
      <v-app-bar-title class="text-amber">Voicer</v-app-bar-title>
      <v-spacer />
      <v-btn
        size="small"
        color="primary"
        prepend-icon="mdi-content-save"
        class="mr-3"
        :disabled="!dirty"
        :loading="loading.save"
        @click="saveSettings"
      >
        Save
      </v-btn>
      <v-btn icon size="small" class="mr-2" @click="refreshAll" :loading="loading.audio">
        <v-icon>mdi-refresh</v-icon>
      </v-btn>
    </v-app-bar>

    <v-main>
      <v-container fluid class="pa-4">
        <v-row>
          <v-col cols="12" md="4">
            <AudioPanel
              :settings="local"
              :info="audio"
              :loading="loading.audio"
              @change="onChange"
              @reload="loadAudio"
            />
          </v-col>
          <v-col cols="12" md="4">
            <ElevenLabsPanel
              :settings="local"
              :connection="connection"
              :loading="loading.connect"
              @change="onChange"
              @connect="connect"
              @disconnect="disconnect"
              @reload="reloadVoicesAndModels"
            />
          </v-col>
          <v-col cols="12" md="4">
            <TestPanel
              :settings="local"
              :audio="audio"
              :connection="connection"
              @use-voice="(id) => onChange({ voice_id: id })"
            />
          </v-col>
        </v-row>
      </v-container>
    </v-main>
  </v-app>
</template>

<script setup>
import { computed, onMounted, reactive, watch } from 'vue'
import { toast } from 'vue3-toastify'
import { ApiService } from '@/services/apiService'
import AudioPanel from '@/components/AudioPanel.vue'
import ElevenLabsPanel from '@/components/ElevenLabsPanel.vue'
import TestPanel from '@/components/TestPanel.vue'

const settings = reactive({})
const local = reactive({})
const audio = reactive({ nodes: [] })
const connection = reactive({
  connected: false,
  subscription: null,
  voices: [],
  models: [],
})
const loading = reactive({ audio: false, save: false, connect: false })

const dirty = computed(() => JSON.stringify(local) !== JSON.stringify(settings))

function applyToBoth(s) {
  Object.keys(settings).forEach((k) => delete settings[k])
  Object.keys(local).forEach((k) => delete local[k])
  Object.assign(settings, s)
  Object.assign(local, JSON.parse(JSON.stringify(s)))
}

async function loadSettings() {
  try {
    const s = await ApiService.getSettings()
    applyToBoth(s)
  } catch {}
}

async function loadAudio() {
  loading.audio = true
  try {
    const info = await ApiService.getAudioInfo()
    Object.keys(audio).forEach((k) => delete audio[k])
    Object.assign(audio, info)
  } catch {} finally {
    loading.audio = false
  }
}

async function reloadVoicesAndModels() {
  try {
    const [v, m] = await Promise.all([ApiService.listVoices(), ApiService.listModels()])
    connection.voices = v.voices || []
    connection.models = m.models || []
  } catch {}
}

// connect saves the api_key, validates it against ElevenLabs, and loads
// voices + models. Used both for the explicit Connect click and for the
// auto-connect attempt on page load when a key is already saved.
async function connect(key) {
  loading.connect = true
  try {
    if (key && key !== '********') {
      const saved = await ApiService.updateSettings({ api_key: key })
      applyToBoth(saved)
    }
    const probe = await ApiService.testApiKey()
    connection.subscription = probe.subscription
    // The backend may have switched output_format based on the detected
    // tier (pcm_44100 for paid, mp3_44100_128 for free). Pick that up.
    if (probe.settings) applyToBoth(probe.settings)
    await reloadVoicesAndModels()
    connection.connected = true
    toast.success('Connected to ElevenLabs')
  } catch (e) {
    connection.connected = false
    connection.subscription = null
    connection.voices = []
    connection.models = []
    // toast already shown by apiService
  } finally { loading.connect = false }
}

async function disconnect() {
  loading.connect = true
  try {
    const saved = await ApiService.updateSettings({ api_key: '' })
    applyToBoth(saved)
    connection.connected = false
    connection.subscription = null
    connection.voices = []
    connection.models = []
    toast.info('Disconnected')
  } catch {} finally { loading.connect = false }
}

async function saveSettings() {
  loading.save = true
  try {
    const saved = await ApiService.updateSettings(local)
    applyToBoth(saved)
    toast.success('Settings saved')
  } catch {} finally { loading.save = false }
}

function onChange(patch) {
  Object.assign(local, patch)
}

async function refreshAll() {
  await loadSettings()
  await loadAudio()
  // Auto-connect if a key is already saved on the camera. The server returns
  // the api_key as "********" so we treat that as "already configured".
  if (settings.api_key && settings.api_key.length > 0 && !connection.connected) {
    connect(null)
  }
}

watch(
  () => audio.nodes,
  (nodes) => {
    if (!nodes || !Array.isArray(nodes)) return
    if (local.audio_node) return
    const first = nodes.find((n) => {
      const cls = (n.media_class || '').toLowerCase()
      return cls.includes('sink') || (n.name || '').toLowerCase().includes('output')
    })
    if (first) local.audio_node = first.name
  },
  { deep: true },
)

onMounted(refreshAll)
</script>

<style>
html, body, #app { height: 100%; }
.text-amber { color: #ffc107 !important; }
.hint { font-size: 12px; color: rgba(255,255,255,0.55); margin-top: 2px; line-height: 1.3; }
.section-card {
  background: rgba(30,30,30,0.6) !important;
  border: 1px solid rgba(255,255,255,0.08) !important;
}
.json-block {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  background: rgba(0,0,0,0.4);
  border: 1px solid rgba(255,255,255,0.08);
  border-radius: 4px;
  padding: 8px;
  max-height: 280px;
  overflow: auto;
}
.v-btn { margin-right: 8px; margin-bottom: 4px; }
.v-btn:last-child { margin-right: 0; }
.v-chip { margin-right: 8px; margin-bottom: 4px; }
.v-chip:last-child { margin-right: 0; }
.section-card .v-card-title .v-btn,
.section-card .v-card-title .v-chip { margin-bottom: 0; }

/* Compact vue3-toastify notifications. Defaults are sized for marketing
   sites; we want something terser for an admin UI. */
.Toastify__toast {
  min-height: 36px !important;
  padding: 6px 10px !important;
  font-size: 12px !important;
  border-radius: 4px !important;
}
.Toastify__toast-body {
  font-size: 12px !important;
  padding: 0 !important;
  margin: 0 4px !important;
}
.Toastify__toast-icon {
  width: 14px !important;
  height: 14px !important;
  margin-right: 6px !important;
}
.Toastify__toast-icon svg { width: 14px !important; height: 14px !important; }
.Toastify__close-button > svg { width: 12px !important; height: 12px !important; }
.Toastify__progress-bar { height: 2px !important; }
</style>
