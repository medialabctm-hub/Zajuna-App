import { describe, expect, it } from 'vitest'
import { isTerminalJobStatus, jobJustFinished } from './jobStatus'

describe('isTerminalJobStatus', () => {
  it('reconoce solo los estados finales', () => {
    expect(isTerminalJobStatus('completed')).toBe(true)
    expect(isTerminalJobStatus('failed')).toBe(true)
    expect(isTerminalJobStatus('cancelled')).toBe(true)
    expect(isTerminalJobStatus('running')).toBe(false)
    expect(isTerminalJobStatus('waiting_user')).toBe(false)
    expect(isTerminalJobStatus(undefined)).toBe(false)
  })
})

describe('jobJustFinished', () => {
  it('detecta el paso de activo a final', () => {
    expect(jobJustFinished('running', 'completed')).toBe(true)
    expect(jobJustFinished('retrying', 'failed')).toBe(true)
    expect(jobJustFinished('queued', 'cancelled')).toBe(true)
  })

  it('ignora estados desconocidos y trabajos que ya estaban terminados', () => {
    expect(jobJustFinished(undefined, 'completed')).toBe(false)
    expect(jobJustFinished('completed', 'completed')).toBe(false)
    expect(jobJustFinished('running', 'running')).toBe(false)
    expect(jobJustFinished('running', undefined)).toBe(false)
  })
})
