<template>
  <v-card class="section-card pa-4">
    <div class="d-flex mb-3 flex-wrap" style="align-items: baseline;">
      <v-icon color="amber" class="mr-2" style="align-self: center;">mdi-cloud-key-outline</v-icon>
      <span class="text-h6 text-amber mr-2">ElevenLabs</span>
      <small
      style="font-size: 10px;color: #c3c2c2;"
        v-if="connection.connected"
      >
        {{ tierLabel }}<span v-if="usageLabel" class="ml-1">{{ usageLabel }}</span>
    </small>
      <v-spacer />
      <v-btn
        v-if="connection.connected"
        size="x-small" variant="text"
        prepend-icon="mdi-refresh"
        :loading="loading"
        style="align-self: center;"
        @click="emit('reload')"
      >
        Reload
      </v-btn>
    </div>

    <HintField
      v-if="!connection.connected"
      label="API key"
      icon="mdi-key-variant"
      help="Found in ElevenLabs Profile, API Keys. Stored on the camera in plain text."
      hint="Type your key, then click Connect to validate it and load voices and models."
    >
      <div class="d-flex align-center">
        <v-text-field
          v-model="apiKeyLocal"
          :type="showKey ? 'text' : 'password'"
          density="compact"
          hide-details
          placeholder="sk_..."
          autocomplete="off"
          spellcheck="false"
          class="flex-grow-1"
        />
        <v-btn
          variant="text"
          size="small"
          icon
          class="ml-1"
          :title="showKey ? 'Hide' : 'Show'"
          @click="showKey = !showKey"
        >
          <v-icon size="18">{{ showKey ? 'mdi-eye-off' : 'mdi-eye' }}</v-icon>
        </v-btn>
      </div>
    </HintField>

    <v-alert
      v-if="!connection.connected"
      type="info"
      variant="outlined"
      density="compact"
      border="start"
      class="mb-2"
    >
      <div class="text-caption">
        <strong>Required key permissions:</strong>
      </div>
      <ul class="text-caption pl-4 ma-0">
        <li>Models, read</li>
        <li>User, read</li>
        <li>Text to speech</li>
        <li>Voices, read</li>
      </ul>
      <div class="text-caption mt-1">
        Set these on the key page in ElevenLabs (Profile, API Keys, Edit).
      </div>
    </v-alert>

    <div v-if="!connection.connected" class="d-flex flex-wrap mb-2">
      <v-btn
        size="small"
        color="primary"
        prepend-icon="mdi-cloud-check"
        :disabled="!canConnect"
        :loading="loading"
        @click="emit('connect', apiKeyLocal)"
      >
        Connect
      </v-btn>
    </div>

    <div v-else>
      <HintField
        label="Voice"
        icon="mdi-account-voice"
        help="Select a voice from your ElevenLabs account. Only voices marked 'free' work on the free tier; the rest require a paid plan."
      >
        <v-select
          :model-value="settings.voice_id"
          :items="voiceOptions"
          density="compact"
          hide-details
          :loading="loading"
          @update:model-value="(v) => emit('change', { voice_id: v })"
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

      <HintField
        label="Model"
        icon="mdi-brain"
        :help="modelHelp"
      >
        <v-select
          :model-value="settings.model_id"
          :items="modelOptions"
          density="compact"
          hide-details
          :loading="loading"
          @update:model-value="(v) => emit('change', { model_id: v })"
        />
      </HintField>

      <v-row dense>
        <v-col cols="6">
          <HintField label="Stability" icon="mdi-tune-variant"
            :hint="`${Number(settings.stability || 0).toFixed(2)} : higher steadier, lower more expressive`">
            <v-slider :model-value="settings.stability" :min="0" :max="1" :step="0.05"
              color="amber" density="compact" hide-details
              @update:model-value="(v) => emit('change', { stability: v })" />
          </HintField>
        </v-col>
        <v-col cols="6">
          <HintField label="Similarity" icon="mdi-account-voice"
            :hint="`${Number(settings.similarity_boost || 0).toFixed(2)} : pulls toward source voice`">
            <v-slider :model-value="settings.similarity_boost" :min="0" :max="1" :step="0.05"
              color="amber" density="compact" hide-details
              @update:model-value="(v) => emit('change', { similarity_boost: v })" />
          </HintField>
        </v-col>
        <v-col cols="6">
          <HintField label="Style" icon="mdi-drama-masks"
            :hint="`${Number(settings.style || 0).toFixed(2)} : exaggeration, increases latency`">
            <v-slider :model-value="settings.style" :min="0" :max="1" :step="0.05"
              color="amber" density="compact" hide-details
              @update:model-value="(v) => emit('change', { style: v })" />
          </HintField>
        </v-col>
        <v-col cols="6">
          <HintField label="Speaker boost" icon="mdi-volume-plus"
            hint="Sharpens voice clarity at the cost of small latency.">
            <v-switch :model-value="settings.use_speaker_boost" color="amber"
              density="compact" hide-details
              @update:model-value="(v) => emit('change', { use_speaker_boost: !!v })" />
          </HintField>
        </v-col>
      </v-row>

      <v-divider class="my-3" />

      <v-btn
        size="small"
        variant="tonal"
        color="error"
        prepend-icon="mdi-cloud-off-outline"
        :loading="loading"
        @click="emit('disconnect')"
      >
        Disconnect &amp; clear key
      </v-btn>
    </div>
  </v-card>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import HintField from '@/components/HintField.vue'

const props = defineProps({
  settings:   { type: Object, required: true },
  connection: { type: Object, required: true },
  loading:    Boolean,
})
const emit = defineEmits(['change', 'connect', 'disconnect', 'reload'])

const showKey = ref(false)

// Local input model bound only to the v-text-field. Synced from settings on
// incoming changes. Doing it this way (instead of :model-value="settings.api_key")
// stops unrelated re-renders from clobbering what the user has typed.
const apiKeyLocal = ref('')
watch(
  () => props.settings.api_key,
  (v) => {
    apiKeyLocal.value = v || ''
  },
  { immediate: true },
)

// Whenever the loaded voice/model list changes, fall back to the first one if
// the saved value is not in the list. Keeps the dropdown showing a real
// selection rather than a stale ID.
watch(
  () => props.connection.voices,
  (voices) => {
    if (!voices || voices.length === 0) return
    const cur = props.settings.voice_id
    if (!cur || !voices.some((v) => v.voice_id === cur)) {
      emit('change', { voice_id: voices[0].voice_id })
    }
  },
)
watch(
  () => props.connection.models,
  (models) => {
    if (!models || models.length === 0) return
    const cur = props.settings.model_id
    if (!cur || !models.some((m) => m.model_id === cur)) {
      emit('change', { model_id: models[0].model_id })
    }
  },
)

const canConnect = computed(() => {
  const k = (apiKeyLocal.value || '').trim()
  return k.length >= 8 && k !== '********'
})

const tierLabel = computed(() => {
  const s = props.connection.subscription
  if (!s) return 'connected'
  return `Tier: ${s.tier || 'free'}`
})
const usageLabel = computed(() => {
  const s = props.connection.subscription
  if (!s || !s.character_limit) return ''
  return `· ${(s.character_count || 0).toLocaleString()} / ${s.character_limit.toLocaleString()} chars`
})

const voiceOptions = computed(() => {
  // Only voices with category "premade" work on the free tier; everything
  // else (cloned, generated, professional, library) needs a paid plan.
  // We sort free voices first so users land on a working choice by default.
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

const modelOptions = computed(() => {
  return (props.connection.models || []).map((m) => {
    const langCount = (m.languages || []).length
    const tail = langCount > 0 ? ` (${langCount} languages)` : ''
    return { title: `${m.name || m.model_id}${tail}`, value: m.model_id }
  })
})

const modelHelp = computed(() => {
  const n = (props.connection.models || []).length
  return n > 0
    ? `Loaded ${n} text-to-speech models from ElevenLabs.`
    : 'No models loaded yet. Click Reload.'
})
</script>
