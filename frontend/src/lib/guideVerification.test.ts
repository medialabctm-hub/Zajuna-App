import { describe, expect, it } from 'vitest'
import type { Job, JobStatus } from '../types'
import { findGuideForItem, guideRowStatus, guideVerification, guidedItemCodes, latestItemCaptureJob, relativeTime, resolvedGuideCodes, RESULT_SETTLE_MS, updateSeenGuides } from './guideVerification'

const job = (id: string, status: JobStatus, createdAt: string, extra: Partial<Job> = {}): Job => ({
  id,
  type: 'capture-checklist',
  status,
  progress: 0,
  createdAt,
  fichaId: 'f1',
  itemCodes: ['7.3.2'],
  ...extra,
})

describe('latestItemCaptureJob', () => {
  it('sin trabajos no hay verificación', () => {
    expect(latestItemCaptureJob([], 'f1', '7.3.2')).toBeUndefined()
    expect(guideVerification(undefined, 'PENDIENTE')).toEqual({ kind: 'idle' })
  })

  it('el más reciente del ítem gana e ignora otras fichas, otros ítems y otros tipos', () => {
    const jobs = [
      job('old', 'completed', '2026-10-01T10:00:00Z'),
      job('new', 'running', '2026-10-01T11:00:00Z'),
      job('other-ficha', 'running', '2026-10-01T12:00:00Z', { fichaId: 'f2' }),
      job('other-item', 'running', '2026-10-01T12:00:00Z', { itemCodes: ['9.1.6'] }),
      job('no-ficha', 'running', '2026-10-01T12:00:00Z', { fichaId: undefined }),
      job('discover', 'running', '2026-10-01T12:00:00Z', { type: 'discover-course-maps' }),
    ]
    expect(latestItemCaptureJob(jobs, 'f1', '7.3.2')?.id).toBe('new')
  })

  it('una captura completa de la ficha también verifica el ítem', () => {
    const jobs = [job('item', 'completed', '2026-10-01T10:00:00Z'), job('full', 'queued', '2026-10-01T11:00:00Z', { itemCodes: undefined })]
    expect(latestItemCaptureJob(jobs, 'f1', '7.3.2')?.id).toBe('full')
  })
})

describe('guideVerification', () => {
  it('en cola y corriendo', () => {
    expect(guideVerification(job('a', 'queued', '2026-10-01T10:00:00Z'))).toMatchObject({ kind: 'running', queued: true })
    expect(guideVerification(job('a', 'running', '2026-10-01T10:00:00Z'))).toMatchObject({ kind: 'running', queued: false })
    expect(guideVerification(job('a', 'retrying', '2026-10-01T10:00:00Z')).kind).toBe('running')
  })

  it('terminado: cumplido o sigue pendiente según el ítem', () => {
    const done = job('a', 'completed', '2026-10-01T10:00:00Z')
    expect(guideVerification(done, 'SI').kind).toBe('fulfilled')
    expect(guideVerification(done, 'PENDIENTE').kind).toBe('pending')
  })

  it('fallido o cancelado', () => {
    expect(guideVerification(job('a', 'failed', '2026-10-01T10:00:00Z')).kind).toBe('failed')
    expect(guideVerification(job('a', 'cancelled', '2026-10-01T10:00:00Z')).kind).toBe('cancelled')
  })
})

describe('relativeTime', () => {
  const now = Date.parse('2026-10-01T12:00:00Z')
  it('expresa cuánto hace', () => {
    expect(relativeTime('2026-10-01T11:59:50Z', now)).toBe('hace un momento')
    expect(relativeTime('2026-10-01T11:45:00Z', now)).toBe('hace 15 min')
    expect(relativeTime('2026-10-01T09:00:00Z', now)).toBe('hace 3 h')
    expect(relativeTime('2026-09-30T12:00:00Z', now)).toBe('hace 1 día')
    expect(relativeTime(undefined, now)).toBe('')
  })
})

describe('cumplidos recientemente', () => {
  it('una guía vista que desaparece con el ítem en SI se muestra como cumplida', () => {
    const seen = updateSeenGuides({ '7.3.2': '2026-09-29T10:00:00Z' }, ['7.3.2', '14.1.1'], '2026-10-01T10:00:00Z')
    expect(Object.keys(seen).sort()).toEqual(['14.1.1', '7.3.2'])
    const later = updateSeenGuides(seen, ['14.1.1'], '2026-10-01T11:00:00Z')
    expect(resolvedGuideCodes(later, ['14.1.1'], { '7.3.2': 'SI', '14.1.1': 'PENDIENTE' })).toEqual(['7.3.2'])
    expect(resolvedGuideCodes(later, ['14.1.1'], { '7.3.2': 'PENDIENTE' })).toEqual([])
    // Pasado un día se olvida.
    expect(Object.keys(updateSeenGuides(later, [], '2026-10-03T12:00:00Z'))).toEqual([])
  })

  it('etiqueta de estado por fila', () => {
    expect(guideRowStatus({ kind: 'idle' }).label).toBe('Pendiente')
    expect(guideRowStatus(guideVerification(job('a', 'running', '2026-10-01T10:00:00Z'))).label).toBe('Verificando…')
    expect(guideRowStatus(guideVerification(job('a', 'completed', '2026-10-01T10:00:00Z'), 'PENDIENTE')).label).toBe('Sigue sin encontrarse')
    expect(guideRowStatus(guideVerification(job('a', 'failed', '2026-10-01T10:00:00Z'))).tone).toBe('failed')
  })
})

describe('resultado recién terminado, cancelación y guías agrupadas', () => {
  const finished = '2026-10-01T15:00:00Z'
  const now = Date.parse(finished)
  it('muestra «Revisando…» mientras llega el estado del ítem', () => {
    const done = job('j1', 'completed', finished, { finishedAt: finished })
    expect(guideVerification(done, 'PENDIENTE', now + 1000).kind).toBe('settling')
    expect(guideVerification(done, 'PENDIENTE', now + RESULT_SETTLE_MS + 1).kind).toBe('pending')
    expect(guideVerification(done, 'SI', now + 1000).kind).toBe('fulfilled')
    expect(guideRowStatus(guideVerification(done, 'PENDIENTE', now + 1000)).label).toBe('Revisando…')
  })
  it('una verificación cancelada no es un error', () => {
    const cancelled = guideVerification(job('j2', 'cancelled', finished))
    expect(cancelled.kind).toBe('cancelled')
    expect(guideRowStatus(cancelled).label).toBe('Cancelada')
  })
  it('encuentra la guía del grupo para un ítem cubierto', () => {
    const guides = [{ itemCode: '1.2.1', alsoItems: ['1.2.2', '1.2.5'] }, { itemCode: '7.3.2' }]
    expect(findGuideForItem(guides, '1.2.5')?.itemCode).toBe('1.2.1')
    expect(findGuideForItem(guides, '7.3.2')?.itemCode).toBe('7.3.2')
    expect(findGuideForItem(guides, '4.1')).toBeUndefined()
    expect([...guidedItemCodes(guides)].sort()).toEqual(['1.2.1', '1.2.2', '1.2.5', '7.3.2'])
  })
})
