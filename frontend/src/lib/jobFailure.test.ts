import { describe, expect, it } from 'vitest'
import { explainJobFailure, partialCaptureFailures, unresolvedFailedJobs } from './jobFailure'
import type { Job, JobEvent } from '../types'

function job(overrides: Partial<Job>): Job {
  return { id: 'j1', type: 'capture-checklist', status: 'failed', progress: 100, updatedAt: '2026-09-01T10:00:00Z', ...overrides }
}

describe('explainJobFailure', () => {
  it('turns a rejected login into a concrete next step with the account action', () => {
    const result = explainJobFailure(job({ errorCode: 'zajuna_login_failed', errorMessage: 'no se pudo iniciar sesión: autenticación de Zajuna rechazada: login HTTP 403' }))
    expect(result.title).toMatch(/usuario o contraseña/)
    expect(result.actions.map((action) => action.kind)).toContain('settings')
    expect(result.technical).toContain('HTTP 403')
  })

  it('detects network problems hidden behind a login error code', () => {
    const result = explainJobFailure(job({ errorCode: 'zajuna_login_failed', errorMessage: 'conectar con Zajuna: dial tcp: lookup zajuna.sena.edu.co: no such host' }))
    expect(result.title).toBe('No hubo conexión con Zajuna')
  })

  it('lists every failed item of a partial capture', () => {
    const result = explainJobFailure(job({
      errorCode: 'capture_partial_failure',
      errorMessage: 'captura incompleta: 10 guardadas, 2 con error (ítems 6.1, 7.2.1). Primer error: 6.1: selector no encontrado',
    }))
    expect(result.cause).toContain('Se guardaron 10 evidencias y 2 fallaron')
    expect(result.next).toContain('6.1, 7.2.1')
    expect(result.actions[0].kind).toBe('checklist')
  })

  it('parses the partial capture message that also reports skipped batches', () => {
    const result = explainJobFailure(job({
      errorCode: 'capture_partial_failure',
      errorMessage: 'captura incompleta: 8 guardadas, 3 omitidas, 1 con error (ítems 5.1). Primer error: 5.1: timeout',
    }))
    expect(result.cause).toContain('Se guardaron 8 evidencias y 1 fallaron')
    expect(result.next).toContain('5.1')
  })

  it('points to activities when none were selected', () => {
    expect(explainJobFailure(job({ errorCode: 'activities_not_selected' })).actions[0].to).toBe('/actividades')
  })

  it('always offers a next step for unknown errors', () => {
    const result = explainJobFailure(job({ errorCode: 'worker_panic', errorMessage: 'worker panic: nil pointer' }))
    expect(result.next).toBeTruthy()
    expect(result.actions.length).toBeGreaterThan(0)
  })
})

describe('partialCaptureFailures', () => {
  it('groups every failed slot by item with a plain reason', () => {
    const events: JobEvent[] = [
      { jobId: 'j1', kind: 'evidence_captured', message: 'Evidencia guardada', data: { itemCode: '1.1.1', slotNumber: 1 } },
      { jobId: 'j1', kind: 'evidence_failed', message: '6.1: el selector requerido no apareció en la página destino: #module-1 (candidatos=0)', data: { itemCode: '6.1', slotNumber: 2 } },
      { jobId: 'j1', kind: 'evidence_failed', message: '6.1: el selector requerido no apareció en la página destino: #module-2 (candidatos=0)', data: { itemCode: '6.1', slotNumber: 1 } },
      { jobId: 'j1', kind: 'evidence_failed', message: '9.1.5: sesión de Zajuna expirada o página de login', data: { itemCode: '9.1.5', slotNumber: 1 } },
      { jobId: 'j1', kind: 'evidence_skipped', message: 'Lote de filas vacío', data: { itemCode: '5.1', slotNumber: 3 } },
    ]
    expect(partialCaptureFailures(events)).toEqual([
      { itemCode: '6.1', slots: [1, 2], reasons: ['No encontramos la sección esperada en la página'] },
      { itemCode: '9.1.5', slots: [1], reasons: ['La sesión de Zajuna se cerró durante el proceso'] },
    ])
  })

  it('falls back to the message prefix when the event has no data', () => {
    const events: JobEvent[] = [{ jobId: 'j1', kind: 'evidence_failed', message: '7.2.1: timeout 30000ms exceeded' }]
    expect(partialCaptureFailures(events)).toEqual([
      { itemCode: '7.2.1', slots: [], reasons: ['Zajuna tardó demasiado en responder'] },
    ])
  })
})

describe('unresolvedFailedJobs', () => {
  it('hides failures resolved by a later successful run and dismissed ones', () => {
    const jobs: Job[] = [
      job({ id: 'old-fail', updatedAt: '2026-09-01T10:00:00Z' }),
      job({ id: 'later-ok', status: 'completed', updatedAt: '2026-09-02T10:00:00Z' }),
      job({ id: 'sync-fail', type: 'sync-fichas', updatedAt: '2026-09-03T10:00:00Z' }),
      job({ id: 'dismissed', type: 'export-report', dismissed: true }),
    ]
    expect(unresolvedFailedJobs(jobs).map((entry) => entry.id)).toEqual(['sync-fail'])
  })
})
