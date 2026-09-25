export type WorkflowStepKey = 'sync' | 'routes' | 'activities' | 'capture' | 'review'
export type WorkflowStepState = 'done' | 'current' | 'running' | 'pending'

export interface WorkflowStep {
  key: WorkflowStepKey
  number: number
  label: string
  hint: string
  to: string
  state: WorkflowStepState
}

export interface WorkflowInput {
  fichasCount: number
  hasActiveFicha: boolean
  syncRunning: boolean
  mapReady: boolean
  discoverRunning: boolean
  selectedActivities: number
  evidenceCount: number
  captureRunning: boolean
  /** Evidencias con problemas (pendientes + rechazadas); undefined si aún no hay revisión. */
  reviewOpen?: number
  reviewTotal?: number
  /** Ítems con toda su evidencia aprobada que aún no están marcados como cumplidos. */
  unmarkedApproved?: number
}

/**
 * Items whose every evidence is approved and that are still pending in the
 * checklist. A "Sí" or "No" set by the person is never counted (nor
 * overwritten by "Marcar como cumplidos").
 */
export function approvedItemsNotMarked(
  evidences: ReadonlyArray<{ itemCode?: string; status: string }>,
  items: ReadonlyArray<{ itemCode: string; status?: string }>,
): string[] {
  const byItem = new Map<string, boolean>()
  for (const entry of evidences) {
    if (!entry.itemCode) continue
    byItem.set(entry.itemCode, (byItem.get(entry.itemCode) ?? true) && entry.status === 'approved')
  }
  const marked = new Set(items.filter((item) => item.status === 'SI' || item.status === 'NO').map((item) => item.itemCode))
  return [...byItem.entries()].filter(([code, ok]) => ok && !marked.has(code)).map(([code]) => code)
}

/**
 * The instructor's flow, in order. A step is "current" when every previous
 * step is done; later steps stay "pending" so the UI can point to exactly
 * one next action.
 */
export function computeWorkflow(input: WorkflowInput): WorkflowStep[] {
  const done: Record<WorkflowStepKey, boolean> = {
    sync: input.fichasCount > 0 && input.hasActiveFicha,
    routes: input.mapReady,
    activities: input.selectedActivities > 0,
    capture: input.evidenceCount > 0,
    // Reviewed means nothing left to fix and the approved items already count
    // in the checklist (otherwise the PDF shows 0 %).
    review: (input.reviewTotal ?? 0) > 0 && input.reviewOpen === 0 && (input.unmarkedApproved ?? 0) === 0,
  }
  const running: Record<WorkflowStepKey, boolean> = {
    sync: input.syncRunning,
    routes: input.discoverRunning,
    activities: false,
    capture: input.captureRunning,
    review: false,
  }
  const definitions: Array<Omit<WorkflowStep, 'state' | 'number'>> = [
    {
      key: 'sync',
      label: 'Sincronizar fichas',
      hint: input.fichasCount > 0 ? 'Elige la ficha con la que vas a trabajar.' : 'Trae tus fichas desde Zajuna.',
      to: '/fichas',
    },
    { key: 'routes', label: 'Buscar rutas', hint: 'Leemos el contenido del curso de la ficha activa.', to: '/fichas' },
    { key: 'activities', label: 'Seleccionar actividades', hint: 'Marca las actividades técnicas que tú calificas.', to: '/actividades' },
    { key: 'capture', label: 'Preparar evidencias', hint: 'Capturamos las evidencias en Zajuna.', to: '/resumen' },
    {
      key: 'review',
      label: 'Revisar evidencias',
      hint: input.reviewOpen
        ? `${input.reviewOpen} evidencias necesitan tu revisión.`
        : input.unmarkedApproved
          ? `Marca los ${input.unmarkedApproved} ítems aprobados como cumplidos.`
          : 'Aprueba las correctas y corrige las que fallaron.',
      to: '/revision',
    },
  ]
  let currentAssigned = false
  return definitions.map((definition, index) => {
    let state: WorkflowStepState
    if (running[definition.key]) {
      state = 'running'
      currentAssigned = true
    } else if (done[definition.key] && !currentAssigned) {
      state = 'done'
    } else if (!currentAssigned) {
      state = 'current'
      currentAssigned = true
    } else {
      state = done[definition.key] ? 'done' : 'pending'
    }
    return { ...definition, number: index + 1, state }
  })
}

export function currentWorkflowStep(steps: WorkflowStep[]) {
  return steps.find((step) => step.state === 'current' || step.state === 'running')
}
