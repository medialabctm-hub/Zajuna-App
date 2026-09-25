import { describe, expect, it } from 'vitest'
import { activitiesUsedForEvidence, isSelectable } from './activitySelection'
import type { Activity } from '../types'

const activity = (id: string, phaseSection: number, title: string, technical = true): Activity => ({ id, title, phaseSection, technical, selected: false })

describe('activity selection', () => {
  it('never allows transversal activities', () => {
    expect(isSelectable(activity('1', 1, 'Ética', false))).toBe(false)
    expect(isSelectable({ ...activity('2', 1, 'Técnica'), selectable: false })).toBe(false)
    expect(isSelectable(activity('3', 1, 'Técnica'))).toBe(true)
  })

  it('marks the first activities in phase order as the ones used for evidence', () => {
    const all = [activity('a', 9, 'Taller'), activity('b', 3, 'Mapa'), activity('c', 3, 'Crucigrama'), activity('d', 1, 'Foro')]
    const used = activitiesUsedForEvidence(all, new Set(['a', 'b', 'c', 'd']), 2)
    expect([...used]).toEqual(['d', 'c'])
  })
})
