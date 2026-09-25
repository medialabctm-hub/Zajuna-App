import { describe, expect, it } from 'vitest'
import { connectionStatus, readStoredConnectionTest, writeStoredConnectionTest } from './connectionStatus'
import type { Job } from '../types'

function testJob(overrides: Partial<Job>): Job {
  return { id: 'job-test', type: 'test-zajuna-connection', status: 'queued', progress: 0, ...overrides }
}

describe('connectionStatus', () => {
  it('no marca como verificada una cuenta solo porque el setup está completo', () => {
    const status = connectionStatus({ setupComplete: true })
    expect(status.label).toBe('Configurada')
    expect(status.label).not.toBe('Verificada')
    expect(status.detail).toMatch(/no se han probado/)
  })

  it('pide configurar cuando no hay credenciales', () => {
    expect(connectionStatus({ setupComplete: false }).label).toBe('Pendiente')
    expect(connectionStatus(undefined).tone).toBe('pending')
  })

  it('muestra la prueba en curso mientras el job está activo', () => {
    for (const status of ['queued', 'running', 'retrying'] as const) {
      const result = connectionStatus({ setupComplete: true }, testJob({ status }))
      expect(result.testing).toBe(true)
      expect(result.tone).toBe('running')
    }
  })

  it('solo verifica con un job completado y muestra las fichas encontradas', () => {
    const result = connectionStatus({ setupComplete: true }, testJob({ status: 'completed', finishedAt: '2026-09-24T12:00:00Z', result: { authenticated: true, fichas: 3 } }))
    expect(result.label).toBe('Verificada')
    expect(result.tone).toBe('ok')
    expect(result.detail).toContain('3 fichas')
  })

  it('explica un fallo real del job con el siguiente paso', () => {
    const result = connectionStatus({ setupComplete: true }, testJob({ status: 'failed', errorCode: 'zajuna_login_failed', errorMessage: 'autenticación de Zajuna rechazada: login HTTP 403' }))
    expect(result.label).toBe('Falló')
    expect(result.tone).toBe('error')
    expect(result.detail).toMatch(/usuario o contraseña/)
  })

  it('trata una prueba cancelada como no verificada', () => {
    expect(connectionStatus({ setupComplete: true }, testJob({ status: 'cancelled' })).label).toBe('Sin verificar')
  })
})

describe('prueba de conexión recordada', () => {
  function memoryStorage() {
    const values = new Map<string, string>()
    return {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => { values.set(key, value) },
      removeItem: (key: string) => { values.delete(key) },
    }
  }

  it('guarda, lee y olvida el último job', () => {
    const storage = memoryStorage()
    writeStoredConnectionTest({ jobId: 'job-1', username: '123' }, storage)
    expect(readStoredConnectionTest(storage)).toEqual({ jobId: 'job-1', username: '123' })
    writeStoredConnectionTest(null, storage)
    expect(readStoredConnectionTest(storage)).toBeNull()
  })

  it('ignora valores corruptos', () => {
    const storage = memoryStorage()
    storage.setItem('zajuna.connectionTest', '{no-json')
    expect(readStoredConnectionTest(storage)).toBeNull()
    storage.setItem('zajuna.connectionTest', JSON.stringify({ jobId: 1 }))
    expect(readStoredConnectionTest(storage)).toBeNull()
  })

  it('funciona sin almacenamiento local', () => {
    expect(readStoredConnectionTest(undefined)).toBeNull()
    expect(() => writeStoredConnectionTest({ jobId: 'x', username: 'y' }, undefined)).not.toThrow()
  })
})
