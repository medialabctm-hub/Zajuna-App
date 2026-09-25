import { Fragment, useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import {
  useActivities,
  useDashboard,
  useDeleteEvidence,
  useDiscoverCourseMaps,
  useGenerateReport,
  useReviews,
  useSaveReview,
  useSetItemStatus,
  useSetupStatus,
  useTargets,
  isNotFound,
} from '../hooks/api'
import {
  confidenceFor,
  confidenceSummary,
  formatDate,
  ROUTE_GROUP_LABELS,
  ROUTE_KIND_LABELS,
  routeStatusClass,
  routeStatusLabel,
} from '../lib/format'
import { Icon } from '../components/Icon'
import { MissingActiveFicha, PageError, PageSkeleton } from '../components/AsyncState'
import { evidenceDownloadUrl } from '../api/client'
import { useToast } from '../hooks/useToast'
import { friendlyError } from '../lib/friendlyError'
import { RouteDiscoveryAction } from '../components/RouteDiscoveryAction'
import { CaptureAction } from '../components/WorkflowActions'
import { ActivitySelector } from '../components/ActivitySelector'
import type {
  DashboardCategory,
  DashboardItem,
  Evidence,
  ItemStatus,
  RouteReview,
  RouteReviewStatus,
  RouteTarget,
  TargetsResponse,
} from '../types'

interface RouteGroup {
  key: string
  target: RouteTarget
  itemCodes: string[]
  names: string[]
  slots: number
}

function computeRouteKey(target: RouteTarget): string {
  return `${target.groupName}|${target.url || target.cssSelector || target.itemCode}`
}

function getRouteStatus(reviews: RouteReview[], key: string): RouteReviewStatus {
  return reviews.find((review) => review.routeKey === key)?.status || 'review'
}

function humanRoute(target: RouteTarget): string {
  const kindLabel = ROUTE_KIND_LABELS[target.routeKind] || 'sección del curso'
  const location = target.url || target.cssSelector
  return location ? `${kindLabel} · ${location}` : kindLabel
}

function TaskRow({
  item,
  onPreview,
  onSetStatus,
}: {
  item: DashboardItem
  onPreview: (evidence: Evidence) => void
  onSetStatus: (itemCode: string, status: ItemStatus) => void
}) {
  const confidence = confidenceFor(item)
  const maxEvidences = Number(item.maxEvidences) || 1
  const filled = Math.min(Number(item.evidenceCount) || 0, maxEvidences)
  const current = item.status || 'PENDIENTE'
  const segments: { value: ItemStatus; label: string }[] = [
    { value: 'SI', label: 'Sí' },
    { value: 'NO', label: 'No' },
    { value: 'PENDIENTE', label: 'Pend.' },
  ]
  return (
    <article className="task task-grid">
      <span className="task-code mono">{item.itemCode}</span>
      <div className="task-main">
        <div className="task-desc">{item.description}</div>
        <div className="task-meta">
          <span>{item.categoryLabel}</span>
          <span className={`confidence ${confidence.key}`} title={confidence.detail}>
            {confidence.label}
          </span>
        </div>
        <div className="task-evidences">
          {item.evidences && item.evidences.length ? (
            item.evidences.map((evidence) => (
              <button key={evidence.id} className="evidence-link" onClick={() => onPreview(evidence)}>
                Ver evidencia {evidence.slotNumber || 1}
              </button>
            ))
          ) : (
            <span className="helper">Aún no hay un archivo</span>
          )}
        </div>
        <Link className="task-detail-link" to={`/checklist/${encodeURIComponent(item.itemCode)}`}>
          Ver detalle
        </Link>
      </div>
      <div className="task-slots">
        <div className="slot-row">
          {Array.from({ length: maxEvidences }).map((_, index) => (
            <span key={index} className={`slot-dot${index < filled ? ' filled' : ''}`} />
          ))}
        </div>
        <small>
          {filled} / {maxEvidences}
        </small>
      </div>
      <div className="status-seg-group" role="group" aria-label="Cambiar estado">
        {segments.map((segment) => (
          <button
            key={segment.value}
            type="button"
            className={`status-seg ${segment.value.toLowerCase()} ${current === segment.value ? 'active' : ''}`}
            aria-pressed={current === segment.value}
            onClick={() => onSetStatus(item.itemCode, segment.value)}
          >
            {segment.label}
          </button>
        ))}
      </div>
    </article>
  )
}

function ConfidenceSummarySection({
  items,
  routeReviewOpen,
  onToggleRoutes,
}: {
  items: DashboardItem[]
  routeReviewOpen: boolean
  onToggleRoutes: () => void
}) {
  const summary = confidenceSummary(items)
  return (
    <section className="card confidence-card">
      <div className="card-pad">
        <div className="confidence-intro">
          <div>
            <h3>Confianza de las evidencias</h3>
            <p className="helper" style={{ marginTop: 5 }}>
              Las rutas automáticas pueden confirmarse o corregirse cuando lo necesites.
            </p>
          </div>
          <button className="button ghost small" onClick={onToggleRoutes}>
            {routeReviewOpen ? 'Ocultar rutas' : 'Ver rutas'}
          </button>
        </div>
        <div className="confidence-grid">
          <div className="confidence-stat high">
            <strong>{summary.high}</strong>
            <span>Confirmadas</span>
          </div>
          <div className="confidence-stat review">
            <strong>{summary.review}</strong>
            <span>Por revisar</span>
          </div>
          <div className="confidence-stat empty">
            <strong>{summary.empty}</strong>
            <span>Sin evidencia</span>
          </div>
        </div>
      </div>
    </section>
  )
}

function RouteEntry({
  group,
  index,
  review,
  manualEdit,
  onManualChange,
  onSaveRoute,
}: {
  group: RouteGroup
  index: number
  review: RouteReview | undefined
  manualEdit: { manualUrl?: string; manualSelector?: string } | undefined
  onManualChange: (key: string, field: 'manualUrl' | 'manualSelector', value: string) => void
  onSaveRoute: (key: string, status: RouteReviewStatus) => void
}) {
  const decision = review?.status || 'review'
  const target = group.target
  const label = ROUTE_GROUP_LABELS[target.groupName] || 'Ruta de evidencia'
  const codeText = group.itemCodes.slice(0, 6).join(' · ') + (group.itemCodes.length > 6 ? ' · …' : '')
  const nameText = group.names.slice(0, 2).join(' · ') + (group.names.length > 2 ? ' · …' : '')
  const manualUrl = manualEdit?.manualUrl ?? review?.manualUrl ?? ''
  const manualSelector = manualEdit?.manualSelector ?? review?.manualSelector ?? ''
  return (
    <details className="route-entry" open={decision === 'review' && index === 0}>
      <summary>
        <span className="route-summary-copy">
          <span>
            <Icon name="route" size={16} />
          </span>
          <span>
            <strong>{label}</strong>
            <small>
              {codeText} · {group.slots} evidencia{group.slots === 1 ? '' : 's'}
            </small>
          </span>
        </span>
        <span className={`status-chip ${routeStatusClass(decision)}`}>{routeStatusLabel(decision)}</span>
      </summary>
      <div className="route-detail">
        <div className="route-detail-grid">
          <div className="route-detail-box">
            <span className="route-label">Qué se espera</span>
            <p>{nameText || 'Contenido relacionado con esta sección'}</p>
          </div>
          <div className="route-detail-box">
            <span className="route-label">Dónde se encontró</span>
            <p>{humanRoute(target)}</p>
          </div>
        </div>
        <div className="route-meta">
          <span className="badge">
            {target.itemCode}
            {target.activityTitle ? ` · ${target.activityTitle}` : ''}
          </span>
          <span className="badge">{target.cssSelector ? 'Captura dirigida' : 'Ruta general'}</span>
          {target.revealSelectors?.length ? <span className="badge">Despliega contenido</span> : null}
        </div>
        <p className="route-note">
          {decision === 'confirmed'
            ? 'La ruta quedó confirmada y se usará en la próxima captura.'
            : decision === 'correction'
              ? 'La ruta quedó marcada para corregir antes de capturar.'
              : 'Revisa la sección y confirma que corresponde. Los foros se limitarán al contenido del instructor cuando aplique.'}
        </p>
        <details className="route-advanced">
          <summary>Editar manualmente</summary>
          <div className="route-advanced-grid">
            <label>
              Enlace alternativo
              <input
                value={manualUrl}
                onChange={(event) => onManualChange(group.key, 'manualUrl', event.target.value)}
                placeholder="Opcional"
              />
            </label>
            <label>
              Referencia de sección
              <input
                value={manualSelector}
                onChange={(event) => onManualChange(group.key, 'manualSelector', event.target.value)}
                placeholder="Opcional"
              />
            </label>
          </div>
          <p className="helper" style={{ marginTop: 8 }}>
            Usa estos campos solo si la ruta automática necesita una corrección específica.
          </p>
          <div className="route-actions">
            <button className="button ghost small" onClick={() => onSaveRoute(group.key, decision)}>
              Guardar ajuste
            </button>
          </div>
        </details>
        <div className="route-actions">
          <button className="button ghost small" onClick={() => onSaveRoute(group.key, 'correction')}>
            Marcar para corregir
          </button>
          <button className="button small" onClick={() => onSaveRoute(group.key, 'confirmed')}>
            Confirmar ruta
          </button>
        </div>
      </div>
    </details>
  )
}

function RouteReviewSection({
  targetsData,
  reviews,
  routeFilter,
  routeQuery,
  onFilterChange,
  onQueryChange,
  manualEdits,
  onManualChange,
  onSaveRoute,
  onDiscover,
  isDiscovering,
  targetsError,
  reviewsError,
  onRetryTargets,
}: {
  targetsData: TargetsResponse | undefined
  reviews: RouteReview[]
  routeFilter: 'all' | RouteReviewStatus
  routeQuery: string
  onFilterChange: (value: 'all' | RouteReviewStatus) => void
  onQueryChange: (value: string) => void
  manualEdits: Record<string, { manualUrl?: string; manualSelector?: string }>
  onManualChange: (key: string, field: 'manualUrl' | 'manualSelector', value: string) => void
  onSaveRoute: (key: string, status: RouteReviewStatus) => void
  onDiscover: () => void
  isDiscovering: boolean
  targetsError?: boolean
  reviewsError?: boolean
  onRetryTargets?: () => void
}) {
  if (targetsError) {
    return (
      <section className="card route-card">
        <div className="card-pad">
          <div className="eyebrow">Revisión guiada</div>
          <h3 style={{ marginTop: 7 }}>No pudimos cargar las rutas</h3>
          <p className="helper" style={{ marginTop: 6 }}>El checklist sigue disponible. Reintenta la consulta o vuelve a buscar rutas.</p>
          <div className="inline" style={{ marginTop: 14 }}>
            <button className="button ghost small" type="button" onClick={onRetryTargets}>Reintentar</button>
            <RouteDiscoveryAction compact variant="primary" />
          </div>
        </div>
      </section>
    )
  }
  if (!targetsData || targetsData.mapReady === false) {
    return (
      <section className="card route-card">
        <div className="card-pad">
          <div className="eyebrow">Paso 2 · Buscar rutas</div>
          <h3 style={{ marginTop: 7 }}>Rutas aún no disponibles</h3>
          <p className="helper" style={{ marginTop: 6 }}>
            {targetsData?.discovery?.message || 'Busca las rutas del curso antes de revisar o capturar evidencias.'}
          </p>
          <div className="inline" style={{ marginTop: 14 }}>
            <RouteDiscoveryAction compact variant="primary" />
            <button className="button ghost small" type="button" onClick={onDiscover} disabled={isDiscovering}>Actualizar estado</button>
          </div>
        </div>
      </section>
    )
  }

  const targets = targetsData.targets || []
  const groups = new Map<string, RouteGroup>()
  targets.forEach((target) => {
    const key = target.routeKey || computeRouteKey(target)
    let group = groups.get(key)
    if (!group) {
      group = { key, target, itemCodes: [], names: [], slots: 0 }
      groups.set(key, group)
    }
    group.slots += 1
    const coveredCodes = target.coveredItemCodes?.length ? target.coveredItemCodes : [target.itemCode]
    coveredCodes.forEach((itemCode) => {
      if (!group.itemCodes.includes(itemCode)) group.itemCodes.push(itemCode)
    })
    if (target.name && !group.names.includes(target.name)) group.names.push(target.name)
  })
  const allEntries = [...groups.values()]
  const query = routeQuery.trim().toLowerCase()
  const entries = allEntries.filter((group) => {
    const decision = getRouteStatus(reviews, group.key)
    const haystack = [group.target.groupName, group.target.itemCode, group.target.activityTitle, ...group.itemCodes, ...group.names]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
    return (routeFilter === 'all' || decision === routeFilter) && (!query || haystack.includes(query))
  })

  return (
    <section className="card route-card">
      <div className="card-pad">
        <div className="confidence-intro">
          <div>
            <div className="eyebrow">Revisión guiada</div>
            <h3 style={{ marginTop: 7 }}>Rutas detectadas</h3>
            <p className="helper" style={{ marginTop: 6 }}>
              Confirma las secciones que usarás como evidencia. Se agrupan las rutas que comparten una misma captura.
            </p>
            {targetsData.summary ? (
              <p className="helper" style={{ marginTop: 6 }}>
                {targetsData.summary.captureUnitCount ?? allEntries.length} unidades físicas · {targetsData.summary.coverageCount ?? targets.length} criterios cubiertos
              </p>
            ) : null}
            {reviewsError ? <p className="helper" role="alert" style={{ color: 'var(--no)', marginTop: 7 }}>No pudimos cargar tus revisiones guardadas. Puedes consultar las rutas, pero espera antes de guardar cambios.</p> : null}
          </div>
          <span className="badge">
            {entries.length} visibles · {allEntries.length} rutas
          </span>
        </div>
        <div className="gallery-controls">
          <input
            id="route-review-search"
            type="search"
            value={routeQuery}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="Buscar por sección o actividad"
            aria-label="Buscar rutas"
          />
          <select
            id="route-review-filter"
            aria-label="Filtrar rutas"
            value={routeFilter}
            onChange={(event) => onFilterChange(event.target.value as 'all' | RouteReviewStatus)}
          >
            <option value="all">Todas las rutas</option>
            <option value="review">Por revisar</option>
            <option value="confirmed">Confirmadas</option>
            <option value="correction">Para corregir</option>
          </select>
        </div>
        <div className="route-list">
          {entries.length ? (
            entries.map((group, index) => (
              <RouteEntry
                key={group.key}
                group={group}
                index={index}
                review={reviews.find((review) => review.routeKey === group.key)}
                manualEdit={manualEdits[group.key]}
                onManualChange={onManualChange}
                onSaveRoute={onSaveRoute}
              />
            ))
          ) : (
            <div className="route-empty">No encontramos rutas con esos filtros.</div>
          )}
        </div>
        <div className="route-footer">
          <span className="helper">Las confirmaciones y ajustes quedan guardados en este equipo.</span>
          <button className="button ghost small" onClick={onDiscover} disabled={isDiscovering}>
            Actualizar rutas
          </button>
        </div>
      </div>
    </section>
  )
}

function PreviewModal({
  evidence,
  onClose,
  onDelete,
  isDeleting,
}: {
  evidence: Evidence
  onClose: () => void
  onDelete: () => void
  isDeleting: boolean
}) {
  const format = String(evidence.format || '').toLowerCase()
  const src = evidenceDownloadUrl(evidence.id)
  const closeRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    closeRef.current?.focus()
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
      if (event.key !== 'Tab') return
      const dialog = closeRef.current?.closest('[role="dialog"]')
      const focusable = Array.from(dialog?.querySelectorAll<HTMLElement>('button, a[href], iframe, input, select, textarea, [tabindex]:not([tabindex="-1"])') || []).filter((element) => !element.hasAttribute('disabled'))
      if (!focusable.length) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      previous?.focus?.()
    }
  }, [onClose])
  return (
    <div
      id="evidence-modal"
      className="evidence-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="evidence-preview-title"
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <div className="evidence-dialog">
        <div className="evidence-dialog-head">
          <h3 id="evidence-preview-title">{evidence.name || 'Vista previa de evidencia'}</h3>
          <button ref={closeRef} className="button ghost small" type="button" onClick={onClose}>
            Cerrar
          </button>
        </div>
        <div className="evidence-dialog-body">
          {format === 'pdf' ? (
            <iframe src={src} title={`Vista previa de ${evidence.name}`} />
          ) : format === 'html' ? (
            <iframe sandbox="allow-same-origin" src={src} title={`Vista previa de ${evidence.name}`} />
          ) : (
            <img src={src} alt={evidence.name} />
          )}
        </div>
        <div className="evidence-dialog-foot">
          <span className="helper">
            {evidence.source || 'Evidencia local'} · {formatDate(evidence.capturedAt)}
          </span>
          <a className="button secondary small" href={src} target="_blank" rel="noreferrer">
            Descargar
          </a>
          <button className="button ghost small" type="button" onClick={onDelete} disabled={isDeleting}>
            Eliminar
          </button>
        </div>
      </div>
    </div>
  )
}

export function Checklist() {
  const toast = useToast()
  const [searchParams, setSearchParams] = useSearchParams()
  const dashboardQuery = useDashboard()
  const dashboard = dashboardQuery.data

  const activitiesQuery = useActivities(dashboard?.activeFichaId)
  const targetsQuery = useTargets(dashboard?.activeFichaId)
  const reviewsQuery = useReviews(dashboard?.activeFichaId)
  const setupQuery = useSetupStatus()

  const setItemStatus = useSetItemStatus()
  const saveReview = useSaveReview()
  const generateReport = useGenerateReport()
  const discoverCourseMaps = useDiscoverCourseMaps()
  const deleteEvidence = useDeleteEvidence()

  const [category, setCategory] = useState(() => searchParams.get('category') || 'all')
  const [routeReviewOpen, setRouteReviewOpen] = useState(false)
  const [routeFilter, setRouteFilter] = useState<'all' | RouteReviewStatus>('all')
  const [routeQuery, setRouteQuery] = useState('')
  const [preview, setPreview] = useState<Evidence | null>(null)
  const [manualEdits, setManualEdits] = useState<Record<string, { manualUrl?: string; manualSelector?: string }>>({})
  const itemsSectionRef = useRef<HTMLElement>(null)

  useEffect(() => {
    const fromUrl = searchParams.get('category') || 'all'
    setCategory((current) => (current === fromUrl ? current : fromUrl))
  }, [searchParams])

  if (dashboardQuery.isLoading) {
    return <PageSkeleton label="Cargando checklist" />
  }
  if (dashboardQuery.isError && isNotFound(dashboardQuery.error)) {
    return <MissingActiveFicha message="Selecciona una ficha activa para ver el checklist." />
  }
  if (dashboardQuery.isError) {
    return <PageError message="No pudimos cargar el checklist de la ficha activa." action={<button className="button" onClick={() => dashboardQuery.refetch()}>Reintentar</button>} />
  }
  if (!dashboard) {
    return <MissingActiveFicha />
  }

  const items = dashboard.items || []
  const categories: DashboardCategory[] = dashboard.categories || []
  const summary = dashboard.summary || { yes: 0, no: 0, pending: 0, percentage: 0 }
  const done = Number(summary.yes) || 0
  const failed = Number(summary.no) || 0
  const pending = Number(summary.pending) || 0
  const total = Math.max(Number(summary.total) || items.length, 0)
  const progressTotal = Math.max(total, 1)
  const progress = Math.max(0, Math.min(100, Number(summary.percentage) || 0))
  const reviews = reviewsQuery.data || []

  const filtered = items.filter((item) => {
    if (category === 'all') return true
    if (category === 'pending') return item.status === 'PENDIENTE'
    if (category === 'no') return item.status === 'NO'
    if (category === 'empty') return !item.evidenceCount
    return item.categoryCode === category
  })

  let lastCategoryCode: string | null = null
  const taskGroups = filtered.map((item) => {
    const code = item.categoryCode || ''
    let divider: { label: string; done: number; count: number } | null = null
    if (code !== lastCategoryCode) {
      lastCategoryCode = code
      const meta = categories.find((entry) => entry.code === code)
      const catItems = filtered.filter((other) => other.categoryCode === code)
      const catDone = catItems.filter((other) => other.status === 'SI').length
      divider = { label: meta?.label || `Categoría ${code}`, done: catDone, count: catItems.length }
    }
    return { item, divider }
  })

  function handleSetStatus(itemCode: string, status: ItemStatus) {
    if (!dashboard) return
    setItemStatus.mutate(
      { itemCode, fichaId: dashboard.activeFichaId, status },
      { onError: (error) => toast(friendlyError(error.message), true) },
    )
  }

  function handleCategoryChange(nextCategory: string) {
    setCategory(nextCategory)
    setSearchParams((current) => {
      const next = new URLSearchParams(current)
      if (nextCategory === 'all') next.delete('category')
      else next.set('category', nextCategory)
      return next
    }, { replace: true })
    window.requestAnimationFrame(() => {
      const section = itemsSectionRef.current
      if (!section) return
      const top = section.getBoundingClientRect().top + window.scrollY - 24
      window.scrollTo({ top, behavior: 'smooth' })
    })
  }

  function handleExportReport() {
    if (!dashboard) return
    generateReport.mutate(
      { title: dashboard.ficha.name || 'Checklist', fichaId: dashboard.activeFichaId, format: 'pdf', evidenceLimit: 100 },
      {
        onSuccess: () => toast('Estamos generando tu reporte.'),
        onError: (error) => toast(friendlyError(error.message), true),
      },
    )
  }

  function handleDiscover() {
    discoverCourseMaps.mutate(
      {
        username: setupQuery.data?.zajunaUsername || '',
        documentType: setupQuery.data?.zajunaDocumentType || 'CC',
      },
      {
        onSuccess: () => toast('Estamos actualizando el mapa del curso.'),
        onError: (error) => toast(friendlyError(error.message), true),
      },
    )
  }

  function handleManualChange(key: string, field: 'manualUrl' | 'manualSelector', value: string) {
    setManualEdits((prev) => ({ ...prev, [key]: { ...prev[key], [field]: value } }))
  }

  function handleSaveRoute(routeKeyValue: string, status: RouteReviewStatus) {
    if (!dashboard) return
    const existingReview = reviews.find((review) => review.routeKey === routeKeyValue)
    const manualUrl = manualEdits[routeKeyValue]?.manualUrl ?? existingReview?.manualUrl ?? ''
    const manualSelector = manualEdits[routeKeyValue]?.manualSelector ?? existingReview?.manualSelector ?? ''
    saveReview.mutate(
      { fichaId: dashboard.activeFichaId, routeKey: routeKeyValue, status, manualUrl, manualSelector },
      {
        onSuccess: () =>
          toast(
            status === 'confirmed'
              ? 'Ruta confirmada.'
              : status === 'correction'
                ? 'Ruta marcada para corregir.'
                : 'Ajuste guardado.',
          ),
        onError: (error) => toast(friendlyError(error.message), true),
      },
    )
  }

  function handleDeletePreview() {
    if (!preview) return
    if (!window.confirm('¿Eliminar esta evidencia? Esta acción no se puede deshacer.')) return
    deleteEvidence.mutate(preview.id, {
      onSuccess: () => {
        toast('Evidencia eliminada.')
        setPreview(null)
      },
      onError: (error) => toast(friendlyError(error.message), true),
    })
  }

  const toggleRouteReview = () => setRouteReviewOpen((open) => !open)
  const emptyEvidenceCount = items.filter((item) => !item.evidenceCount).length

  return (
    <>
      <div className="checklist-layout">
        <aside className="card checklist-category-nav">
          <h3>Categorías · {total} ítems</h3>
          <div className="checklist-category-list">
            <button
              type="button"
              aria-pressed={category === 'all'}
              className={`checklist-category-item ${category === 'all' ? 'active' : ''}`}
              onClick={() => handleCategoryChange('all')}
            >
              <strong>Todas</strong>
              <small>{total}</small>
            </button>
            {categories.map((entry) => (
              <button
                key={entry.code}
                type="button"
                aria-pressed={category === entry.code}
                className={`checklist-category-item ${category === entry.code ? 'active' : ''}`}
                onClick={() => handleCategoryChange(entry.code)}
              >
                <strong>{entry.label || `Categoría ${entry.code}`}</strong>
                <small>
                  {Number(entry.yes) || 0}/{Number(entry.total) || 0}
                </small>
              </button>
            ))}
          </div>
          <div className="route-note">El estado y las evidencias de cada punto se guardan en este equipo.</div>
        </aside>

        <div className="checklist-main">
          <section className="checklist-header">
            <div className="checklist-heading">
              <div>
                <div className="eyebrow">
                  Ficha activa{' '}
                  <span className="badge" style={{ marginLeft: 8 }}>
                    {dashboard.ficha.phase || 'Fase de Ejecución'}
                  </span>
                </div>
                <h2>{dashboard.ficha.name || 'Checklist de la ficha'}</h2>
                <p className="helper" style={{ marginTop: 7 }}>
                  Ficha {dashboard.ficha.externalId || '—'} · curso {dashboard.ficha.courseId || '—'}
                </p>
              </div>
              <div className="checklist-actions">
                <RouteDiscoveryAction compact variant="primary" />
                <button className="button ghost small" onClick={toggleRouteReview}>
                  {routeReviewOpen ? 'Ocultar mapa' : 'Ver mapa'}
                </button>
                <button className="button ghost small" onClick={handleExportReport} disabled={generateReport.isPending}>
                  Generar PDF
                </button>
                <CaptureAction compact />
              </div>
            </div>
            <div className="checklist-progress">
              <i>
                <span style={{ width: `${(done / progressTotal) * 100}%`, background: 'var(--brand)' }} />
                <span style={{ width: `${(failed / progressTotal) * 100}%`, background: 'var(--no)' }} />
                <span style={{ width: `${(pending / progressTotal) * 100}%`, background: '#f0c77e' }} />
              </i>
              <div className="checklist-progress-copy">
                <strong>{progress} %</strong>
                <span>
                  <i style={{ display: 'inline-block', width: 8, height: 8, borderRadius: 3, background: 'var(--yes)', marginRight: 5 }} />
                  {done}
                </span>
                <span>
                  <i style={{ display: 'inline-block', width: 8, height: 8, borderRadius: 3, background: 'var(--no)', marginRight: 5 }} />
                  {failed}
                </span>
                <span>
                  <i style={{ display: 'inline-block', width: 8, height: 8, borderRadius: 3, background: '#f0c77e', marginRight: 5 }} />
                  {pending}
                </span>
              </div>
            </div>
          </section>

          <div className="checklist-filter-bar">
            <div className="checklist-filter-tabs">
              <button type="button" aria-pressed={category === 'all'} className={`checklist-filter-tab ${category === 'all' ? 'active' : ''}`} onClick={() => handleCategoryChange('all')}>
                Todas {total}
              </button>
              <button type="button" aria-pressed={category === 'pending'} className={`checklist-filter-tab ${category === 'pending' ? 'active' : ''}`} onClick={() => handleCategoryChange('pending')}>
                Pendientes {pending}
              </button>
              <button type="button" aria-pressed={category === 'no'} className={`checklist-filter-tab ${category === 'no' ? 'active' : ''}`} onClick={() => handleCategoryChange('no')}>
                No cumplidas {failed}
              </button>
              <button type="button" aria-pressed={category === 'empty'} className={`checklist-filter-tab ${category === 'empty' ? 'active' : ''}`} onClick={() => handleCategoryChange('empty')}>
                Sin evidencia {emptyEvidenceCount}
              </button>
            </div>
            <span className="helper">
              Mostrando {filtered.length} de {items.length} ítems
            </span>
          </div>

          {activitiesQuery.isLoading ? (
            <section className="card"><div className="card-pad"><p className="helper" role="status">Cargando actividades de la ficha…</p></div></section>
          ) : activitiesQuery.isError ? (
            <section className="card"><div className="card-pad"><p className="helper" role="alert">No pudimos cargar las actividades de esta ficha.</p><button className="button ghost small" type="button" onClick={() => activitiesQuery.refetch()} style={{ marginTop: 12 }}>Reintentar</button></div></section>
          ) : (
            activitiesQuery.data?.mapReady === false ? (
              <section className="card">
                <div className="card-pad">
                  <div className="eyebrow">Paso 2 · Buscar rutas</div>
                  <h3 style={{ marginTop: 7 }}>Todavía no hay actividades del curso</h3>
                  <p className="helper" style={{ marginTop: 6 }}>
                    {activitiesQuery.data.discovery?.message || 'Busca las rutas del curso para cargar las actividades y elegir cuáles estarán a tu cargo.'}
                  </p>
                  <div className="inline" style={{ marginTop: 14 }}>
                    <RouteDiscoveryAction compact variant="primary" />
                    <span className="helper">Después vuelve aquí para seleccionar actividades.</span>
                  </div>
                </div>
              </section>
            ) : activitiesQuery.data ? (
              <ActivitySelector key={dashboard.activeFichaId} data={activitiesQuery.data} fichaId={dashboard.activeFichaId} />
            ) : null
          )}

          <ConfidenceSummarySection items={filtered} routeReviewOpen={routeReviewOpen} onToggleRoutes={toggleRouteReview} />

          {routeReviewOpen && (
            <RouteReviewSection
              targetsData={targetsQuery.data}
              reviews={reviews}
              routeFilter={routeFilter}
              routeQuery={routeQuery}
              onFilterChange={setRouteFilter}
              onQueryChange={setRouteQuery}
              manualEdits={manualEdits}
              onManualChange={handleManualChange}
              onSaveRoute={handleSaveRoute}
              onDiscover={handleDiscover}
              isDiscovering={discoverCourseMaps.isPending}
              targetsError={targetsQuery.isError}
              reviewsError={reviewsQuery.isError}
              onRetryTargets={() => targetsQuery.refetch()}
            />
          )}

          <section ref={itemsSectionRef} className="card checklist-items-card">
            <div className="checklist-items-head">
              <div>
                <h3>Ítems del checklist</h3>
                <p className="helper" style={{ marginTop: 5 }}>
                  Cambia el estado después de revisar la información y abre las evidencias relacionadas.
                </p>
              </div>
              <span className="badge">{filtered.length} visibles</span>
            </div>
            <div className="task-list">
              {filtered.length ? (
                taskGroups.map(({ item, divider }) => (
                  <Fragment key={item.itemCode}>
                    {divider && (
                      <div className="checklist-cat-divider">
                        <strong>{divider.label}</strong>
                        <span>
                          {divider.done} de {divider.count} cumplidos
                        </span>
                      </div>
                    )}
                    <TaskRow item={item} onPreview={(evidence) => setPreview(evidence)} onSetStatus={handleSetStatus} />
                  </Fragment>
                ))
              ) : (
                <div className="empty">No hay ítems con este filtro.</div>
              )}
            </div>
          </section>
        </div>
      </div>

      {preview && (
        <PreviewModal
          evidence={preview}
          onClose={() => setPreview(null)}
          onDelete={handleDeletePreview}
          isDeleting={deleteEvidence.isPending}
        />
      )}
    </>
  )
}
