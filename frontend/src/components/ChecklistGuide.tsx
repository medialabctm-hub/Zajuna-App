import { useRef, useState, type ChangeEvent } from 'react'
import { Link } from 'react-router-dom'
import { useCapture, useDiscoverCourseMaps, useSetupStatus, useUploadEvidence } from '../hooks/api'
import { useToast } from '../hooks/useToast'
import { friendlyError } from '../lib/friendlyError'
import { Icon } from './Icon'
import type { ChecklistGuide as Guide, ChecklistGuideKind } from '../types'

const GUIDE_KIND_LABEL: Record<ChecklistGuideKind, string> = {
  'content-absent': 'Falta contenido en Zajuna',
  'empty-section': 'Subsección vacía',
  'route-missing': 'No encontramos la sección',
}

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
 * Resumen plegado de los ítems que dependen del instructor. Va cerrado por
 * defecto para no competir con el checklist: una línea y, al abrir, la lista.
 */
export function GuideSummary({ guides }: { guides: Guide[] }) {
  if (!guides.length) return null
  return (
    <details className="card guide-summary">
      <summary>
        <span className="guide-summary-icon" aria-hidden="true">
          <Icon name="help" size={16} />
        </span>
        <span className="guide-summary-copy">
          <strong>
            {guides.length === 1 ? '1 ítem depende de ti en Zajuna' : `${guides.length} ítems dependen de ti en Zajuna`}
          </strong>
          <small>No podemos publicar contenido en tu curso. Te decimos qué hacer y nosotros terminamos el resto.</small>
        </span>
        <span className="guide-summary-toggle" aria-hidden="true">Ver guías</span>
      </summary>
      <ul className="guide-list">
        {guides.map((guide) => (
          <li key={guide.itemCode}>
            <Link to={`/checklist/${encodeURIComponent(guide.itemCode)}#guia`}>
              <span className="guide-list-code mono">{guide.itemCode}</span>
              <span className="guide-list-copy">
                <strong>{guide.headline}</strong>
                <small>{guide.categoryLabel} · {GUIDE_KIND_LABEL[guide.kind]}</small>
              </span>
              <span className="guide-list-arrow" aria-hidden="true">→</span>
            </Link>
          </li>
        ))}
      </ul>
    </details>
  )
}

interface GuideCardProps {
  guide: Guide
  fichaId: string
  /** El ítem tiene al menos una ruta: «Ya lo hice» puede volver a capturarlo. */
  canRecapture: boolean
}

/**
 * Guía de un ítem que la app no puede completar sola: qué hacer en Zajuna, un
 * enlace directo a la página, texto sugerido y lo que el instructor entrega
 * para que la app termine (volver a capturar o subir su evidencia).
 */
export function GuideCard({ guide, fichaId, canRecapture }: GuideCardProps) {
  const toast = useToast()
  const { data: setup } = useSetupStatus()
  const capture = useCapture()
  const upload = useUploadEvidence()
  const discover = useDiscoverCourseMaps()
  const fileRef = useRef<HTMLInputElement>(null)
  const [copied, setCopied] = useState(false)
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
      onSuccess: () => toast('Recibimos tu evidencia. La revisamos y, si está bien, el ítem queda cumplido.'),
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
  return (
    <section className="card guide-card" id="guia" aria-labelledby="guide-title">
      <div className="card-pad">
        <div className="guide-card-head">
          <div>
            <div className="eyebrow">Cómo completarlo · {GUIDE_KIND_LABEL[guide.kind]}</div>
            <h3 id="guide-title">{guide.headline}</h3>
            <p className="helper">{guide.why}</p>
          </div>
          {zajunaUrl ? (
            <a className="button ghost small" href={zajunaUrl} target="_blank" rel="noreferrer">
              Abrir en Zajuna ↗
            </a>
          ) : null}
        </div>

        {guide.detected ? <p className="guide-detected">{guide.detected}</p> : null}

        <ol className="guide-steps">
          {guide.steps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
        {zajunaUrl && guide.zajunaLabel ? (
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
                disabled={capture.isPending || !canRecapture}
                title={canRecapture ? undefined : 'Primero busca las rutas de nuevo para encontrar la página.'}
              >
                {capture.isPending ? 'Enviando…' : 'Ya lo hice, verificar'}
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
            <small className="helper">PNG, JPG o PDF de hasta 25 MB{guide.missingSlots?.length ? ` · se guarda en el slot ${slot}` : ''}.</small>
          ) : null}
        </div>
      </div>
    </section>
  )
}
