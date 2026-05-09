import Vue from '@vitejs/plugin-vue'
import Vuetify, { transformAssetUrls } from 'vite-plugin-vuetify'
import Fonts from 'unplugin-fonts/vite'
import { defineConfig } from 'vite'
import { fileURLToPath, URL } from 'node:url'
import { resolve } from 'path'

// Same URL convention as msf:
//   manifest.apiPath  -> "voicer"
//   appName           -> "voicer"
// → reverse-proxy base /local/voicer/voicer
export default defineConfig({
  base: '/local/voicer/voicer',
  plugins: [
    Vue({ template: { transformAssetUrls } }),
    Vuetify({ autoImport: true }),
    Fonts({
      fontsource: {
        families: [{ name: 'Roboto', weights: [300, 400, 500, 700], styles: ['normal'], subset: 'latin-ext' }],
      },
    }),
  ],
  build: {
    outDir: resolve(__dirname, '../ax_voicer/html'),
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // Avoid leading underscore in chunk filenames so Go's embed doesn't
        // skip them (same caveat msf hit).
        chunkFileNames: (chunkInfo) => {
          const name = chunkInfo.name.replace(/^_/, 'internal-')
          return `assets/${name}-[hash].js`
        },
      },
    },
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
    extensions: ['.js', '.json', '.mjs', '.vue'],
  },
  server: {
    port: 3001,
    proxy: {
      '/local/voicer/voicer/api': {
        target: 'http://10.0.0.48',
        changeOrigin: true,
      },
      '/local/voicer/voicer/playvoice': {
        target: 'http://10.0.0.48',
        changeOrigin: true,
      },
    },
  },
})
