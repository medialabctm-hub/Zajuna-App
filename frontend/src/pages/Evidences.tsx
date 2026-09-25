import { useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react'
import { evidenceDownloadUrl, evidenceThumbnailUrl } from '../api/client'
import { MissingActiveFicha, PageError, PageSkeleton } from '../components/AsyncState'
import {
  useDashboard,
  useDeleteEvidence,
  useEvidences,
  useEvidenceGroups,
  useEvidenceReview,
  useRebuildEvidenceGroups,
  useSetItemStatus,
  useUploadEvidence,
  isNotFound,
} from '../hooks/api'
import { useToast } from '../hooks/useToast'
import { friendlyError } from '../lib/friendlyError'
import { confidenceFor, formatDate } from '../lib/format'
import { groupState, type ReviewStatusById } from '../lib/evidenceGroupState'
import type { DashboardItem, Evidence, EvidenceGroup } from '../types'

type GroupFilter = 'all' | 'review' | 'high' | 'manual'


const EMPTY_GROUPS: EvidenceGroup[] = []

/** A unique visual content (same bytes or same file) and every row that uses it. */
export interface EvidenceContent {
  key: string
  evidence: Evidence
  evidenceIds: string[]
  itemCodes: string[]
}

function evidenceContentKey(evidence: Evidence): string {
  const hash = String(evidence.sha256 || '').trim().toLowerCase()
  if (hash) return `hash:${hash}`
  const fileKey = String(evidence.fileKey || '').trim()
  if (fileKey) return `file:${fileKey}`
  return `id:${evidence.id}`
}

function compareItemCodes(left: string, right: string) {
  return left.localeCompare(right, 'es', { numeric: true })
}

/**
 * Collapses evidence rows that point to identical content (same SHA-256, or
 * same stored file when the hash is missing) so each image is shown once, while
 * keeping the list of checklist items it covers. Order of first appearance is
 * preserved.
 */
// oxlint-disable-next-line react/only-export-components
export function dedupeEvidencesByContent(evidences: Evidence[]): EvidenceContent[] {
  const byKey = new Map<string, EvidenceContent>()
  for (const evidence of evidences) {
    if (!evidence?.id) continue
    const key = evidenceContentKey(evidence)
    let entry = byKey.get(key)
    if (!entry) {
      entry = { key, evidence, evidenceIds: [], itemCodes: [] }
      byKey.set(key, entry)
    }
    if (!entry.evidenceIds.includes(evidence.id)) entry.evidenceIds.push(evidence.id)
    const code = String(evidence.itemCode || '').trim()
    if (code && !entry.itemCodes.includes(code)) entry.itemCodes.push(code)
  }
  const result = [...byKey.values()]
  result.forEach((entry) => entry.itemCodes.sort(compareItemCodes))
  return result
}

function usedInLabel(itemCodes: string[]) {
  return `Usada en ${itemCodes.length} ítem${itemCodes.length === 1 ? '' : 's'}`
}


export function Evidences() {
  const dashboardQuery = useDashboard()
  const dashboard = dashboardQuery.data
  const activeFichaId = dashboard?.activeFichaId
  const groupsQuery = useEvidenceGroups(activeFichaId)
  const evidencesQuery = useEvidences(activeFichaId)
  const reviewQuery = useEvidenceReview(activeFichaId)
  const reviewStatus = useMemo<ReviewStatusById>(
    () => new Map((reviewQuery.data?.evidences ?? []).map((entry) => [entry.evidenceId, entry.status])),
    [reviewQuery.data],
  )
  const evidenceGroups = groupsQuery.data
  const evidences = evidencesQuery.data
  const rebuildGroups = useRebuildEvidenceGroups()
  const uploadEvidence = useUploadEvidence()
  const deleteEvidence = useDeleteEvidence()
  const setItemStatus = useSetItemStatus()
  const toast = useToast()

  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<GroupFilter>('all')
  const [formatFilter, setFormatFilter] = useState('all')
  const [expanded, setExpanded] = useState(false)
  const [preview, setPreview] = useState<Evidence | null>(null)
  const [uploadItemCode, setUploadItemCode] = useState('')
  const [selectedEvidenceIds, setSelectedEvidenceIds] = useState<Set<string>>(new Set())
  const fileInputRef = useRef<HTMLInputElement>(null)

  const groups = evidenceGroups ?? EMPTY_GROUPS
  const groupedEvidences = useMemo(() => {
    const seen = new Set<string>()
    return groups.flatMap((group) => group.evidences || []).filter((evidence) => {
      if (!evidence.id || seen.has(evidence.id)) return false
      seen.add(evidence.id)
      return true
    })
  }, [groups])
  const flatEvidences = evidences?.length ? evidences : groupedEvidences
  // Byte-identical captures shared by several ítems are shown once.
  const uniqueContents = useMemo(() => dedupeEvidencesByContent(flatEvidences), [flatEvidences])

  if (dashboardQuery.isLoading) return <PageSkeleton label="Cargando galería de evidencias" />
  if (dashboardQuery.isError && isNotFound(dashboardQuery.error)) {
    return <MissingActiveFicha message="Selecciona una ficha activa para ver sus evidencias." />
  }
  if (dashboardQuery.isError || !dashboard) return <PageError message="No pudimos cargar las evidencias de la ficha activa." action={<button className="button" onClick={() => dashboardQuery.refetch()}>Reintentar</button>} />
  if (groupsQuery.isLoading || evidencesQuery.isLoading) return <PageSkeleton label="Cargando archivos de evidencia" />
  if (groupsQuery.isError || evidencesQuery.isError) {
    return (
      <PageError
        message="No pudimos cargar los archivos de evidencia de esta ficha."
        action={<button className="button" onClick={() => { groupsQuery.refetch(); evidencesQuery.refetch() }}>Reintentar</button>}
      />
    )
  }

  const dashboardItems = dashboard?.items || []
  const relatedItems = dashboardItems.filter((item) => item.evidenceCount)

  const handleRebuild = () => {
    if (!activeFichaId) return
    rebuildGroups.mutate(activeFichaId, {
      onSuccess: () => toast('Agrupación de evidencias actualizada.'),
      onError: (error) => toast(friendlyError(error.message), true),
    })
  }

  const handlePickUpload = () => {
    fileInputRef.current?.click()
  }

  const handleFileChange = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file || !activeFichaId) return
    const form = new FormData()
    form.append('file', file)
    form.append('fichaId', activeFichaId)
    if (uploadItemCode) form.append('itemCode', uploadItemCode)
    uploadEvidence.mutate(form, {
      onSuccess: (result) => toast(`Evidencia "${result.name || file.name}" añadida.`),
      onError: (error) => toast(friendlyError(error.message), true),
    })
  }

  const handleDelete = (evidenceId: string) => {
    if (!window.confirm('¿Eliminar esta evidencia? Esta acción no se puede deshacer.')) return
    deleteEvidence.mutate(evidenceId, {
      onSuccess: () => {
        toast('Evidencia eliminada.')
        setPreview(null)
      },
      onError: (error) => toast(friendlyError(error.message), true),
    })
  }

  const handleMarkRelatedYes = async () => {
    if (!activeFichaId || !relatedItems.length || setItemStatus.isPending) return
    try {
      await Promise.all(
        relatedItems
          .filter((item) => item.status !== 'SI')
          .map((item) => setItemStatus.mutateAsync({ itemCode: item.itemCode, fichaId: activeFichaId, status: 'SI' })),
      )
      toast('Marcamos como Sí las tareas que ya tienen evidencia asociada.')
    } catch (error) {
      toast(friendlyError(error instanceof Error ? error.message : String(error)), true)
    }
  }

  const previewIndex = preview ? uniqueContents.findIndex((content) => content.evidenceIds.includes(preview.id)) : -1
  const previewContent = previewIndex >= 0 ? uniqueContents[previewIndex] : undefined
  const showPrevious = () => {
    if (previewIndex > 0) setPreview(uniqueContents[previewIndex - 1].evidence)
  }
  const showNext = () => {
    if (previewIndex >= 0 && previewIndex < uniqueContents.length - 1) setPreview(uniqueContents[previewIndex + 1].evidence)
  }

  return (
    <div className="grid main-grid">
      <div className="grid">
        <EvidenceGallery
          groups={groups}
          reviewStatus={reviewStatus}
          query={query}
          onQueryChange={setQuery}
          filter={filter}
          onFilterChange={setFilter}
          formatFilter={formatFilter}
          onFormatFilterChange={setFormatFilter}
          expanded={expanded}
          onToggleExpanded={() => setExpanded((current) => !current)}
          onRebuild={handleRebuild}
          rebuilding={rebuildGroups.isPending}
          onPreview={setPreview}
        />
        <EvidenceMiniatures
          contents={uniqueContents}
          totalRows={flatEvidences.length}
          selectedIds={selectedEvidenceIds}
          onSelectionChange={setSelectedEvidenceIds}
          onPreview={setPreview}
        />
        <section className="card">
          <div className="card-pad">
            <div className="side-title">
              <div>
                <h3>Archivos relacionados</h3>
                <p className="helper" style={{ marginTop: 5 }}>
                  Estas son las tareas que ya tienen una evidencia local asociada.
                </p>
              </div>
              <div className="inline evidence-related-actions">
                <span className="badge">{relatedItems.length} con evidencia</span>
                <button className="button ghost small" type="button" onClick={handleMarkRelatedYes} disabled={!relatedItems.length || setItemStatus.isPending}>
                  {setItemStatus.isPending ? 'Guardando…' : 'Marcar relacionadas como Sí'}
                </button>
              </div>
            </div>
            <div className="task-list" style={{ marginTop: 14 }}>
              {relatedItems.length ? (
                relatedItems.map((item) => (
                  <Task key={item.itemCode} item={item} fichaId={activeFichaId} onPreview={setPreview} />
                ))
              ) : (
                <div className="empty">Aún no hay archivos asociados a esta ficha.</div>
              )}
            </div>
          </div>
        </section>
      </div>
      <aside className="side-stack">
        <section className="card">
          <div className="card-pad">
            <h3>Agregar una evidencia</h3>
            <p className="helper" style={{ marginTop: 8 }}>
              Puedes subir una captura o documento que ya tengas en este equipo.
            </p>
            <div className="manual-upload">
              <label htmlFor="manual-evidence-item">Relacionar con una actividad (opcional)</label>
              <select
                id="manual-evidence-item"
                value={uploadItemCode}
                onChange={(event) => setUploadItemCode(event.target.value)}
              >
                <option value="">Evidencia general de la ficha</option>
                {dashboardItems.map((item) => (
                  <option key={item.itemCode} value={item.itemCode}>
                    {item.itemCode} · {item.description}
                  </option>
                ))}
              </select>
              <input
                ref={fileInputRef}
                id="manual-evidence-file"
                type="file"
                accept=".png,.jpg,.jpeg,.pdf,.html"
                hidden
                onChange={handleFileChange}
              />
              <button
                className="button secondary"
                type="button"
                disabled={uploadEvidence.isPending || !activeFichaId}
                onClick={handlePickUpload}
              >
                Subir evidencia
              </button>
              <p className="helper">Acepta PNG, JPG, PDF o HTML.</p>
            </div>
          </div>
        </section>
      </aside>
      {preview && <PreviewModal evidence={preview} sharedItemCodes={previewContent?.itemCodes || []} index={previewIndex} total={uniqueContents.length} onPrevious={showPrevious} onNext={showNext} onClose={() => setPreview(null)} onDelete={handleDelete} />}
    </div>
  )
}

interface EvidenceGalleryProps {
  groups: EvidenceGroup[]
  reviewStatus: ReviewStatusById
  query: string
  onQueryChange: (value: string) => void
  filter: GroupFilter
  onFilterChange: (value: GroupFilter) => void
  formatFilter: string
  onFormatFilterChange: (value: string) => void
  expanded: boolean
  onToggleExpanded: () => void
  onRebuild: () => void
  rebuilding: boolean
  onPreview: (evidence: Evidence) => void
}

function EvidenceMiniatures({
  contents,
  totalRows,
  selectedIds,
  onSelectionChange,
  onPreview,
}: {
  contents: EvidenceContent[]
  totalRows: number
  selectedIds: Set<string>
  onSelectionChange: (ids: Set<string>) => void
  onPreview: (evidence: Evidence) => void
}) {
  const evidences = contents.map((content) => content.evidence)
  const toggle = (id: string) => {
    const next = new Set(selectedIds)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    onSelectionChange(next)
  }

  const selectAll = () => onSelectionChange(new Set(evidences.map((evidence) => evidence.id)))
  const clearAll = () => onSelectionChange(new Set())

  return (
    <section className="card evidence-miniatures">
      <div className="card-pad">
        <div className="side-title">
          <div>
            <div className="eyebrow">Revisión visual</div>
            <h3 style={{ marginTop: 7 }}>Miniaturas seleccionables</h3>
            <p className="helper" style={{ marginTop: 5 }}>
              Selecciona las evidencias que quieres revisar juntas. Esta selección es local y no altera la agrupación del reporte.
            </p>
          </div>
          <span className="badge">{selectedIds.size} seleccionadas</span>
        </div>
        <div className="miniature-toolbar">
          <span className="helper">
            {evidences.length} imagen{evidences.length === 1 ? '' : 'es'} única{evidences.length === 1 ? '' : 's'}
            {totalRows > evidences.length ? ` (${totalRows} registros de evidencia)` : ''}
          </span>
          <div className="inline">
            <button className="button ghost small" type="button" onClick={selectAll} disabled={!evidences.length}>Seleccionar todas</button>
            <button className="button ghost small" type="button" onClick={clearAll} disabled={!selectedIds.size}>Limpiar</button>
          </div>
        </div>
        {evidences.length ? (
          <div className="evidence-miniature-grid">
            {contents.map(({ key, evidence, itemCodes }) => {
              const format = String(evidence.format || '').toLowerCase()
              const image = format.includes('png') || format.includes('jpg') || format.includes('jpeg') || format.includes('webp')
              const selected = selectedIds.has(evidence.id)
              const shared = itemCodes.length > 1
              return (
                <article key={key} className={`evidence-miniature${selected ? ' selected' : ''}`}>
                  <button
                    className="evidence-miniature-select"
                    type="button"
                    aria-pressed={selected}
                    aria-label={`${selected ? 'Quitar de la selección' : 'Seleccionar'} ${evidence.name || 'evidencia local'}`}
                    onClick={() => toggle(evidence.id)}
                  >
                    <span className="evidence-miniature-preview">
                      {image ? <img src={evidenceThumbnailUrl(evidence.id)} alt="" loading="lazy" decoding="async" /> : <span className="evidence-format-icon">{format.toUpperCase() || 'FILE'}</span>}
                      <span className="evidence-select-mark" aria-hidden="true">{selected ? '✓' : ''}</span>
                    </span>
                    <span className="evidence-miniature-copy">
                      <strong>{evidence.name || 'Evidencia local'}</strong>
                      <small>{String(evidence.format || 'archivo').toUpperCase()} · {formatDate(evidence.capturedAt)}</small>
                      {shared ? (
                        <span className="badge" title={`Ítems: ${itemCodes.join(', ')}`}>
                          {usedInLabel(itemCodes)}: {itemCodes.join(', ')}
                        </span>
                      ) : itemCodes.length === 1 ? (
                        <small className="mono">Ítem {itemCodes[0]}</small>
                      ) : null}
                    </span>
                  </button>
                  <button className="evidence-miniature-open" type="button" onClick={() => onPreview(evidence)}>Vista previa</button>
                </article>
              )
            })}
          </div>
        ) : (
          <div className="empty">Aún no hay miniaturas disponibles para esta ficha.</div>
        )}
      </div>
    </section>
  )
}

function EvidenceGallery({
  groups,
  reviewStatus,
  query,
  onQueryChange,
  filter,
  onFilterChange,
  formatFilter,
  onFormatFilterChange,
  expanded,
  onToggleExpanded,
  onRebuild,
  rebuilding,
  onPreview,
}: EvidenceGalleryProps) {
  const normalizedQuery = query.trim().toLowerCase()

  const allFormats = useMemo(() => {
    const set = new Set<string>()
    groups.forEach((group) => {
      const groupEvidences = group.evidences ?? []
      groupEvidences.forEach((evidence) => {
        const format = String(evidence.format || '').toLowerCase()
        if (format) set.add(format)
      })
    })
    return [...set]
  }, [groups])

  const matches = groups.filter((group) => {
    const confidence = groupState(group, reviewStatus).key
    const haystack = [group.title, ...(group.itemCodes || []), group.reason].join(' ').toLowerCase()
    const formatOk =
      formatFilter === 'all' ||
      (group.evidences ?? []).some((evidence) => String(evidence.format || '').toLowerCase() === formatFilter)
    return (filter === 'all' || confidence === filter) && (!normalizedQuery || haystack.includes(normalizedQuery)) && formatOk
  })

  const total = dedupeEvidencesByContent(matches.flatMap((group) => group.evidences ?? [])).length
  const visible = expanded ? matches : matches.slice(0, 6)

  return (
    <section className="card evidence-gallery">
      <div className="card-pad">
        <div className="confidence-intro">
          <div>
            <div className="eyebrow">Evidencias organizadas</div>
            <h3 style={{ marginTop: 7 }}>Galería de evidencias</h3>
            <p className="helper" style={{ marginTop: 6 }}>
              Agrupamos las imágenes idénticas (aunque cubran varios ítems) para que cada una aparezca una sola vez en tu reporte.
            </p>
          </div>
          <button className="button ghost small" type="button" disabled={rebuilding} onClick={onRebuild}>
            Revisar agrupación
          </button>
        </div>
        <div className="gallery-controls">
          <input
            id="evidence-group-search"
            type="search"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="Buscar por título o código"
            aria-label="Buscar grupos de evidencias"
          />
          <select
            id="evidence-group-filter"
            aria-label="Filtrar grupos de evidencias"
            value={filter}
            onChange={(event) => onFilterChange(event.target.value as GroupFilter)}
          >
            <option value="all">Todos los estados</option>
            <option value="review">Por revisar o rechazadas</option>
            <option value="high">Aprobadas</option>
            <option value="manual">Agregadas por ti</option>
          </select>
        </div>
        {allFormats.length ? (
          <div className="checklist-filter-tabs" style={{ marginTop: 10 }}>
            {['all', ...allFormats].map((value) => (
              <button
                key={value}
                type="button"
                className={`checklist-filter-tab ${formatFilter === value ? 'active' : ''}`}
                aria-pressed={formatFilter === value}
                onClick={() => onFormatFilterChange(value)}
              >
                {value === 'all' ? 'Todos los formatos' : value.toUpperCase()}
              </button>
            ))}
          </div>
        ) : null}
        <div className="gallery-summary">
          <div className="gallery-summary-stat"><strong>{matches.length}</strong><span>grupos visibles</span></div>
          <div className="gallery-summary-stat"><strong>{total}</strong><span>imágenes únicas</span></div>
          <div className="gallery-summary-stat"><strong>{groups.length}</strong><span>grupos totales</span></div>
        </div>
        <div className="evidence-gallery-grid">
          {visible.length ? (
            visible.map((group, index) => (
              <EvidenceGroupCard key={group.id || `${group.title || 'grupo'}-${index}`} group={group} reviewStatus={reviewStatus} onPreview={onPreview} />
            ))
          ) : (
            <div className="empty">
              {groups.length === 0
                ? 'Todavía no hay evidencias en esta ficha.'
                : 'No encontramos grupos con esos filtros.'}
            </div>
          )}
        </div>
        {matches.length > 6 && (
          <div className="gallery-more">
            <p className="helper">
              {expanded ? 'Mostrando todos los grupos filtrados.' : 'Mostrando 6 grupos para una revisión rápida.'} El reporte
              conserva todos los archivos relacionados.
            </p>
            <button className="button ghost small" type="button" onClick={onToggleExpanded}>
              {expanded ? 'Mostrar menos' : 'Mostrar todos los grupos'}
            </button>
          </div>
        )}
      </div>
    </section>
  )
}

function EvidenceGroupCard({ group, reviewStatus, onPreview }: { group: EvidenceGroup; reviewStatus: ReviewStatusById; onPreview: (evidence: Evidence) => void }) {
  const evidences = group.evidences ?? []
  const evidence = evidences[0]
  const confidence = groupState(group, reviewStatus)
  const codes = [...(group.itemCodes || [])].sort(compareItemCodes)
  const itemsLabel = codes.slice(0, 5).join(' · ') + (codes.length > 5 ? ' · …' : '')
  // Rows that share the same bytes count as one file.
  const count = dedupeEvidencesByContent(evidences).length

  return (
    <article className="evidence-group-card">
      <div className="evidence-group-top">
        <span className={`confidence ${confidence.key}`}>{confidence.label}</span>
        <span className="confidence-stamp">
          {count} imagen{count === 1 ? '' : 'es'}
        </span>
        {codes.length > 1 ? (
          <span className="badge" title={`Ítems: ${codes.join(', ')}`}>{usedInLabel(codes)}</span>
        ) : null}
      </div>
      <h4>{group.title || 'Evidencia agrupada'}</h4>
      <p className="helper">{itemsLabel || 'Evidencia general de la ficha'}</p>
      <div className="evidence-group-foot">
        <span className="group-reason">{group.reason || 'Misma sección del curso'}</span>
        {evidence && (
          <button className="button ghost small" type="button" onClick={() => onPreview(evidence)}>
            Vista previa
          </button>
        )}
      </div>
    </article>
  )
}

function Task({
  item,
  fichaId,
  onPreview,
}: {
  item: DashboardItem
  fichaId?: string
  onPreview: (evidence: Evidence) => void
}) {
  const toast = useToast()
  const setItemStatus = useSetItemStatus()
  const confidence = confidenceFor(item)
  const maxEvidences = Number(item.maxEvidences) || 1
  const filled = Math.min(Number(item.evidenceCount) || 0, maxEvidences)
  const current = String(item.status || 'PENDIENTE')

  const handleStatus = (status: string) => {
    if (!fichaId || status === current) return
    setItemStatus.mutate(
      { itemCode: item.itemCode, fichaId, status },
      { onError: (error) => toast(friendlyError(error.message), true) },
    )
  }

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
          {item.evidences.length ? (
            item.evidences.map((evidence) => (
              <button key={evidence.id} className="evidence-link" type="button" onClick={() => onPreview(evidence)}>
                Ver evidencia {evidence.slotNumber || 1}
              </button>
            ))
          ) : (
            <span className="helper">Aún no hay un archivo</span>
          )}
        </div>
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
        <button
          type="button"
          className={`status-seg si ${current === 'SI' ? 'active' : ''}`}
          aria-pressed={current === 'SI'}
          onClick={() => handleStatus('SI')}
        >
          Sí
        </button>
        <button
          type="button"
          className={`status-seg no ${current === 'NO' ? 'active' : ''}`}
          aria-pressed={current === 'NO'}
          onClick={() => handleStatus('NO')}
        >
          No
        </button>
        <button
          type="button"
          className={`status-seg pendiente ${current === 'PENDIENTE' ? 'active' : ''}`}
          aria-pressed={current === 'PENDIENTE'}
          onClick={() => handleStatus('PENDIENTE')}
        >
          Pend.
        </button>
      </div>
    </article>
  )
}

function PreviewModal({
  evidence,
  sharedItemCodes,
  index,
  total,
  onPrevious,
  onNext,
  onClose,
  onDelete,
}: {
  evidence: Evidence
  sharedItemCodes: string[]
  index: number
  total: number
  onPrevious: () => void
  onNext: () => void
  onClose: () => void
  onDelete: (id: string) => void
}) {
  const format = String(evidence.format || '').toLowerCase()
  const src = evidenceDownloadUrl(evidence.id)
  const title = evidence.name || 'Vista previa de evidencia'
  const closeRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    closeRef.current?.focus()
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
      if (event.key === 'ArrowLeft') onPrevious()
      if (event.key === 'ArrowRight') onNext()
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
  }, [onClose, onNext, onPrevious])

  return (
    <div
      id="evidence-modal"
      className="evidence-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="evidence-preview-title"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <div className="evidence-dialog">
        <div className="evidence-dialog-head">
          <h3 id="evidence-preview-title">{title}</h3>
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
        <div className="evidence-dialog-navigation" aria-label="Navegar evidencias">
          <button className="button ghost small" type="button" onClick={onPrevious} disabled={index <= 0}>← Anterior</button>
          <span className="helper">{index + 1} de {total}</span>
          <button className="button ghost small" type="button" onClick={onNext} disabled={index < 0 || index >= total - 1}>Siguiente →</button>
        </div>
        <div className="evidence-dialog-foot">
          <span className="helper">
            {evidence.source || 'Evidencia local'} · {formatDate(evidence.capturedAt)}
            {sharedItemCodes.length > 1 ? ` · ${usedInLabel(sharedItemCodes)}: ${sharedItemCodes.join(', ')}` : ''}
          </span>
          <a className="button secondary small" href={src} target="_blank" rel="noreferrer">
            Descargar
          </a>
          <button className="button ghost small" type="button" onClick={() => onDelete(evidence.id)}>
            Eliminar
          </button>
        </div>
      </div>
    </div>
  )
}
