/**
 * The translation table, and the property that outlives it: an unknown token
 * still reads as words. Pins outlive rosters and rosters grow before this file
 * hears about it, so the fallback is the part a renamed preset actually hits.
 */

import { describe, expect, it } from 'vitest'
import { levelLabel, presetLabel } from './claudecopy'

describe('presetLabel', () => {
  it('names the served presets in friendly words', () => {
    expect(presetLabel('off')).toBe('Off')
    expect(presetLabel('consultant')).toBe('Consultant')
    expect(presetLabel('second-opinion')).toBe('Second opinion')
    expect(presetLabel('collaborator')).toBe('Collaborator')
  })

  it('reads null as the unpinned position', () => {
    expect(presetLabel(null)).toBe('Follow the deck')
    expect(presetLabel(undefined)).toBe('Follow the deck')
  })

  it('de-hyphenates and capitalises a token it has never met', () => {
    expect(presetLabel('devils-advocate')).toBe('Devils advocate')
  })
})

describe('levelLabel', () => {
  it('names the axis levels in plain words', () => {
    expect(levelLabel('on-request')).toBe('answers when asked')
    expect(levelLabel('rethink')).toBe('the whole deck')
    expect(levelLabel('none')).toBe('never edits')
  })

  it('falls back the same way for an unknown level', () => {
    expect(levelLabel('foresees-all')).toBe('Foresees all')
  })
})
