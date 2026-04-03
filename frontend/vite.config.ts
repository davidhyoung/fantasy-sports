import path from 'path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import basicSsl from '@vitejs/plugin-basic-ssl'

export default defineConfig({
  plugins: [
    react(),
    // Generates a self-signed cert so Vite serves over HTTPS.
    // Yahoo requires HTTPS redirect URIs, so this is needed for local dev.
    // Your browser will warn about the cert the first time — click "proceed anyway".
    // Skip SSL when VITE_NO_SSL is set (e.g. for headless preview).
    ...(process.env.VITE_NO_SSL ? [] : [basicSsl()]),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      // All /api/* calls go to the Go backend
      '/api': 'http://localhost:8080',
      // Auth flow redirects (/auth/login, /auth/callback, /auth/logout)
      // also need to reach Go. The browser follows these as full-page
      // redirects, so Vite must proxy them too.
      '/auth': 'http://localhost:8080',
    },
  },
})
