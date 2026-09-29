import { Link } from 'react-router-dom'
import { useCapture, useDashboard, useDismissJobs, useSetupStatus, useSyncFichas, useDiscoverCourseMaps } from '../hooks/api'
import { useToast } from '../hooks/useToast'
import { friendlyError } from '../lib/friendlyError'
import { explainJobFailure } from '../lib/jobFailure'
import type { Job } from '../types'
import { RouteDiscoveryAction } from './RouteDiscoveryAction'

const RETRYABLE_TYPES = new Set(['sync-fichas', 'discover-course-maps', 'capture-checklist', 'export-report'])

/**
 * Explains a failed/cancelled job as "qué pasó · por qué · qué hacer" and
 * offers the matching action, instead of the raw core message.
 */
export function FailureGuide({ job, compact = false, showDismiss = false }: { job: Job; compact?: boolean; showDismiss?: boolean }) {
  const toast = useToast()
  const { data: setup } = useSetupStatus()
  const { data: dashboard } = useDashboard()
  const syncFichas = useSyncFichas()
  const discover = useDiscoverCourseMaps()
  const capture = useCapture()
  const dismiss = useDismissJobs()
  const explanation = explainJobFailure(job)
  const account = { username: setup?.zajunaUsername || '', documentType: setup?.zajunaDocumentType || 'CC' }
  const busy = syncFichas.isPending || discover.isPending || capture.isPending

  const onError = (error: Error) => toast(friendlyError(error.message), true)

  function retry() {
    if (job.type === 'sync-fichas') {
      syncFichas.mutate(account, { onSuccess: () => toast('Volvimos a sincronizar tus fichas.'), onError })
    } else if (job.type === 'discover-course-maps') {
      // Reintenta la ficha que falló, no la que esté activa ahora.
      const fichaId = job.fichaId || dashboard?.activeFichaId
      if (!fichaId) {
        toast('Elige una ficha activa antes de buscar rutas.', true)
        return
      }
      discover.mutate({ ...account, fichaId }, { onSuccess: () => toast('Volvimos a buscar las rutas del curso.'), onError })
    } else if (job.type === 'capture-checklist') {
      // Reintenta exactamente lo que falló: su ficha y sus ítems.
      const fichaId = job.fichaId || dashboard?.activeFichaId
      if (!fichaId) {
        toast('Elige una ficha activa antes de preparar evidencias.', true)
        return
      }
      capture.mutate({ fichaId, ...account, ...(job.itemCodes?.length ? { itemCodes: job.itemCodes } : {}) }, { onSuccess: () => toast('Estamos preparando tus evidencias de nuevo.'), onError })
    }
  }

  function handleDismiss() {
    dismiss.mutate([job.id], {
      onSuccess: () => toast('Aviso descartado. El proceso sigue disponible en Trabajos.'),
      onError,
    })
  }

  const actions = explanation.actions
    .filter((action) => action.kind !== 'retry' || RETRYABLE_TYPES.has(job.type))
    .slice(0, compact ? 2 : 3)

  return (
    <div className={`failure-guide${compact ? ' compact' : ''}`} role={compact ? undefined : 'alert'}>
      <strong>{explanation.title}</strong>
      {compact ? null : <p>{explanation.cause}</p>}
      <p className="failure-next"><b>Qué hacer:</b> {explanation.next}</p>
      {actions.length || showDismiss ? (
        <div className="failure-actions">
          {actions.map((action) => {
            if (action.kind === 'retry') {
              if (job.type === 'export-report') {
                return <Link key="retry" className="button small" to="/reportes">Generar de nuevo</Link>
              }
              return (
                <button key="retry" type="button" className="button small" onClick={retry} disabled={busy}>
                  {busy ? 'Enviando…' : action.label}
                </button>
              )
            }
            if (action.kind === 'discover') return <RouteDiscoveryAction key="discover" compact variant="primary" />
            return (
              <Link key={action.kind} className="button ghost small" to={action.to || '/resumen'}>
                {action.label}
              </Link>
            )
          })}
          {showDismiss && !job.dismissed ? (
            <button type="button" className="button ghost small" onClick={handleDismiss} disabled={dismiss.isPending} title="Quitar de “Requiere tu atención”. El proceso sigue en Trabajos.">
              Descartar aviso
            </button>
          ) : null}
        </div>
      ) : null}
      {!compact && explanation.technical ? (
        <details>
          <summary>Detalle técnico (para soporte)</summary>
          <code>{explanation.technical}</code>
        </details>
      ) : null}
    </div>
  )
}
