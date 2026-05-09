import { createApp } from 'vue'
import App from './App.vue'

import 'vuetify/styles'
import { createVuetify } from 'vuetify'
import * as components from 'vuetify/components'
import * as directives from 'vuetify/directives'
import '@mdi/font/css/materialdesignicons.css'
import 'unfonts.css'

import Vue3Toastify from 'vue3-toastify'
import 'vue3-toastify/dist/index.css'

const vuetify = createVuetify({
  components,
  directives,
  theme: {
    defaultTheme: 'voicerDark',
    themes: {
      voicerDark: {
        dark: true,
        colors: {
          background: '#121212',
          surface: '#1a1a1a',
          primary: '#ffc107',
          secondary: '#90caf9',
          error: '#ff5252',
          success: '#4caf50',
        },
      },
    },
  },
})

const app = createApp(App)
app.use(vuetify)
app.use(Vue3Toastify, {
  theme: 'dark',
  position: 'top-right',
  autoClose: 4000,
  pauseOnHover: true,
  closeOnClick: false,
  newestOnTop: true,
})
app.mount('#app')
