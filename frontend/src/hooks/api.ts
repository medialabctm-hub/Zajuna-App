import { useCallback, useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { ApiError, api } from '../api/client'
import { readStoredConnectionTest, writeStoredConnectionTest, type StoredConnectionTest } from '../lib/connectionStatus'
import type { Job, JobStatus } from '../types'

const POLL_MS = 5000
// Without active work nothing changes on its own: user actions and finished
// jobs already invalidate what they touch, so idle polling only has to catch
// changes made elsewhere (a scheduled run, another window).
const IDLE_POLL_MS = 30_000
const IDLE_JOBS_POLL_MS = 15_000
const ACTIVE_JOB_STATUSES: JobStatus[] = ['queued', 'running', 'waiting_user', 'retrying']

function hasActiveJobs(jobs: Job[] | undefined) {
  return !!jobs?.some((job) => ACTIVE_JOB_STATUSES.includes(job.status))
}

/** Polls fast only while a job is running; otherwise falls back to the idle rate. */
function activityPollInterval(queryClient: QueryClient) {
  return () => (hasActiveJobs(queryClient.getQueryData<Job[]>(['jobs'])) ? POLL_MS : IDLE_POLL_MS)
}

export function isNotFound(error: unknown) {
  return error instanceof ApiError && error.status === 404
}

function retryTransient(failureCount: number, error: unknown) {
  // A missing active ficha/map is an expected first-run state. Retrying it
  // only delays the empty-state action and creates noisy local traffic.
  return !isNotFound(error) && failureCount < 1
}

export function useSetupStatus() {
  return useQuery({ queryKey: ['setup'], queryFn: api.getSetupStatus })
}

export function useSettings() {
  return useQuery({ queryKey: ['settings'], queryFn: api.getSettings })
}

export function useSaveSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.saveSettings,
    onSuccess: (settings) => queryClient.setQueryData(['settings'], settings),
  })
}

export function useDiagnostics() {
  return useQuery({ queryKey: ['diagnostics'], queryFn: api.getDiagnostics, refetchInterval: IDLE_POLL_MS })
}

export function useNotifications() {
  const queryClient = useQueryClient()
  return useQuery({ queryKey: ['notifications'], queryFn: api.listNotifications, refetchInterval: activityPollInterval(queryClient) })
}

export function useMarkNotificationRead() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.markNotificationRead,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notifications'] }),
  })
}

export function useMarkAllNotificationsRead() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.markAllNotificationsRead,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notifications'] }),
  })
}

export function useBackups() {
  return useQuery({ queryKey: ['backups'], queryFn: api.listBackups })
}

export function useDeleteBackup() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.deleteBackup,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['backups'] }),
  })
}

export function useCleanupBackups() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.cleanupBackups,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['backups'] }),
  })
}

export function useRestoreBackup() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.restoreBackup,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['backups'] }),
  })
}

export function useSaveSetup() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.saveSetup,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['setup'] }),
  })
}

/**
 * Encola la prueba real de conexión (`test-zajuna-connection`) y sigue el
 * resultado del último job. El id se recuerda en este equipo para que el
 * estado sobreviva a recargas; se olvida al guardar credenciales nuevas.
 */
export function useZajunaConnectionTest(username?: string) {
  const queryClient = useQueryClient()
  const [stored, setStored] = useState<StoredConnectionTest | null>(() => readStoredConnectionTest())
  const jobId = stored && stored.username === (username || '') ? stored.jobId : undefined
  const jobQuery = useJob(jobId)
  const mutation = useMutation({
    mutationFn: api.testZajunaConnection,
    onSuccess: (job) => {
      const next = { jobId: job.id, username: username || '' }
      writeStoredConnectionTest(next)
      setStored(next)
      queryClient.setQueryData(['job', job.id], job)
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
    },
  })
  const forget = useCallback(() => {
    writeStoredConnectionTest(null)
    setStored(null)
  }, [])
  const job = jobQuery.isError ? undefined : jobQuery.data
  return { job, start: mutation.mutateAsync, isStarting: mutation.isPending, forget }
}

export function useFichas() {
  const queryClient = useQueryClient()
  return useQuery({ queryKey: ['fichas'], queryFn: () => api.listFichas(100), refetchInterval: activityPollInterval(queryClient) })
}

export function useSyncFichas() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.syncFichas,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['fichas'] })
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
    },
  })
}

export function useSetActiveFicha() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.setActiveFicha,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      queryClient.invalidateQueries({ queryKey: ['evidenceGroups'] })
      queryClient.invalidateQueries({ queryKey: ['evidences'] })
      queryClient.invalidateQueries({ queryKey: ['targets'] })
      queryClient.invalidateQueries({ queryKey: ['reviews'] })
    },
  })
}

export function useJobs() {
  const queryClient = useQueryClient()
  const previousStatuses = useRef<Record<string, string>>({})
  const query = useQuery({
    queryKey: ['jobs'],
    queryFn: () => api.listJobs(50),
    refetchInterval: (current) => (hasActiveJobs(current.state.data) ? POLL_MS : IDLE_JOBS_POLL_MS),
  })

  useEffect(() => {
    if (!query.data) return
    const terminal = new Set(['completed', 'failed', 'cancelled'])
    const previous = previousStatuses.current
    const completedCapture = query.data.some((job) => {
      const wasActive = ACTIVE_JOB_STATUSES.includes(previous[job.id] as JobStatus)
      return wasActive && terminal.has(job.status)
    })
    const startedWork = query.data.some(
      (job) => ACTIVE_JOB_STATUSES.includes(job.status) && !ACTIVE_JOB_STATUSES.includes(previous[job.id] as JobStatus),
    )
    previousStatuses.current = Object.fromEntries(query.data.map((job) => [job.id, job.status]))
    // Refetching now moves the dashboard to the fast rate without waiting
    // for its next idle tick.
    if (startedWork) queryClient.invalidateQueries({ queryKey: ['dashboard'] })
    if (!completedCapture) return
    queryClient.invalidateQueries({ queryKey: ['dashboard'] })
    queryClient.invalidateQueries({ queryKey: ['evidenceGroups'] })
    queryClient.invalidateQueries({ queryKey: ['evidences'] })
    queryClient.invalidateQueries({ queryKey: ['activities'] })
    queryClient.invalidateQueries({ queryKey: ['targets'] })
    queryClient.invalidateQueries({ queryKey: ['reviews'] })
    queryClient.invalidateQueries({ queryKey: ['reports'] })
    queryClient.invalidateQueries({ queryKey: ['evidenceReview'] })
    // Finished jobs create notifications and a ficha sync rewrites the list.
    queryClient.invalidateQueries({ queryKey: ['notifications'] })
    queryClient.invalidateQueries({ queryKey: ['fichas'] })
  }, [query.data, queryClient])

  return query
}

export function useDismissJobs() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.dismissJobs,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
      queryClient.invalidateQueries({ queryKey: ['job'] })
    },
  })
}

export function useAppInfo() {
  return useQuery({ queryKey: ['appInfo'], queryFn: api.getAppInfo, staleTime: 60_000 })
}

export function useResetApp() {
  return useMutation({ mutationFn: api.resetApp })
}

export function useJob(jobId?: string) {
  return useQuery({
    queryKey: ['job', jobId],
    queryFn: () => api.getJob(jobId as string),
    enabled: !!jobId,
    retry: retryTransient,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status && ['completed', 'failed', 'cancelled'].includes(status) ? false : POLL_MS
    },
  })
}

export function useJobEvents(jobId?: string, enabled = true, jobStatus?: JobStatus) {
  const terminal = jobStatus && ['completed', 'failed', 'cancelled'].includes(jobStatus)
  return useQuery({
    queryKey: ['jobEvents', jobId],
    queryFn: () => api.getJobEvents(jobId as string),
    enabled: !!jobId && enabled,
    retry: retryTransient,
    refetchInterval: enabled && !terminal ? POLL_MS : false,
  })
}

export function useCancelJob() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.cancelJob,
    onSuccess: (_data, jobId) => {
      queryClient.invalidateQueries({ queryKey: ['job', jobId] })
      queryClient.invalidateQueries({ queryKey: ['jobEvents', jobId] })
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
    },
  })
}

export function useSchedules() {
  const queryClient = useQueryClient()
  return useQuery({ queryKey: ['schedules'], queryFn: api.listSchedules, refetchInterval: activityPollInterval(queryClient) })
}

export function useCreateSchedule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.createSchedule,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['schedules'] }),
  })
}

export function useSetScheduleEnabled() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => api.setScheduleEnabled(id, enabled),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['schedules'] }),
  })
}

export function useDashboard(fichaId?: string) {
  const queryClient = useQueryClient()
  const pollInterval = activityPollInterval(queryClient)
  return useQuery({
    queryKey: ['dashboard', fichaId ?? 'active'],
    queryFn: () => api.getDashboard(fichaId),
    retry: retryTransient,
    // Sin ficha activa el core responde 404. Volver a pedirlo cada vez que se
    // monta un componente que lo usa devolvía la consulta a «cargando», el
    // Resumen desmontaba y montaba sus botones y eso entraba en bucle (cientos
    // de peticiones por segundo). Elegir una ficha invalida 'dashboard'.
    retryOnMount: false,
    refetchInterval: (query) => (isNotFound(query.state.error) ? false : pollInterval()),
  })
}

export function useChecklistItemDetail(itemCode?: string, fichaId?: string) {
  return useQuery({
    queryKey: ['checklistItemDetail', fichaId, itemCode],
    queryFn: () => api.getChecklistItemDetail(itemCode as string, fichaId),
    enabled: !!itemCode && !!fichaId,
    retry: retryTransient,
  })
}

export function useActivities(fichaId?: string) {
  return useQuery({
    queryKey: ['activities', fichaId],
    queryFn: () => api.getActivities(fichaId as string),
    enabled: !!fichaId,
    retry: retryTransient,
    staleTime: 10_000,
  })
}

export function useSaveActivities() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.saveActivities,
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['activities', variables.fichaId] })
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
    },
  })
}

export function useReviews(fichaId?: string) {
  return useQuery({
    queryKey: ['reviews', fichaId],
    queryFn: () => api.getReviews(fichaId as string),
    enabled: !!fichaId,
    retry: retryTransient,
  })
}

export function useSaveReview() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.saveReview,
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['reviews', variables.fichaId] })
      queryClient.invalidateQueries({ queryKey: ['targets', variables.fichaId] })
    },
  })
}

export function useTargets(fichaId?: string) {
  return useQuery({
    queryKey: ['targets', fichaId],
    queryFn: () => api.getTargets(fichaId as string),
    enabled: !!fichaId,
    retry: retryTransient,
  })
}

export function useSetItemStatus() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ itemCode, ...input }: { itemCode: string; fichaId: string; status: string }) =>
      api.setItemStatus(itemCode, input),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['checklistItemDetail', variables.fichaId, variables.itemCode] })
    },
  })
}

export function useCapture() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.capture,
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
      queryClient.invalidateQueries({ queryKey: ['dashboard', variables.fichaId] })
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['evidenceGroups', variables.fichaId] })
      queryClient.invalidateQueries({ queryKey: ['evidences', variables.fichaId] })
      queryClient.invalidateQueries({ queryKey: ['activities', variables.fichaId] })
      queryClient.invalidateQueries({ queryKey: ['targets', variables.fichaId] })
      queryClient.invalidateQueries({ queryKey: ['reviews', variables.fichaId] })
    },
  })
}

export function useDiscoverCourseMaps() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.discoverCourseMaps,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
      queryClient.invalidateQueries({ queryKey: ['fichas'] })
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['activities'] })
      queryClient.invalidateQueries({ queryKey: ['targets'] })
      queryClient.invalidateQueries({ queryKey: ['reviews'] })
    },
  })
}

export function useEvidenceGroups(fichaId?: string) {
  return useQuery({
    queryKey: ['evidenceGroups', fichaId],
    queryFn: () => api.getEvidenceGroups(fichaId as string),
    enabled: !!fichaId,
    retry: retryTransient,
  })
}

export function useEvidences(fichaId?: string) {
  return useQuery({
    queryKey: ['evidences', fichaId],
    queryFn: () => api.listEvidences(fichaId),
    enabled: !!fichaId,
    retry: retryTransient,
  })
}

export function useRebuildEvidenceGroups() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.rebuildEvidenceGroups,
    onSuccess: (_data, fichaId) => queryClient.invalidateQueries({ queryKey: ['evidenceGroups', fichaId] }),
  })
}

export function useUploadEvidence() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.uploadEvidence,
    onSuccess: (_data, variables) => {
      const fichaId = variables.get('fichaId')
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['evidenceGroups'] })
      queryClient.invalidateQueries({ queryKey: ['evidences', fichaId] })
    },
  })
}

export function useDeleteEvidence() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.deleteEvidence,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['evidenceGroups'] })
      queryClient.invalidateQueries({ queryKey: ['evidences'] })
    },
  })
}

export function useClearEvidences() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (fichaId?: string) => api.clearEvidences(fichaId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['evidences'] })
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['checklist'] })
    },
  })
}

export function useReports() {
  const queryClient = useQueryClient()
  return useQuery({ queryKey: ['reports'], queryFn: () => api.listReports(50), refetchInterval: activityPollInterval(queryClient) })
}

export function useGenerateReport() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.generateReport,
    onSuccess: () => {
      // The report row appears when its job finishes; tracking the job keeps
      // polling fast until then.
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
      queryClient.invalidateQueries({ queryKey: ['reports'] })
    },
  })
}

export function useCreateBackup() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.createBackup,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['backups'] }),
  })
}

export function useEvidenceReview(fichaId?: string) {
  return useQuery({
    queryKey: ['evidenceReview', fichaId],
    queryFn: () => api.getEvidenceReview(fichaId as string),
    enabled: !!fichaId,
    retry: retryTransient,
    refetchOnWindowFocus: true,
  })
}

export function useVerifyEvidences() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (fichaId: string) => api.verifyEvidences(fichaId),
    onSuccess: (data, fichaId) => {
      queryClient.setQueryData(['evidenceReview', fichaId], data)
      queryClient.invalidateQueries({ queryKey: ['evidenceReview'] })
      // Verifying marks fully approved items as fulfilled in the checklist.
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
    },
  })
}

export function useSetEvidenceReview() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ evidenceId, ...input }: { evidenceId: string; status: 'approved' | 'pending' | 'rejected'; note?: string }) =>
      api.setEvidenceReview(evidenceId, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['evidenceReview'] })
      // A decision can mark (or withdraw) the item in the checklist.
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
    },
  })
}
