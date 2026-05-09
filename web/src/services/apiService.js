import { toast } from 'vue3-toastify'

const base_url = import.meta.env.DEV
  ? '/local/voicer/voicer/api'
  : `${location.origin}${import.meta.env.BASE_URL}/api`

const playvoice_url = import.meta.env.DEV
  ? '/local/voicer/voicer/playvoice'
  : `${location.origin}${import.meta.env.BASE_URL}/playvoice`

export const PLAYVOICE_PUBLIC_URL = playvoice_url

async function send(method, path, payload, opts = {}) {
  const init = { method, headers: { Accept: 'application/json' } }
  if (payload !== undefined) {
    init.headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(payload)
  }
  const url = path.startsWith('http') ? path : base_url + path
  let resp
  try {
    resp = await fetch(url, init)
  } catch (e) {
    if (!opts.silent) toast.error(`Network error: ${e.message}`)
    throw e
  }
  let body = null
  try { body = await resp.json() } catch {}
  if (!resp.ok) {
    const msg = (body && body.error) || `HTTP ${resp.status}`
    if (!opts.silent) toast.error(msg)
    const err = new Error(msg)
    err.status = resp.status
    err.body = body
    throw err
  }
  return body
}

export const ApiService = {
  getSettings: () => send('GET', '/settings'),
  updateSettings: (s) => send('POST', '/settings', s),
  getAudioInfo: () => send('GET', '/audio/info'),
  testApiKey: () => send('POST', '/test/api_key', {}),
  listVoices: () => send('GET', '/test/voices'),
  listModels: () => send('GET', '/test/models'),
  testPlay: (req) => send('POST', '/test/play', req),
  testTone: (req) => send('POST', '/test/tone', req || {}),
  playVoiceUrl: () => playvoice_url,

  // Triggers a download of the synthesised audio (WAV for pcm_*, MP3 otherwise)
  async downloadSynth(req) {
    const resp = await fetch(`${base_url}/test/synth_download`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    })
    if (!resp.ok) {
      let msg = `HTTP ${resp.status}`
      try {
        const j = await resp.json()
        if (j.error) msg = j.error
      } catch {}
      toast.error(msg)
      throw new Error(msg)
    }
    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = req.output_format && req.output_format.startsWith('pcm_') ? 'voicer.wav' : 'voicer.mp3'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  },
}
