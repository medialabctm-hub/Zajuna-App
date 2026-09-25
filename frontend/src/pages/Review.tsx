import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { evidenceDownloadUrl } from '../api/client'
import { MissingActiveFicha, PageError, PageSkeleton } from '../components/AsyncState'
import {
  isNotFound,
  useCapture,
  useDashboard,
  useEvidenceReview,
  useSetEvidenceReview,
  useSetItemStatus,
  useSetupStatus,
  useVerifyEvidences,
} from '../hooks/api'
import { useToast } from '../hooks/useToast'
import { formatDate } from '../lib/format'
import { friendlyError } from '../lib/friendlyError'
import { approvedItemsNotMarked } from '../lib/workflow'
import type {
  EvidenceReviewEntry,
  EvidenceReviewMissingItem,
  EvidenceReviewStatus,
  EvidenceReviewSummary,
} from '../types'

export type ReviewTab = 'pending' | 'approved' | 'rejected' | 'all'

export interface ReviewItemGroup {
  itemCode: string
  itemDescription: string
  entries: EvidenceReviewEntry[]
}

const TABS: { key: ReviewTab; label: string }[] = [
  { key: 'pending', label: 'Pendientes' },
  { key: 'approved', label: 'Aprobadas' },
  { key: 'rejected', label: 'Rechazadas' },
  { key: 'all', label: 'Todas' },
]

const STATUS_LABEL: Record<EvidenceReviewStatus, string> = {
  approved: 'Aprobada',
  pending: 'Pendiente',
  rejected: 'Rechazada',
}

const STATUS_CHIP: Record<EvidenceReviewStatus, string> = {
  approved: 'ok',
  pending: 'review',
  rejected: 'error',
}

const EMPTY_ENTRIES: EvidenceReviewEntry[] = []
const EMPTY_MISSING: EvidenceReviewMissingItem[] = []

function normalizeText(value: string) {
  return value.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().trim()
}

function compareItemCodes(left: string, right: string) {
  return left.localeCompare(right, 'es', { numeric: true })
}

/** Filters review entries by tab (status) and a free-text search over ítem code, description and name. */
// oxlint-disable-next-line react/only-export-components
export function filterReviewEntries(entries: EvidenceReviewEntry[], tab: ReviewTab, query: string): EvidenceReviewEntry[] {
  const needle = normalizeText(query)
  return entries.filter((entry) => {
    if (tab !== 'all' && entry.status !== tab) return false
    if (!needle) return true
    const haystack = normalizeText([entry.itemCode, entry.itemDescription, entry.name].filter(Boolean).join(' '))
    return haystack.includes(needle)
  })
}

/** Filters missing ítems by the same free-text search. */
// oxlint-disable-next-line react/only-export-components
export function filterMissingItems(items: EvidenceReviewMissingItem[], query: string): EvidenceReviewMissingItem[] {
  const needle = normalizeText(query)
  if (!needle) return items
  return items.filter((item) => normalizeText(`${item.itemCode} ${item.description || ''}`).includes(needle))
}

/** Groups entries by ítem code (natural order) and orders each group by slot number. */
// oxlint-disable-next-line react/only-export-components
export function groupReviewByItem(entries: EvidenceReviewEntry[]): ReviewItemGroup[] {
  const groups = new Map<string, ReviewItemGroup>()
  for (const entry of entries) {
    const code = String(entry.itemCode || '').trim() || 'Sin ítem'
    let group = groups.get(code)
    if (!group) {
      group = { itemCode: code, itemDescription: entry.itemDescription || '', entries: [] }
      groups.set(code, group)
    }
    if (!group.itemDescription && entry.itemDescription) group.itemDescription = entry.itemDescription
    group.entries.push(entry)
  }
  const result = [...groups.values()].sort((a, b) => compareItemCodes(a.itemCode, b.itemCode))
  result.forEach((group) => group.entries.sort((a, b) => (a.slotNumber || 0) - (b.slotNumber || 0)))
  return result
}

/** Percentage (0–100) of checklist ítems whose evidences are all approved. */
// oxlint-disable-next-line react/only-export-components
export function approvedItemsPercent(summary?: Pick<EvidenceReviewSummary, 'itemsApproved' | 'itemsWithEvidence' | 'itemsMissing'>): number {
  if (!summary) return 0
  const total = (Number(summary.itemsWithEvidence) || 0) + (Number(summary.itemsMissing) || 0)
  if (total <= 0) return 0
  return Math.max(0, Math.min(100, Math.round(((Number(summary.itemsApproved) || 0) / total) * 100)))
}

/** Counts entries per tab so the filter buttons can show them. */
// oxlint-disable-next-line react/only-export-components
export function countByTab(entries: EvidenceReviewEntry[]): Record<ReviewTab, number> {
  const counts: Record<ReviewTab, number> = { pending: 0, approved: 0, rejected: 0, all: entries.length }
  for (const entry of entries) {
    if (entry.status in counts) counts[entry.status]++
  }
  return counts
}

function evidenceLabel(entry: EvidenceReviewEntry) {
  const slot = entry.slotNumber ? `Evidencia ${entry.slotNumber}` : 'Evidencia'
  return `${slot} del ítem ${entry.itemCode}`
}

function Thumbnail({ entry, onOpen, compact = false }: { entry: EvidenceReviewEntry; onOpen: () => void; compact?: boolean }) {
  const [failed, setFailed] = useState(false)
  return (
    <button
      type="button"
      className={`review-thumb${compact ? ' compact' : ''}`}
      onClick={onOpen}
      aria-label={`Ver en grande: ${evidenceLabel(entry)}`}
    >
      {failed ? (
        <span className="review-thumb-missing">No se pudo mostrar la imagen</span>
      ) : (
        <img src={evidenceDownloadUrl(entry.evidenceId)} alt="" loading="lazy" onError={() => setFailed(true)} />
      )}
    </button>
  )
}

function ReviewLightbox({ entry, onClose }: { entry: EvidenceReviewEntry; onClose: () => void }) {
  const closeRef = useRef<HTMLButtonElement>(null)
  const src = evidenceDownloadUrl(entry.evidenceId)

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    closeRef.current?.focus()
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
      if (event.key !== 'Tab') return
      const dialog = closeRef.current?.closest('[role="dialog"]')
      const focusable = Array.from(dialog?.querySelectorAll<HTMLElement>('button, a[href], input, select, textarea, [tabindex]:not([tabindex="-1"])') || []).filter((element) => !element.hasAttribute('disabled'))
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
      className="evidence-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="review-lightbox-title"
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <div className="evidence-dialog review-lightbox">
        <div className="evidence-dialog-head">
          <h3 id="review-lightbox-title">
            {evidenceLabel(entry)}
            {entry.itemDescription ? <span className="helper review-lightbox-desc">{entry.itemDescription}</span> : null}
          </h3>
          <button ref={closeRef} className="button ghost small" type="button" onClick={onClose}>
            Cerrar
          </button>
        </div>
        <div className="evidence-dialog-body review-lightbox-body">
          <img src={src} alt={evidenceLabel(entry)} />
        </div>
        <div className="evidence-dialog-foot">
          <span className="helper">
            {STATUS_LABEL[entry.status] || 'Pendiente'}
            {entry.width && entry.height ? ` · ${entry.width} × ${entry.height} px` : ''}
          </span>
          <a className="button secondary small" href={src} target="_blank" rel="noreferrer">
            Abrir en otra pestaña
          </a>
        </div>
      </div>
    </div>
  )
}

export function Review() {
  const dashboardQuery = useDashboard()
  const fichaId = dashboardQuery.data?.activeFichaId
  const setupQuery = useSetupStatus()
  const reviewQuery = useEvidenceReview(fichaId)
  const verify = useVerifyEvidences()
  const setReview = useSetEvidenceReview()
  const capture = useCapture()
  const toast = useToast()

  const [tab, setTab] = useState<ReviewTab>('pending')
  const [query, setQuery] = useState('')
  const [preview, setPreview] = useState<EvidenceReviewEntry | null>(null)
  const [markingDone, setMarkingDone] = useState(false)
  const setItemStatus = useSetItemStatus()
  const approvedNotMarked = useMemo(
    () => approvedItemsNotMarked(reviewQuery.data?.evidences ?? [], dashboardQuery.data?.items ?? []),
    [reviewQuery.data, dashboardQuery.data],
  )
  const closePreview = useCallback(() => setPreview(null), [])

  const review = reviewQuery.data
  const entries = review?.evidences ?? EMPTY_ENTRIES
  const missingItems = review?.missingItems ?? EMPTY_MISSING
  const counts = useMemo(() => countByTab(entries), [entries])
  const filtered = useMemo(() => filterReviewEntries(entries, tab, query), [entries, tab, query])
  const groups = useMemo(() => groupReviewByItem(filtered), [filtered])
  const visibleMissing = useMemo(() => filterMissingItems(missingItems, query), [missingItems, query])

  if (dashboardQuery.isLoading) return <PageSkeleton label="Cargando la revisión de evidencias" />
  if (dashboardQuery.isError && isNotFound(dashboardQuery.error)) {
    return <MissingActiveFicha message="Selecciona una ficha activa para revisar sus evidencias." />
  }
  if (dashboardQuery.isError || !fichaId) {
    return <PageError message="No pudimos cargar la ficha activa." action={<button className="button" type="button" onClick={() => dashboardQuery.refetch()}>Reintentar</button>} />
  }
  if (reviewQuery.isLoading) return <PageSkeleton label="Revisando tus evidencias" />
  if (reviewQuery.isError || !review) {
    return (
      <PageError
        message="No pudimos cargar la revisión de evidencias de esta ficha."
        action={<button className="button" type="button" onClick={() => reviewQuery.refetch()}>Reintentar</button>}
      />
    )
  }

  const summary = review.summary
  const percent = approvedItemsPercent(summary)
  const totalItems = (summary.itemsWithEvidence || 0) + (summary.itemsMissing || 0)
  const needsWork = (summary.pending || 0) + (summary.rejected || 0) + missingItems.length
  const capturingItem = capture.isPending ? capture.variables?.itemCodes?.[0] : undefined
  const savingId = setReview.isPending ? setReview.variables?.evidenceId : undefined

  const handleVerify = () => {
    verify.mutate(fichaId, {
      onSuccess: (data) => {
        const pending = (data.summary?.pending || 0) + (data.summary?.rejected || 0)
        toast(pending ? `Verificación lista: ${pending} evidencia${pending === 1 ? '' : 's'} necesita${pending === 1 ? '' : 'n'} tu revisión.` : 'Verificación lista: todas las evidencias están aprobadas.')
      },
      onError: (error) => toast(friendlyError(error.message), true),
    })
  }

  const handleDecision = (entry: EvidenceReviewEntry, status: EvidenceReviewStatus) => {
    const messages: Record<EvidenceReviewStatus, string> = {
      approved: `Aprobaste la ${evidenceLabel(entry).toLowerCase()}.`,
      rejected: `Rechazaste la ${evidenceLabel(entry).toLowerCase()}.`,
      pending: `La ${evidenceLabel(entry).toLowerCase()} vuelve a la revisión automática.`,
    }
    // Una misma imagen puede respaldar varios ítems: la decisión se aplica a
    // todas sus copias para no tener que aprobar la misma imagen N veces.
    const sameImage = entry.sha256 ? entries.filter((other) => other.sha256 === entry.sha256) : [entry]
    // En secuencia: evita decenas de peticiones simultáneas y un error parcial
    // informa cuántas quedaron guardadas.
    ;(async () => {
      let saved = 0
      try {
        for (const target of sameImage) {
          await setReview.mutateAsync({ evidenceId: target.evidenceId, status, note: '' })
          saved++
        }
        toast(sameImage.length > 1 ? `${messages[status]} Se aplicó a los ${sameImage.length} ítems que usan esta imagen.` : messages[status])
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        toast(`${friendlyError(message)}${saved ? ` (se guardaron ${saved} de ${sameImage.length})` : ''}`, true)
      }
    })()
  }

  // Un ítem con todas sus evidencias aprobadas puede marcarse como cumplido
  // en el checklist: así el % de cumplimiento refleja la revisión.
  const handleMarkApprovedDone = async () => {
    setMarkingDone(true)
    let marked = 0
    try {
      for (const itemCode of approvedNotMarked) {
        await setItemStatus.mutateAsync({ itemCode, fichaId, status: 'SI' })
        marked++
      }
      toast(`Marcamos ${marked} ítems como cumplidos en el checklist.`)
    } catch (error) {
      toast(`${friendlyError(error instanceof Error ? error.message : String(error))} (se marcaron ${marked})`, true)
    } finally {
      setMarkingDone(false)
    }
  }

  const handleRecapture = (itemCode: string) => {
    capture.mutate(
      {
        fichaId,
        username: setupQuery.data?.zajunaUsername || '',
        documentType: setupQuery.data?.zajunaDocumentType || 'CC',
        itemCodes: [itemCode],
      },
      {
        onSuccess: () => toast(`Estamos volviendo a capturar el ítem ${itemCode}. Al terminar, esta revisión se actualiza sola.`),
        onError: (error) => toast(friendlyError(error.message), true),
      },
    )
  }

  const recaptureButton = (itemCode: string, label = 'Volver a capturar este ítem') => (
    <button
      type="button"
      className="button secondary small"
      onClick={() => handleRecapture(itemCode)}
      disabled={capture.isPending}
      aria-label={`${label}: ${itemCode}`}
    >
      {capturingItem === itemCode ? 'Enviando…' : label}
    </button>
  )

  const entryActions = (entry: EvidenceReviewEntry) => {
    const busy = savingId === entry.evidenceId
    const label = evidenceLabel(entry)
    return (
      <div className="review-actions">
        {entry.status !== 'approved' && (
          <button type="button" className="button small" disabled={busy} onClick={() => handleDecision(entry, 'approved')} aria-label={`Aprobar ${label}`}>
            Aprobar
          </button>
        )}
        {entry.status !== 'rejected' && (
          <button type="button" className="button ghost small review-reject" disabled={busy} onClick={() => handleDecision(entry, 'rejected')} aria-label={`Rechazar ${label}`}>
            Rechazar
          </button>
        )}
        {entry.status !== 'pending' && entry.source === 'manual' && (
          <button type="button" className="button ghost small" disabled={busy} onClick={() => handleDecision(entry, 'pending')} aria-label={`Marcar como pendiente ${label}`}>
            Marcar como pendiente
          </button>
        )}
        <button type="button" className="button ghost small" onClick={() => setPreview(entry)} aria-label={`Ver en grande ${label}`}>
          Ver en grande
        </button>
      </div>
    )
  }

  const renderEntry = (entry: EvidenceReviewEntry) => (
    <li key={entry.evidenceId} className={`review-entry ${entry.status}`}>
      <Thumbnail entry={entry} onOpen={() => setPreview(entry)} />
      <div className="review-entry-body">
        <div className="review-entry-head">
          <strong>{entry.slotNumber ? `Evidencia ${entry.slotNumber}` : 'Evidencia'}</strong>
          <span className={`status-chip ${STATUS_CHIP[entry.status] || 'review'}`}>{STATUS_LABEL[entry.status] || 'Pendiente'}</span>
          {entry.source === 'manual' && <span className="badge muted">Decisión tuya</span>}
        </div>
        {entry.reasons?.length ? (
          <ul className="review-reasons" aria-label="Problemas encontrados">
            {entry.reasons.map((reason) => (
              <li key={`${reason.code}-${reason.message}`}>{reason.message}</li>
            ))}
          </ul>
        ) : (
          <p className="helper">
            {entry.status === 'approved'
              ? 'No encontramos problemas en esta evidencia.'
              : entry.status === 'rejected'
                ? 'La marcaste como rechazada. Vuelve a capturar el ítem o apruébala si es correcta.'
                : 'Revisa la imagen y decide si es correcta.'}
          </p>
        )}
        {entry.sharedWith?.length ? (
          <p className="helper">También se usa en: {entry.sharedWith.join(', ')}</p>
        ) : null}
        {entryActions(entry)}
      </div>
    </li>
  )

  const renderGroups = () => (
    <ol className="review-groups">
      {groups.map((group) => (
        <li key={group.itemCode} className="card review-group">
          <div className="review-group-head">
            <div>
              <span className="task-code">Ítem {group.itemCode}</span>
              {group.itemDescription && <p className="review-group-desc">{group.itemDescription}</p>}
            </div>
            {recaptureButton(group.itemCode)}
          </div>
          <ul className="review-entries">{group.entries.map(renderEntry)}</ul>
        </li>
      ))}
    </ol>
  )

  const renderApprovedGrid = () => (
    <ul className="review-approved-grid">
      {filtered.map((entry) => (
        <li key={entry.evidenceId} className="review-approved-card">
          <Thumbnail entry={entry} compact onOpen={() => setPreview(entry)} />
          <div className="review-approved-copy">
            <strong>Ítem {entry.itemCode}{entry.slotNumber ? ` · ${entry.slotNumber}` : ''}</strong>
            <span className="helper">{entry.source === 'manual' ? 'Aprobada por ti' : 'Aprobada automáticamente'}</span>
          </div>
          {entry.source === 'manual' ? (
            <button
              type="button"
              className="button ghost small"
              disabled={savingId === entry.evidenceId}
              onClick={() => handleDecision(entry, 'pending')}
              aria-label={`Marcar como pendiente ${evidenceLabel(entry)}`}
            >
              Marcar como pendiente
            </button>
          ) : (
            // Una aprobada automáticamente volvería a aprobarse sola al
            // re-verificarla; lo útil es poder rechazarla si no sirve.
            <button
              type="button"
              className="button ghost small danger-outline"
              disabled={savingId === entry.evidenceId}
              onClick={() => handleDecision(entry, 'rejected')}
              aria-label={`Rechazar ${evidenceLabel(entry)}`}
            >
              Rechazar
            </button>
          )}
        </li>
      ))}
    </ul>
  )

  const renderMissing = () =>
    visibleMissing.length ? (
      <section className="card review-missing" aria-labelledby="review-missing-title">
        <div className="card-pad">
          <h2 id="review-missing-title" className="review-section-title">Ítems sin evidencia</h2>
          <p className="helper">
            Estos ítems no tienen ninguna captura. Vuelve a capturarlos; si siguen sin evidencia, revisa su ruta en el Checklist o agrega un archivo desde Evidencias.
          </p>
          <ul className="review-missing-list">
            {visibleMissing.map((item) => (
              <li key={item.itemCode}>
                <div>
                  <span className="task-code">Ítem {item.itemCode}</span>
                  {item.description && <p className="review-group-desc">{item.description}</p>}
                  <p className="helper">{item.reason || 'No se capturó evidencia para este ítem.'}</p>
                </div>
                {recaptureButton(item.itemCode, 'Volver a capturar')}
              </li>
            ))}
          </ul>
        </div>
      </section>
    ) : null

  const renderTabContent = () => {
    if (!entries.length && !missingItems.length) {
      return (
        <div className="card empty review-empty">
          <strong>Todavía no hay evidencias para revisar.</strong>
          <p>Prepara las evidencias desde el Checklist y vuelve aquí para revisarlas.</p>
          <Link className="button" to="/checklist">Ir al Checklist</Link>
        </div>
      )
    }
    if (tab === 'pending' && needsWork === 0) {
      return (
        <div className="card review-done" role="status">
          <div className="card-pad">
            <div className="eyebrow">Todo en orden</div>
            <h2>Todas las evidencias están aprobadas</h2>
            <p className="helper">No quedan evidencias pendientes. El siguiente paso es generar el reporte.</p>
            <Link className="button" to="/reportes">Generar reporte PDF</Link>
          </div>
        </div>
      )
    }
    const nothingHere = !filtered.length
    const emptyMessage = query
      ? 'No hay evidencias que coincidan con tu búsqueda.'
      : tab === 'pending'
        ? 'No hay evidencias pendientes.'
        : tab === 'rejected'
          ? 'No hay evidencias rechazadas.'
          : tab === 'approved'
            ? 'Todavía no hay evidencias aprobadas.'
            : 'No hay evidencias.'
    return (
      <>
        {nothingHere ? <div className="card empty">{emptyMessage}</div> : tab === 'approved' ? renderApprovedGrid() : renderGroups()}
        {(tab === 'pending' || tab === 'all') && renderMissing()}
      </>
    )
  }

  return (
    <div className="review-layout">
      <section className="card review-intro">
        <div className="card-pad review-intro-row">
          <div>
            <p>
              Revisamos automáticamente cada evidencia; las correctas quedan aprobadas y aquí solo te quedan las que tienen problemas.
              La revisión automática detecta problemas técnicos, así que tu decisión siempre manda.
            </p>
            <p className="helper review-verified">Última verificación: {formatDate(review.verifiedAt)}</p>
          </div>
          <button type="button" className="button secondary" onClick={handleVerify} disabled={verify.isPending}>
            {verify.isPending ? 'Verificando…' : 'Verificar de nuevo'}
          </button>
        </div>
      </section>

      <section className="review-summary" role="status" aria-live="polite" aria-label="Resumen de la revisión">
        <div className="metric-grid">
          <article className="metric-card review-metric approved">
            <span className="metric-label">Aprobadas</span>
            <div className="metric-value-row"><strong className="metric-value">{summary.approved || 0}</strong></div>
            <p className="metric-note">de {summary.total || 0} evidencias</p>
          </article>
          <article className="metric-card review-metric pending">
            <span className="metric-label">Pendientes</span>
            <div className="metric-value-row"><strong className="metric-value">{summary.pending || 0}</strong></div>
            <p className="metric-note">necesitan tu revisión</p>
          </article>
          <article className="metric-card review-metric rejected">
            <span className="metric-label">Rechazadas</span>
            <div className="metric-value-row"><strong className="metric-value">{summary.rejected || 0}</strong></div>
            <p className="metric-note">hay que volver a capturarlas</p>
          </article>
          <article className="metric-card review-metric missing">
            <span className="metric-label">Ítems sin evidencia</span>
            <div className="metric-value-row"><strong className="metric-value">{summary.itemsMissing ?? missingItems.length}</strong></div>
            <p className="metric-note">sin ninguna captura</p>
          </article>
        </div>
        <div className="card review-progress">
          <div className="review-progress-head">
            <strong>Ítems aprobados</strong>
            <span>{summary.itemsApproved || 0} de {totalItems} · {percent}%</span>
          </div>
          <div
            className="progress-track"
            role="progressbar"
            aria-label="Ítems con todas sus evidencias aprobadas"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={percent}
          >
            <i style={{ width: `${percent}%` }} />
          </div>
          {approvedNotMarked.length ? (
            <div className="review-mark-done">
              <span className="helper">
                {approvedNotMarked.length} ítem{approvedNotMarked.length === 1 ? '' : 's'} con toda su evidencia aprobada todavía no
                {approvedNotMarked.length === 1 ? ' está marcado' : ' están marcados'} como cumplido{approvedNotMarked.length === 1 ? '' : 's'} en el checklist.
              </span>
              <button type="button" className="button primary small" onClick={handleMarkApprovedDone} disabled={markingDone}>
                {markingDone ? 'Marcando…' : `Marcar ${approvedNotMarked.length} como cumplidos`}
              </button>
            </div>
          ) : null}
        </div>
      </section>

      <section className="review-toolbar" aria-label="Filtrar evidencias">
        <div className="review-tabs" role="group" aria-label="Estado de las evidencias">
          {TABS.map((item) => (
            <button
              key={item.key}
              type="button"
              className={`tab${tab === item.key ? ' active' : ''}`}
              aria-pressed={tab === item.key}
              onClick={() => setTab(item.key)}
            >
              <span className="tab-label">{item.label}</span>
              <span className="tab-count">{counts[item.key]}</span>
            </button>
          ))}
        </div>
        <label className="review-search">
          <span className="sr-only">Buscar por ítem</span>
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Buscar por ítem (ej. 7.4.2) o descripción"
          />
        </label>
      </section>

      {renderTabContent()}

      {preview && <ReviewLightbox entry={preview} onClose={closePreview} />}
    </div>
  )
}
