import { describe, expect, it } from 'vitest'
import type { EvidenceGroup } from '../types'
import { groupState } from '../lib/evidenceGroupState'

const group = (ids: string[], confidence = 'suggested') => ({ id: 'g', confidence, evidences: ids.map((id) => ({ id })) }) as unknown as EvidenceGroup

describe('estado de un grupo en la galería', () => {
  it('sigue a la revisión de evidencias', () => {
    const reviews = new Map([['a', 'approved'], ['b', 'approved'], ['c', 'pending'], ['d', 'rejected']])
    expect(groupState(group(['a', 'b']), reviews).label).toBe('Aprobada')
    expect(groupState(group(['a', 'c']), reviews).label).toBe('Por revisar')
    expect(groupState(group(['a', 'd']), reviews).label).toBe('Rechazada')
  })

  it('sin revisión usa la confianza de la captura', () => {
    expect(groupState(group(['x'], 'manual'), new Map()).label).toBe('Agregada por ti')
    expect(groupState(group(['x']), new Map()).label).toBe('Por revisar')
  })
})
