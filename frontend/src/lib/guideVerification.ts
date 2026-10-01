import type { ChecklistGuide, ChecklistGuideKind, Job, JobStatus } from '../types'

export const GUIDE_KIND_LABEL: Record<ChecklistGuideKind, string> = {
  'content-absent': 'Falta contenido en Zajuna',
  'empty-section': 'Subsección vacía',
  'route-missing': 'No encontramos la sección',
  'content-error': 'Hay que corregir el contenido',
}

const ACTIVE: JobStatus[] = ['queued', 'running', 'waiting_user', 'retrying']

/** Estado de la última verificación de un ítem con guía («Ya lo hice, verificar»). */
export type GuideVerification =
  | { kind: 'idle' }
  | { kind: 'running'; job: Job; queued: boolean }
  | { kind: 'fulfilled'; job: Job }
  | { kind: 'pending'; job: Job }
  | { kind: 'settling'; job: Job }
  | { kind: 'cancelled'; job: Job }
  | { kind: 'failed'; job: Job }

/**
 * Tras terminar el trabajo, el estado del ítem llega un instante después (el
 * dashboard se vuelve a pedir). Durante este margen se muestra «Revisando el
 * resultado…» en vez de un «Seguimos sin encontrarlo» que luego cambia.
 */
export const RESULT_SETTLE_MS = 8000

/** La guía que cubre un ítem: la suya o la de su grupo (alsoItems). */
export function findGuideForItem<T extends Pick<ChecklistGuide, 'itemCode' | 'alsoItems'>>(guides: ReadonlyArray<T>, itemCode: string): T | undefined {
  return guides.find((guide) => guide.itemCode === itemCode || guide.alsoItems?.includes(itemCode))
}

/** Códigos de todos los ítems cubiertos por alguna guía. */
export function guidedItemCodes(guides: ReadonlyArray<Pick<ChecklistGuide, 'itemCode' | 'alsoItems'>>): Set<string> {
  return new Set(guides.flatMap((guide) => [guide.itemCode, ...(guide.alsoItems ?? [])]))
}

function jobTime(job: Job) {
  return Date.parse(job.createdAt || job.updatedAt || '') || 0
}

/**
 * El trabajo de captura más reciente que verificó el ítem en la ficha: uno
 * limitado a ese ítem («Ya lo hice, verificar») o una captura completa de la
 * ficha (sin itemCodes), que también lo vuelve a comprobar.
 */
export function latestItemCaptureJob(jobs: ReadonlyArray<Job>, fichaId: string, itemCode: string): Job | undefined {
  let latest: Job | undefined
  for (const job of jobs) {
    if (job.type !== 'capture-checklist' || !job.fichaId || job.fichaId !== fichaId) continue
    if (job.itemCodes?.length && !job.itemCodes.includes(itemCode)) continue
    if (!latest || jobTime(job) > jobTime(latest)) latest = job
  }
  return latest
}

/** Deriva lo que la guía debe mostrar a partir del trabajo y del estado del ítem. */
export function guideVerification(job: Job | undefined, itemStatus?: string, now: number = Date.now()): GuideVerification {
  if (!job) return { kind: 'idle' }
  if (ACTIVE.includes(job.status)) return { kind: 'running', job, queued: job.status === 'queued' }
  if (job.status === 'completed') {
    if (itemStatus === 'SI') return { kind: 'fulfilled', job }
    const finished = Date.parse(job.finishedAt || job.updatedAt || '') || 0
    return finished && now - finished < RESULT_SETTLE_MS ? { kind: 'settling', job } : { kind: 'pending', job }
  }
  if (job.status === 'cancelled') return { kind: 'cancelled', job }
  return { kind: 'failed', job }
}

/** «hace 3 min», «hace 2 h», «hace 1 día»: cuándo terminó la última verificación. */
export function relativeTime(value?: string, now: number = Date.now()): string {
  const time = Date.parse(value || '')
  if (!time) return ''
  const minutes = Math.max(0, Math.round((now - time) / 60000))
  if (minutes < 1) return 'hace un momento'
  if (minutes < 60) return `hace ${minutes} min`
  const hours = Math.round(minutes / 60)
  if (hours < 24) return `hace ${hours} h`
  const days = Math.round(hours / 24)
  return days === 1 ? 'hace 1 día' : `hace ${days} días`
}

/** Etiqueta corta del estado de una guía en la lista. */
export function guideRowStatus(state: GuideVerification, guideKind?: string): { label: string; tone: 'idle' | 'running' | 'done' | 'pending' | 'failed' } {
  switch (state.kind) {
    case 'running':
      return { label: state.queued ? 'En cola…' : 'Verificando…', tone: 'running' }
    case 'settling':
      return { label: 'Revisando…', tone: 'running' }
    case 'cancelled':
      return { label: 'Cancelada', tone: 'idle' }
    case 'fulfilled':
      return { label: '¡Cumplido!', tone: 'done' }
    case 'pending':
      return { label: guideKind === 'content-error' ? 'Sigue con errores' : 'Sigue sin encontrarse', tone: 'pending' }
    case 'failed':
      return { label: 'Error al verificar', tone: 'failed' }
    default:
      return { label: 'Pendiente', tone: 'idle' }
  }
}

/** Cuánto tiempo se muestra un ítem resuelto en «Cumplidos recientemente». */
export const RESOLVED_WINDOW_MS = 24 * 60 * 60 * 1000

/**
 * Registro de las guías vistas (código → última vez vista). Una guía que
 * desaparece y cuyo ítem está en «SI» se resolvió: se muestra como cumplida
 * durante RESOLVED_WINDOW_MS.
 */
export function updateSeenGuides(seen: Record<string, string>, currentCodes: string[], now: string): Record<string, string> {
  const next: Record<string, string> = {}
  const limit = Date.parse(now) - RESOLVED_WINDOW_MS
  for (const [code, when] of Object.entries(seen)) {
    if (Date.parse(when) >= limit) next[code] = when
  }
  for (const code of currentCodes) next[code] = now
  return next
}

export function resolvedGuideCodes(seen: Record<string, string>, currentCodes: string[], statusByCode: Record<string, string | undefined>): string[] {
  const current = new Set(currentCodes)
  return Object.keys(seen)
    .filter((code) => !current.has(code) && statusByCode[code] === 'SI')
    .sort((left, right) => left.localeCompare(right, 'es', { numeric: true }))
}
