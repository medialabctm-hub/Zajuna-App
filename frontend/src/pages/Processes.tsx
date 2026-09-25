import { useState } from 'react'
import { Link } from 'react-router-dom'
import { PageError, PageSkeleton } from '../components/AsyncState'
import { useJobs } from '../hooks/api'
import { friendlyJobMessage, friendlyJobStatus, friendlyJobType, jobStatusClass } from '../lib/format'
import { explainJobFailure } from '../lib/jobFailure'
import type { Job, JobStatus } from '../types'

type JobFilterValue = 'all' | 'running' | 'queued' | 'waiting_user' | 'retrying' | 'completed' | 'failed' | 'cancelled'

const JOB_FILTER_CHIPS: { value: JobFilterValue; label: string }[] = [
  { value: 'all', label: 'Todos' },
  { value: 'running', label: 'En curso' },
  { value: 'queued', label: 'En espera' },
  { value: 'waiting_user', label: 'Revisión' },
  { value: 'retrying', label: 'Reintentando' },
  { value: 'completed', label: 'Listos' },
  { value: 'failed', label: 'Fallidos' },
  { value: 'cancelled', label: 'Cancelados' },
]

function JobRow({ job }: { job: Job }) {
  const progress = Math.max(0, Math.min(100, Number(job.progress) || 0))
  const status = friendlyJobStatus(job.status)
  const statusClassName = jobStatusClass(job.status)
  const isRunning = job.status === 'running'
  const isWaiting = job.status === 'waiting_user' || job.status === 'retrying'
  const chipClass = `status-chip ${statusClassName}${isWaiting ? ' waiting-pulse' : ''}`

  return (
    <article className="job process-job-card">
      <div className="job-top">
        <strong>{friendlyJobType(job.type)}</strong>
        <span className={chipClass}>
          {isRunning ? <i className="live-pulse" aria-hidden="true" /> : null}
          {status}
        </span>
      </div>
      <small>
        {job.status === 'failed' || job.status === 'cancelled'
          ? `${explainJobFailure(job).title}. Abre el detalle para ver qué hacer.`
          : `${friendlyJobMessage(job.message || job.stage)} · ${progress}%`}
      </small>
      <div className={`progress${isRunning ? ' running' : ''}${job.status === 'failed' ? ' failed' : ''}`}>
        <i style={{ width: `${progress}%` }} />
      </div>
      <div className="job-row-actions">
        <Link className="button ghost small" to={`/trabajos/${encodeURIComponent(job.id)}`}>
          Ver detalle
        </Link>
      </div>
    </article>
  )
}

export function Processes() {
  const jobsQuery = useJobs()
  const jobs = jobsQuery.data ?? []
  const [jobFilter, setJobFilter] = useState<JobFilterValue>('all')

  if (jobsQuery.isLoading) return <PageSkeleton label="Cargando trabajos locales" />
  if (jobsQuery.isError) return <PageError message="No pudimos cargar los trabajos locales." action={<button className="button" onClick={() => jobsQuery.refetch()}>Reintentar</button>} />

  const jobCounts: Partial<Record<JobStatus, number>> = {
    running: 0,
    queued: 0,
    waiting_user: 0,
    retrying: 0,
    completed: 0,
    failed: 0,
    cancelled: 0,
  }
  jobs.forEach((job) => {
    if (job.status in jobCounts) jobCounts[job.status] = (jobCounts[job.status] ?? 0) + 1
  })

  const filteredJobs = jobFilter === 'all' ? jobs : jobs.filter((job) => job.status === jobFilter)
  const latestJob = jobs[0]

  return (
    <div className="grid processes-layout">
      <section className="card processes-card">
        <div className="card-pad">
          <div className="side-title">
            <div>
              <div className="eyebrow">Actividad local</div>
              <h3 style={{ marginTop: 7 }}>Procesos recientes</h3>
              <p className="helper" style={{ marginTop: 5 }}>
                Sigue la actualización de fichas, la revisión de rutas, las capturas y los reportes desde un mismo lugar.
              </p>
            </div>
            <button className="button ghost small" onClick={() => jobsQuery.refetch()}>Actualizar</button>
          </div>
          <div className="checklist-filter-tabs process-filters" style={{ marginTop: 18 }}>
            {JOB_FILTER_CHIPS.map(({ value, label }) => (
              <button key={value} type="button" className={`checklist-filter-tab ${jobFilter === value ? 'active' : ''}`} aria-pressed={jobFilter === value} onClick={() => setJobFilter(value)}>
                {label} {value === 'all' ? jobs.length : (jobCounts[value] ?? 0)}
              </button>
            ))}
          </div>
          <div className="process-summary-grid">
            <div><strong>{jobCounts.running || 0}</strong><span>En curso</span><small>Trabajos activos ahora</small></div>
            <div><strong>{jobCounts.waiting_user || 0}</strong><span>Revisión</span><small>Decisiones pendientes</small></div>
            <div><strong>{jobCounts.failed || 0}</strong><span>Fallidos</span><small>Requieren diagnóstico</small></div>
            <div><strong>{latestJob ? friendlyJobStatus(latestJob.status) : '—'}</strong><span>Último estado</span><small>{latestJob ? friendlyJobType(latestJob.type) : 'Sin procesos'}</small></div>
          </div>
          <div className="job-list process-job-list">
            {filteredJobs.length ? filteredJobs.map((job) => <JobRow key={job.id} job={job} />) : <div className="empty">No hay procesos con este filtro.</div>}
          </div>
        </div>
      </section>
      <section className="card status-guide-card">
        <div className="card-pad">
          <div className="eyebrow">Lectura rápida</div>
          <h3 style={{ marginTop: 7 }}>¿Qué significa cada estado?</h3>
          <div className="status-guide-list">
            <div className="route-note"><strong>En espera:</strong> el trabajo fue aceptado y espera que la aplicación le asigne turno. No es un error.</div>
            <div className="route-note"><strong>En curso:</strong> la aplicación está ejecutando la tarea. El porcentaje y la barra muestran su avance.</div>
            <div className="route-note"><strong>Necesita tu revisión:</strong> la aplicación encontró una decisión que no debe resolver sola; abre el detalle y confirma o corrige.</div>
            <div className="route-note"><strong>Reintentando:</strong> hubo un fallo temporal y el proceso volverá a intentarlo automáticamente.</div>
            <div className="route-note"><strong>Listo:</strong> el resultado quedó guardado en este equipo y puedes revisar sus evidencias o reportes.</div>
            <div className="route-note"><strong>Fallido:</strong> el trabajo terminó sin completar todo. Abre “Ver detalle”: te explicamos qué pasó, qué hacer y te damos el botón para resolverlo. Un fallo se da por resuelto cuando el mismo proceso vuelve a terminar bien.</div>
          </div>
        </div>
      </section>
    </div>
  )
}
