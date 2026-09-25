import { useState } from 'react'
import type { ChangeEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { PageError, PageSkeleton } from '../components/AsyncState'
import {
  useActivities,
  useDashboard,
  useFichas,
  useSetActiveFicha,
} from '../hooks/api'
import { formatDate } from '../lib/format'
import { useToast } from '../hooks/useToast'
import { friendlyError } from '../lib/friendlyError'
import type { Ficha } from '../types'
import { RouteDiscoveryAction } from '../components/RouteDiscoveryAction'
import { CaptureAction, SyncFichasAction } from '../components/WorkflowActions'
import { StepBadge } from '../components/WorkflowSteps'

function FichaTableRow({
  ficha,
  isActive,
  percentage,
  evidenceCount,
  onSelect,
  onOpen,
  disabled,
}: {
  ficha: Ficha
  isActive: boolean
  percentage: number
  evidenceCount: number
  onSelect: (fichaId: string) => void
  onOpen: () => void
  disabled: boolean
}) {
  return (
    <div className={`ficha-table-row${isActive ? ' active' : ''}`} role="row">
      <div role="cell">
        <strong className="mono">{ficha.externalId}</strong>
        <small>{isActive ? 'Ficha activa' : 'Disponible'}</small>
      </div>
      <div role="cell">
        <strong>{ficha.name}</strong>
      </div>
      <span className="mono" role="cell">{ficha.courseId}</span>
      <div role="cell">
        {isActive ? (
          <>
            <div className="progress" style={{ marginTop: 0 }} role="progressbar" aria-label="Cumplimiento" aria-valuenow={percentage} aria-valuemin={0} aria-valuemax={100}>
              <i style={{ width: `${percentage}%` }} />
            </div>
            <small style={{ marginTop: 4 }}>{percentage}%</small>
          </>
        ) : (
          <span>—</span>
        )}
      </div>
      <span role="cell">{isActive ? evidenceCount : '—'}</span>
      <span role="cell">{formatDate(ficha.updatedAt)}</span>
      <div role="cell">
        <button
          className={`button ${isActive ? '' : 'secondary'} small`.trim()}
          disabled={disabled}
          onClick={() => (isActive ? onOpen() : onSelect(ficha.id))}
        >
          {isActive ? 'Abrir resumen' : 'Seleccionar'}
        </button>
      </div>
    </div>
  )
}

export function Fichas() {
  const navigate = useNavigate()
  const toast = useToast()

  const fichasQuery = useFichas()
  const fichasData = fichasQuery.data
  const { data: dashboard } = useDashboard()

  const setActiveFicha = useSetActiveFicha()

  const [fichaQuery, setFichaQuery] = useState('')

  const fichas = fichasData || []
  const active = dashboard?.activeFichaId
  const activitiesQuery = useActivities(active)
  const activities = activitiesQuery.data

  if (fichasQuery.isLoading) return <PageSkeleton label="Cargando fichas locales" />
  if (fichasQuery.isError) return <PageError message="No pudimos cargar las fichas locales." action={<button className="button" onClick={() => fichasQuery.refetch()}>Reintentar</button>} />

  const activeFicha = fichas.find((ficha) => ficha.id === active)
  const summary = dashboard?.summary
  const activeEvidenceCount = (dashboard?.items || []).reduce(
    (sum, item) => sum + (Number(item.evidenceCount) || 0),
    0,
  )

  const query = fichaQuery.trim().toLowerCase()
  const visible = fichas.filter(
    (ficha) =>
      !query ||
      [ficha.externalId, ficha.name, ficha.courseId].some((value) =>
        String(value || '').toLowerCase().includes(query),
      ),
  )


  const handleSelect = (fichaId: string) => {
    setActiveFicha.mutate(fichaId, {
      onSuccess: () => toast('Ficha seleccionada.'),
      onError: (error: Error) => toast(friendlyError(error.message), true),
    })
  }

  const handleOpenChecklist = () => navigate('/checklist')

  const handleQueryChange = (event: ChangeEvent<HTMLInputElement>) => {
    setFichaQuery(event.target.value)
  }

  return (
    <div className="fichas-layout">
      <section className="card active-ficha-card">
        <div className="active-ficha-head">
          <div>
            <div className="eyebrow">
              Ficha activa
            </div>
            <h2>{activeFicha ? activeFicha.name : 'Sin ficha seleccionada'}</h2>
            <div className="active-ficha-meta">
              <span>{activeFicha ? `Ficha ${activeFicha.externalId}` : 'Selecciona una ficha para comenzar'}</span>
              <span>Curso {activeFicha ? activeFicha.courseId : '—'}</span>
              <span>Actualizada {activeFicha ? formatDate(activeFicha.updatedAt) : '—'}</span>
            </div>
          </div>
          <SyncFichasAction />
        </div>
        <div className="active-ficha-grid">
          <div className="active-ficha-stat">
            <strong>{Number(summary?.percentage) || 0}%</strong>
            <span>Cumplimiento</span>
          </div>
          <div className="active-ficha-stat">
            <strong>{Number(summary?.yes) || 0}</strong>
            <span>Ítems cumplidos</span>
          </div>
          <div className="active-ficha-stat">
            <strong>{activeEvidenceCount}</strong>
            <span>Evidencias guardadas</span>
          </div>
          <div className="active-ficha-stat">
            <strong>{activities?.selectedCount || 0}</strong>
            <span>Actividades seleccionadas</span>
          </div>
        </div>
        <div className="ficha-actions" aria-label="Acciones de la ficha activa">
          <button className="button navy small" disabled={!active} onClick={handleOpenChecklist}>
            Abrir checklist
          </button>
          <div className="ficha-action-group">
            <span className="ficha-action-label">Mapa del curso</span>
            <RouteDiscoveryAction compact variant="primary" />
          </div>
          <div className="ficha-action-group">
            <span className="ficha-action-label">Tus actividades</span>
            <Link className="button ghost small" to="/actividades">
              <StepBadge step="activities" />
              Seleccionar actividades
            </Link>
          </div>
          <div className="ficha-action-group">
            <span className="ficha-action-label">Captura</span>
            <CaptureAction compact />
          </div>
        </div>
      </section>
      <section className="card">
        <div className="card-pad">
          <div className="section-toolbar">
            <div>
              <h3>Fichas disponibles</h3>
              <p className="helper">Elige una ficha para trabajar y mantén tus datos disponibles en este equipo.</p>
            </div>
            <input
              className="section-search"
              type="search"
              value={fichaQuery}
              onChange={handleQueryChange}
              placeholder="Buscar por ficha o programa"
              aria-label="Buscar por ficha o programa"
            />
          </div>
        </div>
        <div className="ficha-table" role="table" aria-label="Fichas disponibles">
          <div className="ficha-table-head" role="row">
            <span role="columnheader">Ficha</span>
            <span role="columnheader">Programa</span>
            <span role="columnheader">Curso</span>
            <span role="columnheader">Cumplimiento</span>
            <span role="columnheader">Evidencias</span>
            <span role="columnheader">Actualizada</span>
            <span role="columnheader">Acciones</span>
          </div>
          {visible.length ? (
            visible.map((ficha) => {
              const isActive = ficha.id === active
              const percentage = isActive ? Number(summary?.percentage) || 0 : 0
              return (
                <FichaTableRow
                  key={ficha.id}
                  ficha={ficha}
                  isActive={isActive}
                  percentage={percentage}
                  evidenceCount={activeEvidenceCount}
                  onSelect={handleSelect}
                  onOpen={() => navigate('/resumen')}
                  disabled={setActiveFicha.isPending}
                />
              )
            })
          ) : (
            <div className="ficha-table-empty" role="row">
              <div className="empty" role="cell">
                {query ? 'No encontramos fichas con esa búsqueda.' : 'Todavía no hay fichas sincronizadas.'}
              </div>
            </div>
          )}
        </div>
        <div className="ficha-table-footer">
          <span className="helper">
            Mostrando {visible.length} de {fichas.length} fichas
          </span>
        </div>
      </section>
    </div>
  )
}
