import type { Ficha, Job, JobStatus, JobType } from '../types'

/**
 * Preparación del primer arranque: tras conectar la cuenta se traen las fichas
 * y, una sola vez, las rutas de todas ellas mientras se muestra una pantalla de
 * carga. Todo lo demás (buscar rutas, capturar, revisar) es por ficha.
 *
 * - `loading`: aún no se conocen fichas, trabajos o mapas.
 * - `fichas`: se están sincronizando las fichas.
 * - `fichas-failed` / `no-fichas`: no hay fichas y no hay una sincronización en marcha.
 * - `routes-pending`: hay fichas pero ningún mapa y nadie ha buscado rutas: hay que iniciar la búsqueda.
 * - `routes`: la búsqueda de rutas de todas las fichas está en marcha.
 * - `routes-failed`: esa búsqueda falló y sigue sin haber mapas.
 * - `ready`: se puede usar la aplicación.
 */
export type FirstRunPhase =
  | 'loading'
  | 'fichas'
  | 'fichas-failed'
  | 'no-fichas'
  | 'routes-pending'
  | 'routes'
  | 'routes-failed'
  | 'ready'

const ACTIVE: JobStatus[] = ['queued', 'running', 'waiting_user', 'retrying']

function stamp(job: Job) {
  return String(job.updatedAt || job.createdAt || '')
}

function latest(jobs: Job[], match: (job: Job) => boolean): Job | undefined {
  return jobs.filter(match).sort((a, b) => stamp(b).localeCompare(stamp(a)))[0]
}

const isSync = (job: Job) => job.type === ('sync-fichas' satisfies JobType)

/**
 * La búsqueda de todas las fichas no lleva `fichaId`; la que se pide desde una
 * ficha sí. Así una búsqueda normal nunca abre la pantalla de carga.
 */
const isBulkDiscovery = (job: Job) => job.type === ('discover-course-maps' satisfies JobType) && !job.fichaId

export function firstRunPhase(input: { fichas?: Ficha[]; jobs?: Job[]; mapCourseIds?: string[] }): FirstRunPhase {
  const { fichas, jobs, mapCourseIds } = input
  if (!fichas || !jobs || !mapCourseIds) return 'loading'

  if (fichas.length === 0) {
    const sync = latest(jobs, isSync)
    if (sync && ACTIVE.includes(sync.status)) return 'fichas'
    if (sync?.status === 'failed') return 'fichas-failed'
    return 'no-fichas'
  }

  const bulk = latest(jobs, isBulkDiscovery)
  if (bulk && ACTIVE.includes(bulk.status)) return 'routes'
  if (mapCourseIds.length > 0) return 'ready'
  if (!fichas.some((ficha) => ficha.courseId)) return 'ready'
  if (bulk?.status === 'failed') return 'routes-failed'
  // Si terminó o el usuario la canceló, no se vuelve a lanzar sola.
  if (bulk?.status === 'completed' || bulk?.status === 'cancelled') return 'ready'
  return 'routes-pending'
}
