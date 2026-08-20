/**
 * One bound, raised, and the reason it is raised.
 *
 * `waitFor`, `findBy*` and every async query in testing-library share one
 * timeout, and its default is **1000ms** -- the tightest bound in this suite
 * by a factor of five, and the one a test with several sequential awaits
 * meets first. A screen that fetches a deck, then its shortlist, then its
 * stats spends three of these back to back.
 *
 * Measured 2026-08-21 across eight full runs (see `vitest.config.ts` for the
 * numbers): the median test is 174ms, but 345 of 2845 unloaded results ran
 * past 800ms, and under four saturating CPU hogs p95 moved from 1270ms to
 * 1815ms. A single `waitFor` inside those tests is running with very little
 * room, and room is the entire difference between a slow suite and a flaky
 * one.
 *
 * This does not make anything faster and is not meant to. A test that is
 * genuinely stuck now takes 5s to say so instead of 1s, which is a real cost
 * paid deliberately: a false failure costs a person an hour of looking for a
 * bug that is not there.
 */
import { configure } from '@testing-library/react'

configure({ asyncUtilTimeout: 5000 })
