import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    // Dev server talks to the real API, so the frontend never needs a mock
    // and the two can never drift apart.
    proxy: { '/api': 'http://127.0.0.1:8765' },
  },
  build: {
    // Built straight into the Python package, so `mtglab ui` serves it and a
    // friend never needs Node to run the app.
    outDir: '../src/mtglab/web_dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // Stable names, not content hashes. This bundle is committed so the
        // app runs without a Node toolchain, and hashed filenames would add
        // two new files to git on every single rebuild. Freshness comes from
        // the etag/last-modified that FileResponse already sends.
        entryFileNames: 'assets/app.js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: 'assets/app.[ext]',
      },
    },
  },
})
