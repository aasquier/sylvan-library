import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// Separate from vite.config.ts on purpose. That file's `build.outDir` writes
// straight into the Python package, and CI fails if the committed bundle drifts
// from source -- so the build config is worth leaving strictly alone.
//
// No `globals: true`. Tests import `describe`/`it`/`expect`/`vi` explicitly,
// which keeps `tsconfig.app.json` free of test-only type packages and stops app
// code from being able to reach a test global by accident.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    // Restores spies created with `vi.spyOn`. It does NOT reach a bare
    // `vi.fn()` inside a `vi.mock` factory -- that is module-level state which
    // survives between tests -- so those are reset explicitly in `beforeEach`.
    restoreMocks: true,
  },
})
