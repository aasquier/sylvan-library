import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    // Dev server talks to the real API, so the frontend never needs a mock
    // and the two can never drift apart.
    //
    // `/tarot` too, and it is not an API path: the 78 pictures ship as Python
    // package data rather than through `web/public`, which would store 4.6MB
    // twice in git — once there and once in the committed bundle Vite writes
    // (ADR 21). The consequence is that Vite has nothing to serve them from,
    // so the card backs would flip over onto three broken images in dev and
    // only in dev.
    proxy: {
      '/api': 'http://127.0.0.1:8765',
      '/tarot': 'http://127.0.0.1:8765',
    },
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
        // `[name]`, not a fixed `app`: the day the bundle carried more than
        // one asset per extension (the ivy canopy and its sprigs, all webp),
        // the fixed name collided and Rollup numbered the collisions in
        // emission order — which differs between macOS and CI's Linux, so
        // the committed bundle could never match the checked rebuild. Source
        // basenames are stable on every platform and mean something in a
        // diff.
        assetFileNames: 'assets/[name].[ext]',
        // The OCR engine's client half, lazily imported by `lib/reader.ts`.
        // Named, because its package entry is `src/index.js` and the chunk
        // would otherwise be committed as `src.js` — which tells a reader of
        // the diff nothing, and is exactly what the note above is about.
        manualChunks(id: string) {
          if (id.includes('tesseract.js')) return 'reader'
          return undefined
        },
      },
    },
  },
})
