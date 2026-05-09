<template>
  <v-card class="section-card pa-4">
    <div class="d-flex align-center mb-3">
      <v-icon color="amber" class="mr-2">mdi-speaker</v-icon>
      <span class="text-h6 text-amber">Audio</span>
      <v-spacer />
      <v-btn size="x-small" variant="text" prepend-icon="mdi-refresh" :loading="loading" @click="emit('reload')">
        Refresh
      </v-btn>
    </div>

    <HintField
      label="Output device"
      icon="mdi-speaker-multiple"
      help="The audio output speech is played to. The list comes from the camera; the first one is selected automatically when no value is saved."
    >
      <v-select
        :model-value="settings.audio_node"
        :items="outputOptions"
        density="compact"
        hide-details
        :placeholder="outputOptions.length ? 'Pick a device' : 'No outputs detected yet'"
        @update:model-value="(v) => emit('change', { audio_node: v || '' })"
      />
    </HintField>

    <HintField
      label="Volume"
      icon="mdi-volume-high"
      :hint="`${Number(settings.volume || 1).toFixed(2)}x. Above 1 risks clipping.`"
    >
      <v-slider
        :model-value="settings.volume"
        :min="0"
        :max="2"
        :step="0.05"
        color="amber"
        density="compact"
        hide-details
        @update:model-value="(v) => emit('change', { volume: v })"
      />
    </HintField>

    <v-divider class="my-3" />

    <div class="d-flex align-center mb-2">
      <v-icon color="amber" size="18" class="mr-2">mdi-tune</v-icon>
      <span class="text-subtitle-2">Speaker test</span>
    </div>
    <div class="hint mb-2">Plays a short tone to test the audio output.</div>

    <v-row dense class="mb-1">
      <v-col cols="5">
        <HintField label="Frequency" :hint="`${tone.freq_hz} Hz`">
          <v-slider
            v-model.number="tone.freq_hz"
            :min="100" :max="2000" :step="10"
            color="amber" density="compact" hide-details
          />
        </HintField>
      </v-col>
      <v-col cols="4">
        <HintField label="Duration" :hint="`${tone.duration_ms} ms`">
          <v-slider
            v-model.number="tone.duration_ms"
            :min="100" :max="3000" :step="50"
            color="amber" density="compact" hide-details
          />
        </HintField>
      </v-col>
      <v-col cols="3">
        <HintField label="Wave">
          <v-select
            v-model="tone.waveform"
            :items="[{title:'Saw', value:'saw'}, {title:'Sine', value:'sine'}]"
            density="compact" hide-details
          />
        </HintField>
      </v-col>
    </v-row>
    <v-btn size="small" color="primary" prepend-icon="mdi-play" :loading="toneLoading" @click="playTone">
      Play tone
    </v-btn>
    <div v-if="toneResult" class="hint mt-2">{{ toneResult }}</div>

    <v-divider class="my-3" />

    <div class="d-flex align-center mb-2">
      <v-icon color="amber" size="18" class="mr-2">mdi-cog-outline</v-icon>
      <span class="text-subtitle-2">Detected outputs</span>
    </div>

    <v-alert
      v-if="info.error"
      type="warning"
      variant="outlined"
      density="compact"
      border="start"
      class="mb-2"
    >
      {{ info.error }}
    </v-alert>
    <v-alert
      v-else-if="info.implementation === 'mock'"
      type="info"
      variant="outlined"
      density="compact"
      border="start"
      class="mb-2"
    >
      Host build, audio playback is stubbed. Build on the camera for real output.
    </v-alert>

    <div v-if="outputs.length > 0" class="json-block">
      <div v-for="n in outputs" :key="n.id" class="mb-1">
        <span style="color:#ffc107">{{ niceName(n.name) }}</span>
        <span class="ml-2" style="color:#9c9c9c">{{ n.name }}</span>
        <span class="ml-2" style="color:#777">id={{ n.id }}</span>
        <span class="ml-2" v-if="n.channels">ch={{ n.channels }}</span>
        <span class="ml-2" v-if="n.rate">rate={{ n.rate }}</span>
      </div>
    </div>
    <div v-else class="hint">No outputs reported yet. Click Refresh after the camera audio service starts.</div>
  </v-card>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { toast } from 'vue3-toastify'
import HintField from '@/components/HintField.vue'
import { ApiService } from '@/services/apiService'

const props = defineProps({
  settings: { type: Object, required: true },
  info: { type: Object, default: () => ({}) },
  loading: Boolean,
})
const emit = defineEmits(['change', 'reload'])

const tone = reactive({ freq_hz: 440, duration_ms: 600, waveform: 'saw' })
const toneLoading = ref(false)
const toneResult = ref('')

// Friendly label for "AudioDevice0Output0" style names. Falls back to the raw
// node name when it does not match the Axis convention.
function niceName(name) {
  if (!name) return ''
  const m = String(name).match(/^AudioDevice(\d+)Output(\d+)$/)
  if (m) return `Speaker ${m[2]} (device ${m[1]})`
  return name
}

const outputs = computed(() => {
  const list = props.info.nodes || []
  return list.filter((n) => {
    const cls = (n.media_class || '').toLowerCase()
    return cls.includes('sink') || (n.name || '').toLowerCase().includes('output')
  })
})

const outputOptions = computed(() => {
  return outputs.value.map((n) => {
    const meta = []
    if (n.channels) meta.push(`${n.channels}ch`)
    if (n.rate) meta.push(`${n.rate} Hz`)
    const tail = meta.length ? ` (${meta.join(', ')})` : ''
    return { title: `${niceName(n.name)}${tail}`, value: n.name }
  })
})

async function playTone() {
  toneLoading.value = true
  toneResult.value = ''
  try {
    const r = await ApiService.testTone({ ...tone })
    toneResult.value = `Played ${r.duration_ms} ms ${r.waveform} at ${r.freq_hz} Hz`
    toast.success('Tone played')
  } catch (e) {
    toneResult.value = e.message || 'Tone test failed'
  } finally { toneLoading.value = false }
}
</script>
