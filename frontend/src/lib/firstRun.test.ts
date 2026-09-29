import { describe, expect, it } from 'vitest'
import type { Ficha, Job, JobStatus, JobType } from '../types'
import { firstRunPhase } from './firstRun'

const ficha = (id: string, courseId = `c-${id}`): Ficha => ({ id, externalId: id, name: `Ficha ${id}`, courseId, updatedAt: '2025-01-01T00:00:00Z' })

function job(type: JobType, status: JobStatus, extra: Partial<Job> = {}): Job {
  return { id: `${type}-${status}-${extra.updatedAt ?? ''}`, type, status, progress: 0, updatedAt: '2025-01-01T00:00:00Z', ...extra }
}

describe('firstRunPhase', () => {
  it('espera a conocer fichas, trabajos y mapas', () => {
    expect(firstRunPhase({})).toBe('loading')
    expect(firstRunPhase({ fichas: [], jobs: [] })).toBe('loading')
    expect(firstRunPhase({ fichas: [ficha('1')], jobs: [], mapCourseIds: undefined })).toBe('loading')
  })

  it('sin fichas: sincronizando, fallida o sin intento', () => {
    expect(firstRunPhase({ fichas: [], jobs: [job('sync-fichas', 'running')], mapCourseIds: [] })).toBe('fichas')
    expect(firstRunPhase({ fichas: [], jobs: [job('sync-fichas', 'queued')], mapCourseIds: [] })).toBe('fichas')
    expect(firstRunPhase({ fichas: [], jobs: [job('sync-fichas', 'failed')], mapCourseIds: [] })).toBe('fichas-failed')
    expect(firstRunPhase({ fichas: [], jobs: [], mapCourseIds: [] })).toBe('no-fichas')
  })

  it('usa la sincronización más reciente', () => {
    const jobs = [
      job('sync-fichas', 'failed', { id: 'old', updatedAt: '2025-01-01T00:00:00Z' }),
      job('sync-fichas', 'running', { id: 'new', updatedAt: '2025-01-02T00:00:00Z' }),
    ]
    expect(firstRunPhase({ fichas: [], jobs, mapCourseIds: [] })).toBe('fichas')
  })

  it('con fichas y sin mapas ni búsqueda previa, hay que iniciar la búsqueda de rutas', () => {
    expect(firstRunPhase({ fichas: [ficha('1'), ficha('2')], jobs: [job('sync-fichas', 'completed')], mapCourseIds: [] })).toBe('routes-pending')
  })

  it('mientras la búsqueda de todas las fichas corre, sigue la pantalla de carga aunque ya haya mapas', () => {
    const jobs = [job('discover-course-maps', 'running')]
    expect(firstRunPhase({ fichas: [ficha('1'), ficha('2')], jobs, mapCourseIds: ['c-1'] })).toBe('routes')
  })

  it('una búsqueda de rutas de una ficha no abre la pantalla de carga', () => {
    const jobs = [job('discover-course-maps', 'running', { fichaId: '1' })]
    expect(firstRunPhase({ fichas: [ficha('1')], jobs, mapCourseIds: ['c-1'] })).toBe('ready')
    expect(firstRunPhase({ fichas: [ficha('1')], jobs, mapCourseIds: [] })).toBe('routes-pending')
  })

  it('con mapas guardados la aplicación está lista', () => {
    expect(firstRunPhase({ fichas: [ficha('1')], jobs: [], mapCourseIds: ['c-1'] })).toBe('ready')
  })

  it('si la búsqueda falló y no hay mapas, ofrece reintentar', () => {
    const jobs = [job('discover-course-maps', 'failed')]
    expect(firstRunPhase({ fichas: [ficha('1')], jobs, mapCourseIds: [] })).toBe('routes-failed')
  })

  it('no relanza sola una búsqueda que terminó o el usuario canceló', () => {
    expect(firstRunPhase({ fichas: [ficha('1')], jobs: [job('discover-course-maps', 'completed')], mapCourseIds: [] })).toBe('ready')
    expect(firstRunPhase({ fichas: [ficha('1')], jobs: [job('discover-course-maps', 'cancelled')], mapCourseIds: [] })).toBe('ready')
  })

  it('fichas sin curso asociado no tienen rutas que buscar', () => {
    expect(firstRunPhase({ fichas: [ficha('1', '')], jobs: [], mapCourseIds: [] })).toBe('ready')
  })
})
