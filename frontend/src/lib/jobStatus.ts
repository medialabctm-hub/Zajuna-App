import type { JobStatus } from '../types'

const TERMINAL_JOB_STATUSES: ReadonlySet<string> = new Set<JobStatus>(['completed', 'failed', 'cancelled'])

/** El trabajo ya no cambiará: terminó bien, falló o se canceló. */
export function isTerminalJobStatus(status?: JobStatus | string): boolean {
  return !!status && TERMINAL_JOB_STATUSES.has(status)
}

/**
 * Verdadero solo en la transición de un estado activo conocido a uno final.
 * Un estado desconocido (aún cargando) no cuenta: el trabajo ya terminado que
 * se abre por primera vez no necesita una lectura extra.
 */
export function jobJustFinished(previous?: JobStatus | string, next?: JobStatus | string): boolean {
  return !!previous && !isTerminalJobStatus(previous) && isTerminalJobStatus(next)
}
