import { Link } from 'react-router-dom'
import { useCapture, useDashboard, useSetupStatus, useSyncFichas } from '../hooks/api'
import { useToast } from '../hooks/useToast'
import { useWorkflow } from '../hooks/workflow'
import { friendlyError } from '../lib/friendlyError'
import { StepBadge } from './WorkflowSteps'

interface ActionProps {
  className?: string
  compact?: boolean
  fullWidth?: boolean
}

/** Paso 1: una sola sincronización a la vez (antes se podía lanzar 4 veces seguidas). */
export function SyncFichasAction({ className = '', compact = false, fullWidth = false }: ActionProps) {
  const toast = useToast()
  const { data: setup } = useSetupStatus()
  const syncFichas = useSyncFichas()
  const { step, isCurrent } = useWorkflow()
  const running = step('sync')?.state === 'running'
  const busy = syncFichas.isPending || running
  const classes = ['button', isCurrent('sync') ? 'primary is-next-step' : 'ghost', compact ? 'small' : '', className].filter(Boolean).join(' ')

  function handleSync() {
    if (busy) return
    syncFichas.mutate(
      { username: setup?.zajunaUsername || '', documentType: setup?.zajunaDocumentType || 'CC' },
      {
        onSuccess: () => toast('Estamos sincronizando tus fichas. Cuando termine, elige tu ficha y sigue con el paso 2.'),
        onError: (error) => toast(friendlyError(error.message), true),
      },
    )
  }

  return (
    <span className="workflow-action" style={fullWidth ? { width: '100%' } : undefined}>
      <button type="button" className={classes} onClick={handleSync} disabled={busy} style={fullWidth ? { width: '100%' } : undefined}>
        <StepBadge step="sync" />
        {busy ? 'Sincronizando…' : 'Sincronizar fichas'}
      </button>
    </span>
  )
}

/**
 * Paso 4: preparar evidencias solo cuando las rutas existen y hay actividades
 * seleccionadas; si falta algo, se dice qué y se enlaza al paso que falta.
 */
export function CaptureAction({ className = '', compact = false, fullWidth = false }: ActionProps) {
  const toast = useToast()
  const { data: setup } = useSetupStatus()
  const { data: dashboard } = useDashboard()
  const capture = useCapture()
  const { step, isCurrent, isLoading } = useWorkflow()
  const routes = step('routes')
  const activities = step('activities')
  const running = step('capture')?.state === 'running'
  const blocker = isLoading
    ? null
    : !dashboard?.activeFichaId
    ? { text: 'Primero elige una ficha (paso 1).', to: '/fichas', label: 'Ir al paso 1' }
    : routes?.state !== 'done'
      ? { text: 'Primero busca las rutas del curso (paso 2).', to: '/fichas', label: 'Ir al paso 2' }
      : activities?.state !== 'done'
        ? { text: 'Primero selecciona tus actividades técnicas (paso 3).', to: '/actividades', label: 'Ir al paso 3' }
        : null
  const busy = capture.isPending || running
  const classes = ['button', isCurrent('capture') ? 'primary is-next-step' : 'ghost', compact ? 'small' : '', className].filter(Boolean).join(' ')

  function handleCapture() {
    if (!dashboard?.activeFichaId || blocker || busy) return
    capture.mutate(
      { fichaId: dashboard.activeFichaId, username: setup?.zajunaUsername || '', documentType: setup?.zajunaDocumentType || 'CC' },
      {
        onSuccess: () => toast('Estamos preparando tus evidencias. Al terminar las revisamos automáticamente (paso 5).'),
        onError: (error) => toast(friendlyError(error.message), true),
      },
    )
  }

  return (
    <span className="workflow-action" style={fullWidth ? { width: '100%' } : undefined}>
      <button type="button" className={classes} onClick={handleCapture} disabled={isLoading || !!blocker || busy} style={fullWidth ? { width: '100%' } : undefined}>
        <StepBadge step="capture" />
        {busy ? 'Preparando evidencias…' : isLoading ? 'Comprobando los pasos…' : 'Preparar evidencias'}
      </button>
      {blocker ? (
        <small className="workflow-action-hint">
          {blocker.text} <Link to={blocker.to}>{blocker.label}</Link>
        </small>
      ) : null}
    </span>
  )
}
