import { Link } from 'react-router-dom'
import { useWorkflow } from '../hooks/workflow'
import type { WorkflowStepKey } from '../lib/workflow'

const STATE_LABEL = { done: 'Hecho', current: 'Siguiente paso', running: 'En curso', pending: 'Pendiente' } as const

/** Barra de 5 pasos visible en todas las páginas: señala una sola acción siguiente. */
export function WorkflowSteps() {
  const { steps, current, isLoading } = useWorkflow()
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
        {steps.map((step) => (
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
      {current ? (
        <p className="workflow-next" role="status">
          <b>Paso {current.number}:</b> {current.hint}
        </p>
      ) : (
        <p className="workflow-next" role="status">
          <b>¡Todo revisado!</b> Siguiente: <Link to="/reportes">generar el reporte PDF</Link>.
        </p>
      )}
    </nav>
  )
}

/** Insignia "Paso N" para el botón de un paso; resaltada cuando es el siguiente. */
export function StepBadge({ step }: { step: WorkflowStepKey }) {
  const { step: get, isCurrent } = useWorkflow()
  const entry = get(step)
  if (!entry) return null
  return (
    <span className={`step-badge ${isCurrent(step) ? 'current' : entry.state}`} aria-label={`Paso ${entry.number}${isCurrent(step) ? ', siguiente paso' : ''}`}>
      {entry.state === 'done' ? '✓ ' : ''}Paso {entry.number}
    </span>
  )
}
