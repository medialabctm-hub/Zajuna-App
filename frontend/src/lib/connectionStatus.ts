import type { Job, SetupStatus } from '../types'
import { formatDate } from './format'
import { explainJobFailure } from './jobFailure'

export type ConnectionTone = 'ok' | 'pending' | 'running' | 'error' | 'muted'

export interface ConnectionStatus {
  /** Texto corto del chip. */
  label: string
  tone: ConnectionTone
  /** Frase que explica qué significa el estado y qué hacer. */
  detail: string
  /** Hay una prueba en marcha: la acción debe esperar. */
  testing: boolean
}

export interface StoredConnectionTest {
  jobId: string
  username: string
}

const STORAGE_KEY = 'zajuna.connectionTest'
const ACTIVE = new Set(['queued', 'running', 'waiting_user', 'retrying'])

function fichaCount(result: unknown) {
  if (!result || typeof result !== 'object') return undefined
  const value = Number((result as Record<string, unknown>).fichas)
  return Number.isFinite(value) ? value : undefined
}

/**
 * Estado de la cuenta de Zajuna para Configuración. "Verificada" solo sale de
 * un job `test-zajuna-connection` completado; guardar credenciales
 * (`setupComplete`) solo significa que están configuradas.
 */
export function connectionStatus(setup: Pick<SetupStatus, 'setupComplete'> | undefined, job?: Pick<Job, 'status' | 'type' | 'errorCode' | 'errorMessage' | 'message' | 'finishedAt' | 'updatedAt' | 'result'>): ConnectionStatus {
  if (!setup?.setupComplete) {
    return { label: 'Pendiente', tone: 'pending', detail: 'Guarda tu documento y contraseña de Zajuna para empezar.', testing: false }
  }
  if (!job) {
    return {
      label: 'Configurada',
      tone: 'muted',
      detail: 'Las credenciales están guardadas en este equipo, pero todavía no se han probado contra Zajuna.',
      testing: false,
    }
  }
  if (ACTIVE.has(job.status)) {
    return { label: 'Probando…', tone: 'running', detail: 'Estamos iniciando sesión en Zajuna y abriendo Mis cursos.', testing: true }
  }
  if (job.status === 'completed') {
    const fichas = fichaCount(job.result)
    const when = formatDate(job.finishedAt || job.updatedAt)
    return {
      label: 'Verificada',
      tone: 'ok',
      detail: `Zajuna aceptó la sesión (${when})${fichas !== undefined ? ` y mostró ${fichas} ${fichas === 1 ? 'ficha' : 'fichas'}` : ''}.`,
      testing: false,
    }
  }
  if (job.status === 'cancelled') {
    return { label: 'Sin verificar', tone: 'muted', detail: 'La última prueba se canceló antes de terminar.', testing: false }
  }
  const failure = explainJobFailure(job)
  return { label: 'Falló', tone: 'error', detail: `${failure.title}. ${failure.next}`, testing: false }
}

export function readStoredConnectionTest(storage: Pick<Storage, 'getItem'> | undefined = globalThis.localStorage): StoredConnectionTest | null {
  try {
    const parsed = JSON.parse(storage?.getItem(STORAGE_KEY) || 'null')
    if (parsed && typeof parsed.jobId === 'string' && typeof parsed.username === 'string') return parsed
  } catch {
    // Un valor corrupto equivale a no tener prueba previa.
  }
  return null
}

export function writeStoredConnectionTest(value: StoredConnectionTest | null, storage: Pick<Storage, 'setItem' | 'removeItem'> | undefined = globalThis.localStorage) {
  try {
    if (value) storage?.setItem(STORAGE_KEY, JSON.stringify(value))
    else storage?.removeItem(STORAGE_KEY)
  } catch {
    // Sin almacenamiento local el estado solo dura la sesión actual.
  }
}
