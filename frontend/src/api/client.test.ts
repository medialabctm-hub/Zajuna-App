import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, backupDownloadUrl, captureCapabilityFromLocation, evidenceDownloadUrl, evidenceThumbnailUrl, reportDownloadUrl } from './client'

describe('sesión local', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('toma el secreto del fragmento, lo borra de la URL y lo envía en cada petición', async () => {
    const stored = new Map<string, string>()
    const replaceState = vi.fn()
    captureCapabilityFromLocation({
      location: { hash: '#zc=abc_DEF-123', pathname: '/', search: '' } as Location,
      history: { state: null, replaceState } as unknown as History,
      localStorage: { setItem: (key: string, value: string) => stored.set(key, value) } as unknown as Storage,
    })
    expect(replaceState).toHaveBeenCalledWith(null, '', '/')
    expect([...stored.values()]).toEqual(['abc_DEF-123'])

    const fetchMock = vi.fn(async (_path: string, _init?: RequestInit) => new Response('{"setupComplete":true}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await api.getSetupStatus()
    const init = fetchMock.mock.calls[0][1] ?? {}
    expect(new Headers(init.headers).get('X-Zajuna-Capability')).toBe('abc_DEF-123')
  })

  it('ignora fragmentos que no son del lanzador', () => {
    const replaceState = vi.fn()
    captureCapabilityFromLocation({
      location: { hash: '#seccion', pathname: '/resumen', search: '' } as Location,
      history: { state: null, replaceState } as unknown as History,
    })
    expect(replaceState).not.toHaveBeenCalled()
  })
})

describe('URL helpers de descarga', () => {
  it('codifica ids de evidencia para la galería', () => {
    expect(evidenceDownloadUrl('abc 123')).toBe('/api/evidences/abc%20123/download')
    expect(evidenceDownloadUrl('ev/with/slash')).toBe('/api/evidences/ev%2Fwith%2Fslash/download')
    expect(evidenceThumbnailUrl('ev/with/slash')).toBe('/api/evidences/ev%2Fwith%2Fslash/thumbnail')
  })

  it('codifica ids de reporte y nombres de copia', () => {
    expect(reportDownloadUrl('r1')).toBe('/api/reports/r1/download')
    expect(backupDownloadUrl('copia 2026.zip')).toBe('/api/backups/copia%202026.zip/download')
  })
})

describe('ApiError', () => {
  it('conserva estado y ruta para errores de la API', () => {
    const error = new ApiError('No se pudo completar', 503, '/api/evidences/clear')
    expect(error).toBeInstanceOf(Error)
    expect(error.name).toBe('ApiError')
    expect(error.status).toBe(503)
    expect(error.path).toBe('/api/evidences/clear')
    expect(error.message).toBe('No se pudo completar')
  })
})
