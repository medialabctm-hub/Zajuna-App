import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import { Link } from 'react-router-dom'
import { useCapture, useDiscoverCourseMaps, useJobs, useSetupStatus, useUploadEvidence } from '../hooks/api'
import { useToast } from '../hooks/useToast'
import { friendlyError } from '../lib/friendlyError'
import { friendlyJobMessage, friendlyJobStage } from '../lib/format'
import { explainJobFailure } from '../lib/jobFailure'
import { GUIDE_KIND_LABEL, guideVerification, latestItemCaptureJob, relativeTime, type GuideVerification } from '../lib/guideVerification'
import { Icon } from './Icon'
import type { ChecklistGuide as Guide } from '../types'


/** Solo se enlaza a páginas https: la URL viene del mapa de rutas del curso. */
function safeZajunaUrl(url?: string) {
  if (!url) return ''
  try {
    return new URL(url).protocol === 'https:' ? url : ''
  } catch {
    return ''
  }
}

/**
 * Aviso de una línea en el Checklist: cuántos ítems dependen del instructor y
 * un enlace a su apartado, Guías, donde se completan con su estado en vivo.
 */
export function GuideSummary({ guides }: { guides: Guide[] }) {
  if (!guides.length) return null
  return (
    <section className="card guide-summary">
      <Link className="guide-summary-link" to="/guias">
        <span className="guide-summary-icon" aria-hidden="true">
          <Icon name="help" size={16} />
        </span>
        <span className="guide-summary-copy">
          <strong>
            {guides.length === 1 ? '1 ítem depende de ti en Zajuna' : `${guides.length} ítems dependen de ti en Zajuna`}
          </strong>
          <small>No podemos publicar contenido en tu curso. Te decimos qué hacer y nosotros terminamos el resto.</small>
        </span>
        <span className="guide-summary-toggle">Ver guías →</span>
      </Link>
    </section>
  )
}

interface GuideCardProps {
  guide: Guide
  fichaId: string
  /** Estado del ítem en el checklist (SI, NO, PENDIENTE). */
  itemStatus?: string
  /** El ítem tiene al menos una ruta: «Ya lo hice» puede volver a capturarlo. */
  canRecapture: boolean
}

/** Clave de la última subida del instructor, para recordarla al recargar. */
function uploadKey(fichaId: string, itemCode: string) {
  return `zajuna.guideUpload.${fichaId}.${itemCode}`
}

function readUploadTime(fichaId: string, itemCode: string) {
  try {
    return window.sessionStorage.getItem(uploadKey(fichaId, itemCode)) || ''
  } catch {
    return ''
  }
}

function saveUploadTime(fichaId: string, itemCode: string, value: string) {
  try {
    window.sessionStorage.setItem(uploadKey(fichaId, itemCode), value)
  } catch {
    // Sin almacenamiento solo se pierde el aviso al recargar.
  }
}

/** Tras subir, las guías se refrescan: hasta entonces se sigue mostrando la revisión. */
const UPLOAD_SETTLE_MS = 4000

/** Franja de una línea con lo que está pasando con la verificación del ítem. */
function VerificationStrip({ state, detected, contentError = false, sending, uploading, uploadedAt }: { state: GuideVerification; detected?: string; contentError?: boolean; sending: boolean; uploading: boolean; uploadedAt: string }) {
  if (sending && state.kind !== 'running') {
    return (
      <div className="guide-live running">
        <span className="guide-live-dot" aria-hidden="true" />
        <div className="guide-live-copy">
          <strong role="status">Enviando…</strong>
          <small>Preparamos la verificación de este ítem en Zajuna.</small>
        </div>
      </div>
    )
  }
  const settling = !!uploadedAt && Date.now() - Date.parse(uploadedAt) < UPLOAD_SETTLE_MS
  if (uploading || settling) {
    return (
      <div className="guide-live running">
        <span className="guide-live-dot" aria-hidden="true" />
        <div className="guide-live-copy">
          <strong role="status">Revisando tu evidencia…</strong>
          <small>La guardamos y la revisamos automáticamente.</small>
        </div>
      </div>
    )
  }
  // Una subida posterior a la última verificación manda: su resultado es el que importa.
  const uploadIsNewer = !!uploadedAt && (state.kind === 'idle' || Date.parse(uploadedAt) > Date.parse(state.job.finishedAt || state.job.updatedAt || state.job.createdAt || ''))
  if (uploadIsNewer && state.kind !== 'running') {
    return (
      <div className="guide-live pending">
        <Icon name="warning" size={14} />
        <div className="guide-live-copy">
          <strong role="status">Recibimos tu evidencia, pero quedó por revisar</strong>
          <small>
            La revisión automática no pudo aprobarla ({relativeTime(uploadedAt)}). Ábrela en <Link to="/revision">Revisión</Link> para aprobarla o
            cambiarla.
          </small>
        </div>
      </div>
    )
  }
  if (state.kind === 'idle') return null
  const detail = <Link to={`/trabajos/${encodeURIComponent(state.job.id)}`}>Ver detalle</Link>
  if (state.kind === 'running') {
    const progress = Math.max(0, Math.min(100, Number(state.job.progress) || 0))
    const step = state.queued ? 'En cola: empieza en cuanto termine el trabajo anterior.' : friendlyJobMessage(state.job.message) || friendlyJobStage(state.job.stage)
    return (
      <div className="guide-live running">
        <span className="guide-live-dot" aria-hidden="true" />
        <div className="guide-live-copy">
          <strong role="status">Verificando en Zajuna…</strong>
          <small>
            {step} · {detail}
          </small>
          <span className="guide-live-progress" aria-hidden="true">
            <i style={{ width: `${progress}%` }} />
          </span>
        </div>
      </div>
    )
  }
  if (state.kind === 'settling') {
    return (
      <div className="guide-live running">
        <span className="guide-live-dot" aria-hidden="true" />
        <div className="guide-live-copy">
          <strong role="status">Revisando el resultado…</strong>
          <small>La verificación terminó; estamos actualizando el estado del ítem.</small>
        </div>
      </div>
    )
  }
  if (state.kind === 'cancelled') {
    return (
      <div className="guide-live pending">
        <Icon name="warning" size={14} />
        <div className="guide-live-copy">
          <strong role="status">Verificación cancelada</strong>
          <small>Puedes volver a pulsar «Ya lo hice, verificar» cuando quieras · {detail}</small>
        </div>
      </div>
    )
  }
  if (state.kind === 'fulfilled') {
    return (
      <div className="guide-live done">
        <Icon name="check" size={14} />
        <div className="guide-live-copy">
          <strong role="status">¡Listo! Encontramos la evidencia y el ítem quedó cumplido.</strong>
        </div>
      </div>
    )
  }
  if (state.kind === 'pending') {
    return (
      <div className="guide-live pending">
        <Icon name="search" size={14} />
        <div className="guide-live-copy">
          <strong role="status">{contentError ? 'El contenido sigue con errores' : 'Seguimos sin encontrarlo en Zajuna'}</strong>
          <small>
            {detected ? `${detected.replace(/^Última verificación:\s*/, '')} ` : ''}
            Última verificación {relativeTime(state.job.finishedAt || state.job.updatedAt)} · {detail}
          </small>
        </div>
      </div>
    )
  }
  const failure = explainJobFailure(state.job)
  return (
    <div className="guide-live failed">
      <Icon name="warning" size={14} />
      <div className="guide-live-copy">
        <strong role="status">No pudimos verificar: {failure.title}</strong>
        <small>
          {failure.next} · {detail}
        </small>
      </div>
    </div>
  )
}

/**
 * Guía del ítem en su página de detalle. Si la guía desaparece mientras la
 * persona está aquí (la verificación o su evidencia completaron el ítem), deja
 * en su lugar la confirmación de que quedó cumplido.
 */
export function ItemGuide({ guide, fichaId, itemStatus, canRecapture, resolved = false }: Omit<GuideCardProps, 'guide'> & { guide?: Guide; resolved?: boolean }) {
  const [sawGuide, setSawGuide] = useState(!!guide || resolved)
  useEffect(() => {
    if (guide) setSawGuide(true)
  }, [guide])
  if (guide) return <GuideCard guide={guide} fichaId={fichaId} itemStatus={itemStatus} canRecapture={canRecapture} />
  if (!sawGuide || itemStatus !== 'SI') return null
  return (
    <section className="card guide-card resolved" id="guia">
      <div className="card-pad guide-resolved">
        <span className="guide-resolved-icon" aria-hidden="true">
          <Icon name="check" size={16} />
        </span>
        <div>
          <strong role="status">¡Listo! Encontramos la evidencia y el ítem quedó cumplido.</strong>
          <p className="helper">
            Ya cuenta en el checklist y en el reporte. Puedes verla en <Link to="/evidencias">Evidencias</Link> o volver a{' '}
            <Link to="/guias">las guías pendientes</Link>.
          </p>
        </div>
      </div>
    </section>
  )
}

/**
 * Guía de un ítem que la app no puede completar sola: qué hacer en Zajuna, un
 * enlace directo a la página, texto sugerido y lo que el instructor entrega
 * para que la app termine (volver a capturar o subir su evidencia).
 */
export function GuideCard({ guide, fichaId, itemStatus, canRecapture }: GuideCardProps) {
  const toast = useToast()
  const { data: setup } = useSetupStatus()
  const { data: jobs } = useJobs()
  const capture = useCapture()
  const upload = useUploadEvidence()
  const discover = useDiscoverCourseMaps()
  const fileRef = useRef<HTMLInputElement>(null)
  const [copied, setCopied] = useState(false)
  const [uploadedAt, setUploadedAt] = useState(() => readUploadTime(fichaId, guide.itemCode))
  // El estado sale de la lista de trabajos, así sobrevive a recargar o navegar.
  // El trabajo que devuelve el botón cuenta al instante, antes de que la
  // lista se refresque.
  const knownJobs = capture.data ? [...(jobs ?? []), capture.data] : jobs ?? []
  const rawVerification = guideVerification(latestItemCaptureJob(knownJobs, fichaId, guide.itemCode), itemStatus)
  // A recommendation's item is fulfilled already: while the recommendation
  // exists, the content still has errors, so a finished check is «sigue con
  // errores», never «¡Listo!».
  const verification = guide.advisory && rawVerification.kind === 'fulfilled' ? { kind: 'pending' as const, job: rawVerification.job } : rawVerification
  const verifying = capture.isPending || verification.kind === 'running'
  const [, setTick] = useState(0)
  // Vuelve a pintar cuando termina el margen tras subir (ver UPLOAD_SETTLE_MS).
  useEffect(() => {
    if (!uploadedAt) return
    const left = UPLOAD_SETTLE_MS - (Date.now() - Date.parse(uploadedAt))
    if (left <= 0) return
    const timer = window.setTimeout(() => setTick((value) => value + 1), left + 50)
    return () => window.clearTimeout(timer)
  }, [uploadedAt])
  const account = { username: setup?.zajunaUsername || '', documentType: setup?.zajunaDocumentType || 'CC' }
  const zajunaUrl = safeZajunaUrl(guide.zajunaUrl)
  const slot = guide.missingSlots?.[0] || 1

  function handleRecapture() {
    capture.mutate(
      { fichaId, ...account, itemCodes: [guide.itemCode] },
      {
        onSuccess: () => toast(`Verificando el ítem ${guide.itemCode} en Zajuna. Si la evidencia sale bien, lo marcamos como cumplido.`),
        onError: (error) => toast(friendlyError(error.message), true),
      },
    )
  }

  function handleRediscover() {
    discover.mutate(
      { ...account, fichaId },
      {
        onSuccess: () => toast('Buscamos las rutas del curso de nuevo. Cuando termine, pulsa «Ya lo hice, verificar».'),
        onError: (error) => toast(friendlyError(error.message), true),
      },
    )
  }

  function handleFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    const form = new FormData()
    form.set('fichaId', fichaId)
    form.set('itemCode', guide.itemCode)
    form.set('slotNumber', String(slot))
    form.set('name', `${guide.itemCode} · ${file.name}`)
    form.set('file', file)
    upload.mutate(form, {
      onSuccess: () => {
        const now = new Date().toISOString()
        saveUploadTime(fichaId, guide.itemCode, now)
        setUploadedAt(now)
        toast('Recibimos tu evidencia. La revisamos y, si está bien, el ítem queda cumplido.')
      },
      onError: (error) => toast(friendlyError(error.message), true),
    })
  }

  async function handleCopy() {
    if (!guide.template) return
    const text = guide.template.title ? `${guide.template.title}\n\n${guide.template.body}` : guide.template.body
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      toast('No pudimos copiar el texto. Selecciónalo y cópialo a mano.', true)
    }
  }

  const actions = new Set(guide.actions)
  // A recommendation's item is already fulfilled: only re-checking makes sense.
  if (guide.advisory) actions.delete('upload')
  return (
    <section className="card guide-card" id="guia" aria-labelledby="guide-title">
      <div className="card-pad">
        <div className="guide-card-head">
          <div>
            <div className="eyebrow">Cómo completarlo · {GUIDE_KIND_LABEL[guide.kind]}</div>
            <h3 id="guide-title">{guide.headline}</h3>
            {guide.requirement ? <p className="guide-requirement">{guide.requirement}</p> : null}
            {guide.location ? <p className="guide-location" aria-label={`Dónde está en Zajuna: ${guide.location}`}>{guide.location}</p> : null}
            {guide.alsoItems?.length ? <p className="guide-also">También resuelve: {guide.alsoItems.join(', ')}</p> : null}
            <p className="helper">{guide.why}</p>
          </div>
          {zajunaUrl ? (
            <a className="button ghost small" href={zajunaUrl} target="_blank" rel="noreferrer">
              Abrir en Zajuna ↗
            </a>
          ) : null}
        </div>

        <VerificationStrip state={verification} detected={guide.detected} contentError={guide.kind === 'content-error'} sending={capture.isPending} uploading={upload.isPending} uploadedAt={uploadedAt} />
        {guide.detected && verification.kind !== 'pending' ? <p className="guide-detected">{guide.detected}</p> : null}

        <ol className="guide-steps">
          {guide.steps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
        {zajunaUrl && guide.zajunaLabel && !guide.location ? (
          <p className="helper guide-where">Página: {guide.zajunaLabel.replace(/\s+—\s+Evidencia\s+\d+$/, '')}</p>
        ) : null}

        {guide.template ? (
          <details className="guide-template">
            <summary>{guide.template.label}</summary>
            <div className="guide-template-body">
              {guide.template.title ? <strong>{guide.template.title}</strong> : null}
              <pre>{guide.template.body}</pre>
              <button type="button" className="button ghost small" onClick={handleCopy}>
                {copied ? 'Copiado' : 'Copiar texto'}
              </button>
            </div>
          </details>
        ) : null}

        <div className="guide-handoff">
          <strong>Lo que necesitamos de ti</strong>
          <p>{guide.handoff}</p>
          <div className="guide-actions">
            {actions.has('rediscover') ? (
              <button type="button" className="button ghost small" onClick={handleRediscover} disabled={discover.isPending}>
                {discover.isPending ? 'Enviando…' : 'Buscar rutas de nuevo'}
              </button>
            ) : null}
            {actions.has('recapture') ? (
              <button
                type="button"
                className="button primary small"
                onClick={handleRecapture}
                disabled={verifying || !canRecapture}
                aria-busy={verifying}
                title={canRecapture ? undefined : 'Primero busca las rutas de nuevo para encontrar la página.'}
              >
                {capture.isPending ? 'Enviando…' : verifying ? 'Verificando…' : 'Ya lo hice, verificar'}
              </button>
            ) : null}
            {actions.has('upload') ? (
              <>
                <button type="button" className="button ghost small" onClick={() => fileRef.current?.click()} disabled={upload.isPending}>
                  {upload.isPending ? 'Subiendo…' : 'Subir mi evidencia'}
                </button>
                <input ref={fileRef} type="file" accept=".png,.jpg,.jpeg,.pdf" hidden onChange={handleFile} aria-label={`Subir evidencia del ítem ${guide.itemCode}`} />
              </>
            ) : null}
          </div>
          {actions.has('upload') ? (
            <small className="helper guide-upload-hint">
              {guide.evidenceHint ? <><strong>Qué subir:</strong> {guide.evidenceHint} </> : null}
              PNG, JPG o PDF de hasta 25 MB{guide.missingSlots?.length ? ` · se guarda en el slot ${slot}` : ''}.
            </small>
          ) : null}
        </div>
      </div>
    </section>
  )
}
