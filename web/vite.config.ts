import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [
    vue(),
    {
      name: 'nfc-log-vite-root',
      configureServer() {
        const root = fileURLToPath(new URL('.', import.meta.url))
        // Hilft bei „alte UI“: Im Terminal muss dieses Verzeichnis zu dem Projekt passen, in dem ihr die .vue-Dateien ändert.
        console.log(`[nfc-time-tracking] Vite-Projektroot (src + public): ${root}`)
      },
    },
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
})
