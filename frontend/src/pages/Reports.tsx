import { useState } from 'react'
import type { CSSProperties } from 'react'
import { reportDownloadUrl } from '../api/client'
import { PageError, PageSkeleton } from '../components/AsyncState'
import { Icon } from '../components/Icon'
import { useCreateBackup, useDashboard, useGenerateReport, useReports, isNotFound } from '../hooks/api'
import { useToast } from '../hooks/useToast'
import { formatDate, jobStatusClass, reportStatusLabel } from '../lib/format'
import type { JobStatus, Report } from '../types'

function getErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

function ReportRow({ report }: { report: Report }) {
  return (
    <article className="report-row">
      <div className="report-row-copy">
        <span className="report-icon">
          <Icon name="report" size={16} />
        </span>
        <div>
          <strong>{report.name || 'Reporte de evidencias'}</strong>
          <small>
            {String(report.format || 'pdf').toUpperCase()} · {formatDate(report.updatedAt || report.createdAt)}
          </small>
        </div>
      </div>
      <div className="inline">
        <span className={`status-chip ${jobStatusClass(report.status as JobStatus)}`}>{reportStatusLabel(report.status)}</span>
        {report.status === 'completed' ? (
          <a className="button secondary small" href={reportDownloadUrl(report.id)} target="_blank" rel="noreferrer">
            Abrir PDF
          </a>
        ) : null}
      </div>
    </article>
  )
}

const twoColStyle: CSSProperties = { marginTop: 16 }
const fieldStyle: CSSProperties = { marginTop: 0 }
const formActionsStyle: CSSProperties = { justifyContent: 'flex-start', marginTop: 18 }

export function Reports() {
  const toast = useToast()
  const dashboardQuery = useDashboard()
  const dashboard = dashboardQuery.data
  const reportsQuery = useReports()
  const reports = reportsQuery.data
  const generateReport = useGenerateReport()
  const createBackup = useCreateBackup()

  const [format, setFormat] = useState<'pdf' | 'html'>('pdf')
  const [evidenceLimit, setEvidenceLimit] = useState(100)

  if (dashboardQuery.isLoading || reportsQuery.isLoading) return <PageSkeleton label="Cargando reportes locales" />
  if (reportsQuery.isError) return <PageError message="No pudimos cargar los reportes locales." action={<button className="button" onClick={() => reportsQuery.refetch()}>Reintentar</button>} />
  if (dashboardQuery.isError && !isNotFound(dashboardQuery.error)) {
    return <PageError message="No pudimos cargar la ficha activa." action={<button className="button" onClick={() => dashboardQuery.refetch()}>Reintentar</button>} />
  }

  const handleGenerateReport = () => {
    if (!dashboard) {
      toast('Selecciona una ficha activa antes de generar un reporte.', true)
      return
    }
    generateReport.mutate(
      {
        title: `Reporte de la ficha ${dashboard.ficha.externalId}`,
        fichaId: dashboard.activeFichaId,
        format,
        evidenceLimit,
      },
      {
        onSuccess: () => toast('Estamos preparando tu reporte.'),
        onError: (error) => toast(getErrorMessage(error), true),
      },
    )
  }

  const handleBackup = () => {
    createBackup.mutate(undefined, {
      onSuccess: () => toast('Copia de respaldo guardada correctamente.'),
      onError: (error) => toast(getErrorMessage(error), true),
    })
  }

  const rows = reports || []
  const generateDisabled = !dashboard || generateReport.isPending

  return (
    <div className="grid">
      <section className="card reports-card">
        <div className="card-pad">
          <div className="confidence-intro">
            <div>
              <div className="eyebrow">Historial de reportes</div>
              <h3 style={{ marginTop: 7 }}>Reportes disponibles</h3>
              <p className="helper" style={{ marginTop: 6 }}>
                Genera un PDF cuando termines de revisar y ábrelo o descárgalo desde este equipo.
              </p>
            </div>
            <button className="button primary small" type="button" onClick={handleGenerateReport} disabled={generateDisabled}>
              {generateReport.isPending ? 'Procesando…' : format === 'html' ? 'Generar HTML' : 'Generar PDF'}
            </button>
          </div>
          <div className="report-list">
            {rows.length ? (
              rows.map((report) => <ReportRow key={report.id} report={report} />)
            ) : (
              <div className="empty">{dashboard ? 'Aún no has generado un reporte.' : 'Selecciona una ficha activa para generar un reporte. Los reportes ya creados siguen visibles aquí.'}</div>
            )}
          </div>
        </div>
      </section>
      <section className="card">
        <div className="card-pad">
          <div className="eyebrow">Nuevo reporte</div>
          <h3 style={{ marginTop: 7 }}>Personalizar antes de generar</h3>
          <p className="helper" style={{ marginTop: 6 }}>
            Elige el formato y cuántas evidencias incluir; el resto de los datos vienen de la ficha activa.
          </p>
          <div className="grid two-col" style={twoColStyle}>
            <div className="field" style={fieldStyle}>
              <label htmlFor="report-format">Formato</label>
              <select id="report-format" value={format} onChange={(event) => setFormat(event.target.value as 'pdf' | 'html')}>
                <option value="pdf">PDF</option>
                <option value="html">HTML</option>
              </select>
            </div>
            <div className="field" style={fieldStyle}>
              <label htmlFor="report-limit">Límite de evidencias</label>
              <input
                id="report-limit"
                type="number"
                min={10}
                max={500}
                step={10}
                value={evidenceLimit}
                onChange={(event) => setEvidenceLimit(Number(event.target.value) || 100)}
              />
            </div>
          </div>
          <div className="form-actions" style={formActionsStyle}>
            <button className="button primary" type="button" onClick={handleGenerateReport} disabled={generateDisabled}>
              {generateReport.isPending ? 'Procesando…' : 'Generar reporte'}
            </button>
          </div>
        </div>
      </section>
      <section className="card">
        <div className="card-pad">
          <div className="side-title">
            <div>
              <h3>Respaldo local</h3>
              <p className="helper" style={{ marginTop: 5 }}>
                Guarda una copia de la base, evidencias y reportes para recuperarlos más adelante.
              </p>
            </div>
            <button className="button secondary small" type="button" onClick={handleBackup} disabled={createBackup.isPending}>
              {createBackup.isPending ? 'Procesando…' : 'Guardar copia'}
            </button>
          </div>
          <div className="route-note">Las credenciales no se incluyen en la copia: permanecen protegidas en el sistema.</div>
        </div>
      </section>
    </div>
  )
}
