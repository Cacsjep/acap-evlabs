<template>
  <div class="mb-3">
    <label class="d-flex align-center mb-1" style="font-size: 12px; color: rgba(255,255,255,0.75);">
      <v-icon v-if="icon" :icon="icon" size="14" color="amber" class="mr-1" />
      <span>{{ label }}</span>
      <v-spacer />
      <v-tooltip v-if="help" location="top" max-width="320">
        <template #activator="{ props }">
          <v-icon v-bind="props" size="14" color="amber">mdi-help-circle-outline</v-icon>
        </template>
        <span>{{ help }}</span>
      </v-tooltip>
    </label>
    <slot />
    <div v-if="hint" class="hint">{{ hint }}</div>
    <div v-if="error" class="hint" style="color:#ff8a80;">{{ error }}</div>
  </div>
</template>

<script setup>
defineProps({
  label: String,
  hint: String,
  help: String,
  icon: String,
  error: String,
})
</script>
