import type { Activity } from '../types'

/** Solo las actividades técnicas generan evidencia válida del instructor. */
export function isSelectable(activity: Activity) {
  return activity.selectable ?? activity.technical
}

/**
 * IDs de las actividades marcadas que realmente se usarán como evidencia:
 * el core toma las primeras `limit` en orden de fase y título.
 */
export function activitiesUsedForEvidence(activities: Activity[], selected: Set<string>, limit: number) {
  return new Set(
    activities
      .filter((activity) => selected.has(activity.id))
      .sort((a, b) => (Number(a.phaseSection) || 0) - (Number(b.phaseSection) || 0) || (a.title.toLowerCase() < b.title.toLowerCase() ? -1 : a.title.toLowerCase() > b.title.toLowerCase() ? 1 : 0))
      .slice(0, limit)
      .map((activity) => activity.id),
  )
}
