import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 3456,
    proxy: {
      '/api': 'http://localhost:9876',
      '/ws': {
        target: 'ws://localhost:9876',
        ws: true,
      },
    },
  },
})
