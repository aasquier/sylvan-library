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
        // two new files to git on every single rebuild.
        //
        // Freshness comes from the etag/last-modified that FileResponse
        // already sends — **and from `Cache-Control: no-cache`, which is the
        // half that was missing until 2026-08-13.** An etag is only freshness
        // if the browser asks, and with no `Cache-Control` at all a browser
        // may assign its own heuristic lifetime and skip the question. Safari
        // did, and served a stale `DeckDetail.js` against a redeployed server
        // until the page crashed. See `NO_CACHE` in `api/app.py`; that header
        // is what makes stable filenames safe here rather than merely tidy.
        entryFileNames: 'assets/app.js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: 'assets/app.[ext]',
      },
    },
  },
})
