import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { MissingActiveFicha, PageError, PageSkeleton } from '../components/AsyncState'
import { ItemGuide } from '../components/ChecklistGuide'
import { Icon } from '../components/Icon'
import { useChecklistGuides, useDashboard, useJobs, useTargets, isNotFound, useEvidenceReview } from '../hooks/api'
import {
  GUIDE_KIND_LABEL,
  guideRowStatus,
  guideVerification,
  latestItemCaptureJob,
  resolvedGuideCodes,
  updateSeenGuides,
  findGuideForItem,
} from '../lib/guideVerification'
import type { ChecklistGuide, DashboardItem, Job } from '../types'

function seenKey(fichaId: string) {
  return `zajuna.guidesSeen.${fichaId}`
}

function readSeen(fichaId: string): Record<string, string> {
  try {
    const raw = window.localStorage.getItem(seenKey(fichaId))
    const parsed = raw ? JSON.parse(raw) : {}
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

function writeSeen(fichaId: string, value: Record<string, string>) {
  try {
    window.localStorage.setItem(seenKey(fichaId), JSON.stringify(value))
  } catch {
    // Sin almacenamiento solo se pierde «Cumplidos recientemente».
  }
}

function GuideRow({ guide, fichaId, item, jobs, selected }: { guide: ChecklistGuide; fichaId: string; item?: DashboardItem; jobs: Job[]; selected: boolean }) {
  const verificationState = guideVerification(latestItemCaptureJob(jobs, fichaId, guide.itemCode), item?.status)
  const status = guide.advisory && verificationState.kind !== 'running' && verificationState.kind !== 'settling' ? { label: 'Recomendación', tone: 'pending' as const } : guideRowStatus(verificationState, guide.kind)
  return (
    <li>
      <Link className={`guide-row${selected ? ' selected' : ''}`} to={`/guias/${encodeURIComponent(guide.itemCode)}`} aria-current={selected ? 'page' : undefined}>
        <span className="guide-list-code mono">{guide.itemCode}</span>
        <span className="guide-list-copy">
          <strong>{guide.headline}</strong>
          <small>
            {guide.categoryLabel} · {GUIDE_KIND_LABEL[guide.kind]}
            {guide.alsoItems?.length ? ` · también ${guide.alsoItems.length} ítem${guide.alsoItems.length === 1 ? '' : 's'} más` : ''}
          </small>
        </span>
        <span className={`guide-row-status ${status.tone}`}>
          {status.tone === 'running' ? <i className="guide-live-dot" aria-hidden="true" /> : null}
          {status.label}
        </span>
      </Link>
    </li>
  )
}

/**
 * Apartado de las guías: lo que solo el instructor puede hacer en Zajuna, con
 * el estado en vivo de cada verificación y la guía completa en la misma
 * página (/guias/:itemCode).
 */
export function Guides() {
  const { itemCode } = useParams<{ itemCode: string }>()
  const selectedCode = itemCode ? decodeURIComponent(itemCode) : ''
  const dashboardQuery = useDashboard()
  const fichaId = dashboardQuery.data?.activeFichaId || ''
  const guidesQuery = useChecklistGuides(fichaId || undefined)
  const targetsQuery = useTargets(fichaId || undefined)
  const reviewQuery = useEvidenceReview(fichaId || undefined)
  const { data: jobs } = useJobs()
  const guides = guidesQuery.data?.guides ?? []
  const advice = guidesQuery.data?.advice ?? []
  const items = dashboardQuery.data?.items ?? []
  const codes = guides.map((guide) => guide.itemCode)
  const codesKey = codes.join('|')
  const [seen, setSeen] = useState<Record<string, string>>({})

  // Recuerda qué guías hubo: las que desaparecen con el ítem en «SI» se
  // muestran como cumplidas durante un día.
  useEffect(() => {
    if (!fichaId || !guidesQuery.data) return
    const next = updateSeenGuides(readSeen(fichaId), codesKey ? codesKey.split('|') : [], new Date().toISOString())
    writeSeen(fichaId, next)
    setSeen(next)
  }, [fichaId, guidesQuery.data, codesKey])

  if (dashboardQuery.isLoading) return <PageSkeleton label="Cargando guías" />
  if (dashboardQuery.isError && isNotFound(dashboardQuery.error)) {
    return <MissingActiveFicha message="Elige una ficha para ver lo que depende de ti en Zajuna." />
  }
  if (dashboardQuery.isError || !fichaId) return <MissingActiveFicha />
  if (guidesQuery.isLoading) return <PageSkeleton label="Cargando guías" />
  if (guidesQuery.isError) {
    return <PageError message="No pudimos cargar las guías de esta ficha." action={<button className="button" type="button" onClick={() => guidesQuery.refetch()}>Reintentar</button>} />
  }

  const statusByCode = Object.fromEntries(items.map((item) => [item.itemCode, item.status]))
  const resolved = resolvedGuideCodes(seen, codes, statusByCode)
  const allJobs = jobs ?? []
  const selectedGuide = findGuideForItem(guides, selectedCode) ?? findGuideForItem(advice, selectedCode)
  // Evidence uploaded for this item that the automatic review left pending:
  // the guide is gone (the item has evidence) but it is not fulfilled yet.
  const pendingUpload = (reviewQuery.data?.evidences ?? []).some((entry) => entry.itemCode === selectedCode && !entry.superseded && entry.status !== 'approved')
  const selectedItem = items.find((item) => item.itemCode === selectedCode)
  const targets = (targetsQuery.data?.targets ?? []).filter((target) => target.itemCode === selectedCode || target.coveredItemCodes?.includes(selectedCode))

  return (
    <div className={`guides-layout${selectedCode ? ' has-detail' : ''}`}>
      <section className="card guides-list-card" aria-label="Guías de la ficha">
        <div className="card-pad">
          <div className="side-title">
            <div>
              <h3>{guides.length ? (guides.length === 1 ? '1 ítem pendiente' : `${guides.length} ítems pendientes`) : 'Nada pendiente'}</h3>
              <p className="helper" style={{ marginTop: 5 }}>Ficha {dashboardQuery.data?.ficha?.externalId}</p>
            </div>
          </div>
          {guides.length ? (
            <ul className="guide-list">
              {guides.map((guide) => (
                <GuideRow key={guide.itemCode} guide={guide} fichaId={fichaId} item={items.find((item) => item.itemCode === guide.itemCode)} jobs={allJobs} selected={guide.itemCode === selectedCode} />
              ))}
            </ul>
          ) : (
            <div className="empty">Todo lo que depende de ti en Zajuna está al día. Si una captura vuelve a encontrar algo faltante, aparecerá aquí con su guía.</div>
          )}
          {advice.length ? (
            <div className="guides-resolved">
              <div className="eyebrow">Recomendaciones · no cuentan como pendientes</div>
              <ul className="guide-list">
                {advice.map((guide) => (
                  <GuideRow key={`advice-${guide.itemCode}`} guide={guide} fichaId={fichaId} item={items.find((item) => item.itemCode === guide.itemCode)} jobs={allJobs} selected={guide.itemCode === selectedCode} />
                ))}
              </ul>
            </div>
          ) : null}
          {resolved.length ? (
            <div className="guides-resolved">
              <div className="eyebrow">Cumplidos recientemente</div>
              <ul className="guide-list">
                {resolved.map((code) => {
                  const item = items.find((entry) => entry.itemCode === code)
                  return (
                    <li key={code}>
                      <Link className={`guide-row${code === selectedCode ? ' selected' : ''}`} to={`/guias/${encodeURIComponent(code)}`}>
                        <span className="guide-list-code mono">{code}</span>
                        <span className="guide-list-copy">
                          <strong>{item?.description || 'Ítem del checklist'}</strong>
                          <small>{item?.categoryLabel}</small>
                        </span>
                        <span className="guide-row-status done">
                          <Icon name="check" size={12} /> ¡Cumplido!
                        </span>
                      </Link>
                    </li>
                  )
                })}
              </ul>
            </div>
          ) : null}
        </div>
      </section>

      <div className="guides-detail">
        {selectedCode ? (
          <>
            <Link className="button ghost small guides-back" to="/guias">
              ← Todas las guías
            </Link>
            {selectedGuide || selectedItem?.status === 'SI' ? (
              <ItemGuide
                key={selectedCode}
                guide={selectedGuide}
                fichaId={fichaId}
                itemStatus={selectedItem?.status}
                canRecapture={targets.length > 0}
                resolved={!selectedGuide && selectedItem?.status === 'SI'}
              />
            ) : pendingUpload ? (
              <section className="card">
                <div className="card-pad">
                  <h3>Tu evidencia quedó por revisar</h3>
                  <p className="helper" style={{ marginTop: 6 }}>
                    La recibimos, pero la revisión automática no pudo aprobarla. Ábrela en <Link to="/revision">Revisión</Link> para aprobarla o cambiarla; al
                    aprobarla, el ítem queda cumplido.
                  </p>
                </div>
              </section>
            ) : (
              <section className="card">
                <div className="card-pad">
                  <h3>Este ítem no tiene una guía pendiente</h3>
                  <p className="helper" style={{ marginTop: 6 }}>
                    La app puede completarlo sola. Revisa su estado en <Link to={`/checklist/${encodeURIComponent(selectedCode)}`}>el checklist</Link>.
                  </p>
                </div>
              </section>
            )}
          </>
        ) : guides.length ? (
          <section className="card guides-placeholder">
            <div className="card-pad">
              <span className="guide-summary-icon" aria-hidden="true">
                <Icon name="help" size={16} />
              </span>
              <h3>Elige un ítem para ver su guía</h3>
              <p className="helper">
                Cada guía te dice qué hacer en Zajuna, te lleva a la página exacta y te muestra aquí mismo el avance de «Ya lo hice, verificar».
              </p>
            </div>
          </section>
        ) : null}
      </div>
    </div>
  )
}
