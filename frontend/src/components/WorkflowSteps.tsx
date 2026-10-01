import { Link } from 'react-router-dom'
import { useChecklistGuides, useDashboard } from '../hooks/api'
import { useWorkflow } from '../hooks/workflow'
import type { WorkflowStep, WorkflowStepKey } from '../lib/workflow'

const STATE_LABEL = { done: 'Hecho', current: 'Siguiente paso', running: 'En curso', pending: 'Pendiente' } as const

/**
 * Aviso de una línea mientras falta un paso automático (fichas o rutas). Esos
 * pasos se hacen solos en el primer arranque y no se numeran.
 */
function AutomaticNotice({ step }: { step: WorkflowStep }) {
  if (step.state === 'running') {
    return (
      <p className="workflow-next workflow-auto" role="status">
        <i className="live-pulse" aria-hidden="true" />
        {step.key === 'sync' ? 'Estamos trayendo tus fichas desde Zajuna…' : 'Estamos preparando las rutas del curso…'}{' '}
        <Link to="/trabajos">Ver avance</Link>
      </p>
    )
  }
  return (
    <p className="workflow-next workflow-auto" role="status">
      {step.key === 'sync' ? step.hint : 'Aún no tenemos las rutas del curso de esta ficha.'}{' '}
      <Link to="/fichas">{step.key === 'sync' ? 'Ir a Fichas' : 'Buscar rutas'}</Link>
    </p>
  )
}

/** Barra de 3 pasos visible en las páginas de operación: señala una sola acción siguiente. */
export function WorkflowSteps() {
  const { visible, current, automaticPending, isLoading } = useWorkflow()
  if (isLoading) {
    // Sin datos todavía no mostramos un paso equivocado (antes parpadeaba).
    return (
      <nav className="workflow-steps loading" aria-label="Pasos para preparar tus evidencias" aria-busy="true">
        <p className="workflow-next">Comprobando en qué paso vas…</p>
      </nav>
    )
  }
  return (
    <nav className="workflow-steps" aria-label="Pasos para preparar tus evidencias">
      <ol>
        {visible.map((step) => (
          <li key={step.key} className={`workflow-step ${step.state}`}>
            <Link to={step.to} aria-current={current?.key === step.key ? 'step' : undefined} title={step.hint}>
              <b aria-hidden="true">{step.state === 'done' ? '✓' : step.number}</b>
              <span>
                <strong>{step.label}</strong>
                <small>{STATE_LABEL[step.state]}</small>
              </span>
            </Link>
          </li>
        ))}
      </ol>
      {automaticPending ? (
        <AutomaticNotice step={automaticPending} />
      ) : current ? (
        <p className="workflow-next" role="status">
          <b>Paso {current.number}:</b> {current.hint}
        </p>
      ) : (
        <ReviewedNotice />
      )}
    </nav>
  )
}

/** Insignia "Paso N" para el botón de un paso; resaltada cuando es el siguiente. Los automáticos no llevan. */
export function StepBadge({ step }: { step: WorkflowStepKey }) {
  const { step: get, isCurrent } = useWorkflow()
  const entry = get(step)
  if (!entry || entry.automatic) return null
  return (
    <span className={`step-badge ${isCurrent(step) ? 'current' : entry.state}`} aria-label={`Paso ${entry.number}${isCurrent(step) ? ', siguiente paso' : ''}`}>
      {entry.state === 'done' ? '✓ ' : ''}Paso {entry.number}
    </span>
  )
}

/**
 * Fin del flujo: si quedan ítems que dependen del instructor, se dice antes
 * del reporte (el PDF los mostraría como pendientes).
 */
function ReviewedNotice() {
  const { data: dashboard } = useDashboard()
  const guides = useChecklistGuides(dashboard?.activeFichaId).data?.guides.length || 0
  return (
    <p className="workflow-next" role="status">
      <b>¡Todo revisado!</b>{' '}
      {guides > 0 ? (
        <>
          {guides === 1 ? 'Queda 1 ítem' : `Quedan ${guides} ítems`} que dependen de ti:{' '}
          <Link to="/checklist?category=guia">ver guías</Link> o <Link to="/reportes">generar el reporte PDF</Link>.
        </>
      ) : (
        <>
          Siguiente: <Link to="/reportes">generar el reporte PDF</Link>.
        </>
      )}
    </p>
  )
}
