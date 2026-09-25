import { useActivities, useDashboard, useEvidenceReview, useFichas, useJobs, useTargets } from './api'
import { approvedItemsNotMarked, computeWorkflow, currentWorkflowStep, type WorkflowStep, type WorkflowStepKey } from '../lib/workflow'

const ACTIVE = ['queued', 'running', 'waiting_user', 'retrying']

/** Estado del flujo guiado (1 sincronizar → 5 revisar) con datos reales. */
export function useWorkflow() {
  const fichasQuery = useFichas()
  const dashboardQuery = useDashboard()
  const activeFichaId = dashboardQuery.data?.activeFichaId
  const targetsQuery = useTargets(activeFichaId)
  const activitiesQuery = useActivities(activeFichaId)
  const jobsQuery = useJobs()
  const reviewQuery = useEvidenceReview(activeFichaId)
  const jobs = jobsQuery.data || []
  const running = (type: string) => jobs.some((job) => job.type === type && ACTIVE.includes(job.status))
  const evidenceCount = (dashboardQuery.data?.items || []).reduce((sum, item) => sum + (Number(item.evidenceCount) || 0), 0)
  const summary = reviewQuery.data?.summary
  const steps = computeWorkflow({
    fichasCount: fichasQuery.data?.length || 0,
    hasActiveFicha: !!activeFichaId,
    syncRunning: running('sync-fichas'),
    mapReady: targetsQuery.data?.mapReady === true,
    discoverRunning: running('discover-course-maps'),
    selectedActivities: activitiesQuery.data?.selectedCount || 0,
    evidenceCount,
    captureRunning: running('capture-checklist'),
    reviewOpen: summary ? (Number(summary.pending) || 0) + (Number(summary.rejected) || 0) : undefined,
    reviewTotal: summary ? Number(summary.total) || 0 : undefined,
    unmarkedApproved: reviewQuery.data ? approvedItemsNotMarked(reviewQuery.data.evidences ?? [], dashboardQuery.data?.items ?? []).length : undefined,
  })
  const current = currentWorkflowStep(steps)
  const step = (key: WorkflowStepKey) => steps.find((entry) => entry.key === key) as WorkflowStep
  // Mientras cargan los datos no se sabe qué paso falta: no mostrar bloqueos.
  const isLoading = dashboardQuery.isLoading || (!!activeFichaId && (targetsQuery.isLoading || activitiesQuery.isLoading))
  return { steps, current, step, isLoading, isCurrent: (key: WorkflowStepKey) => current?.key === key }
}
