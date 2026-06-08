import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/health': 'http://127.0.0.1:8080',
      '/themes': 'http://127.0.0.1:8080',
      '/search': 'http://127.0.0.1:8080',
      '/semantic-search': 'http://127.0.0.1:8080',
      '/dictionaries': 'http://127.0.0.1:8080',
      '/dictionary-types': 'http://127.0.0.1:8080',
    },
  },
})
