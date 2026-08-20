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
    // Worker threads instead of the default forked processes: same isolation
    // per test file, ~30% less wall time (26s -> 18s here), because a thread
    // does not pay a process fork plus a fresh V8 for each of 16 files.
    // Measured 2026-08-14, three consecutive clean runs; if a test ever
    // misbehaves only under threads, suspect shared native state and drop
    // this line before debugging the test.
    pool: 'threads',
    // Both timeouts are raised off their defaults (5000 and 10000) because
    // the defaults are marginal for this suite, which is a different thing
    // from generous. Measured 2026-08-21 over eight full runs, three of them
    // against four saturating CPU hogs on this eight-core Mac: the median
    // test takes 174ms unloaded, but p95 is 1270ms and the slowest single
    // test 5804ms -- *past* the 5000ms default on its own. Under load p95
    // rises to 1815ms. Nothing here waits on a real timer; only two files use
    // fake timers at all. These are large React trees rendering in jsdom on
    // an old Intel machine, and they are slow the way a lorry is slow.
    //
    // A correct test that crosses a bound under contention is a bug in the
    // bound. The intermittent 1-3 failures seen on main and on branches were
    // NOT reproduced in those eight runs, so this is containment argued from
    // the margin rather than a fix proven against the failure -- if they
    // return, the timeout was not the mechanism and `setup.ts`'s
    // `asyncUtilTimeout` is the next thing to suspect.
    testTimeout: 20_000,
    hookTimeout: 20_000,
    // Raises testing-library's `waitFor` off its own 1000ms default, which is
    // the tighter of the two bounds and the one a multi-await test meets
    // first. See `src/setup.ts`.
    setupFiles: ['./src/setup.ts'],
    // Restores spies created with `vi.spyOn`. It does NOT reach a bare
    // `vi.fn()` inside a `vi.mock` factory -- that is module-level state which
    // survives between tests -- so those are reset explicitly in `beforeEach`.
    restoreMocks: true,
  },
})
