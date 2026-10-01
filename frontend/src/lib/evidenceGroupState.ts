import type { EvidenceGroup } from '../types'

export interface GroupConfidence {
  key: 'manual' | 'high' | 'review' | 'empty'
  label: string
}

export type ReviewStatusById = ReadonlyMap<string, string>

/** The review screen is the source of truth: a group is approved when all its
 * evidences are, rejected when any is, pending otherwise. Groups without a
 * review yet fall back to the capture confidence. */
export function groupState(group: EvidenceGroup, reviews: ReviewStatusById): GroupConfidence {
  const statuses = (group.evidences ?? []).map((evidence) => reviews.get(evidence.id)).filter((status): status is string => !!status)
  if (statuses.includes('rejected')) return { key: 'review', label: 'Rechazada' }
  if (statuses.length && statuses.every((status) => status === 'approved')) return { key: 'high', label: 'Aprobada' }
  if (statuses.length) return { key: 'review', label: 'Por revisar' }
  return groupConfidence(group.confidence)
}

export function groupConfidence(value?: string): GroupConfidence {
  const raw = String(value || '').toLowerCase()
  if (raw.includes('manual')) return { key: 'manual', label: 'Agregada por ti' }
  if (raw.includes('confirm') || raw.includes('high') || raw.includes('alta')) {
    return { key: 'high', label: 'Confirmada' }
  }
  if (raw.includes('suggest') || raw.includes('review') || raw.includes('revis')) {
    return { key: 'review', label: 'Por revisar' }
  }
  return { key: 'review', label: 'Por revisar' }
}

/** Estado de Revisión de un ítem del checklist: la misma fuente que la galería.
 * Las capturas reemplazadas por una subida aprobada del instructor no cuentan.
 * Devuelve null si el ítem aún no tiene evidencias revisadas. */
export function itemReviewState(
  itemCode: string,
  entries: ReadonlyArray<{ itemCode?: string; status: string; superseded?: boolean }>,
): GroupConfidence | null {
  const statuses = entries.filter((entry) => entry.itemCode === itemCode && !entry.superseded).map((entry) => entry.status)
  if (!statuses.length) return null
  if (statuses.includes('rejected')) return { key: 'review', label: 'Rechazada' }
  if (statuses.every((status) => status === 'approved')) return { key: 'high', label: 'Aprobada' }
  return { key: 'review', label: 'Por revisar' }
}
