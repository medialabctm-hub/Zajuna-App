import { NavLink } from 'react-router-dom'
import { Icon } from './Icon'
import { OPERATION_ITEMS, SYSTEM_ITEMS } from '../lib/nav'
import { useDiagnostics, useFichas, useJobs } from '../hooks/api'

interface SidebarProps {
  open?: boolean
  onClose?: () => void
}

export function Sidebar({ open = false, onClose }: SidebarProps) {
  const { data: fichas } = useFichas()
  const { data: jobs } = useJobs()
  const diagnostics = useDiagnostics()
  const hasActiveJob = (jobs || []).some((job) => ['queued', 'running', 'waiting_user', 'retrying'].includes(job.status))
  const checks = diagnostics.data?.checks || []
  const hasError = checks.some((check) => check.status === 'error')
  const hasWarn = checks.some((check) => check.status === 'warn')
  const healthTitle = diagnostics.isError || hasError ? 'Aplicación con problemas' : 'Aplicación lista'
  const healthDetail = diagnostics.isLoading
    ? 'Comprobando…'
    : diagnostics.isError
      ? 'sin diagnóstico'
      : hasError
        ? 'con incidencias'
        : hasWarn
          ? 'con avisos'
          : 'Lista en este equipo'

  return (
    <aside id="primary-nav" className={`sidebar${open ? ' mobile-open' : ''}`} aria-label="Navegación principal">
      <div className="sidebar-brand">
        <span className="brand-mark">Z</span>
        <div>
          <strong>Zajuna Sync</strong>
          <small>Operación local</small>
        </div>
        <button className="mobile-nav-close" type="button" aria-label="Cerrar navegación" onClick={onClose}>×</button>
      </div>
      <nav className="sidebar-nav">
        <div className="sidebar-label">Operación</div>
        {OPERATION_ITEMS.map((item) => (
          <NavLink
            key={item.path}
            to={item.path}
            onClick={onClose}
            className={({ isActive }) => `sidebar-item${isActive ? ' active' : ''}`}
            aria-label={item.path === '/fichas' ? `Fichas, ${fichas?.length || 0}` : undefined}
          >
            <span className="sidebar-icon">
              <Icon name={item.icon} size={16} />
            </span>
            <span className="nav-label">{item.label}</span>
            {item.path === '/fichas' && <span className="nav-count" aria-hidden="true">{fichas?.length || 0}</span>}
            {item.path === '/trabajos' && <i id="nav-job-dot" className="live-dot" hidden={!hasActiveJob} aria-hidden="true" />}
          </NavLink>
        ))}
        <div className="sidebar-label system">Sistema</div>
        {SYSTEM_ITEMS.map((item) => (
          <NavLink key={item.path} to={item.path} onClick={onClose} className={({ isActive }) => `sidebar-item${isActive ? ' active' : ''}`}>
            <span className="sidebar-icon">
              <Icon name={item.icon} size={16} />
            </span>
            <span className="nav-label">{item.label}</span>
          </NavLink>
        ))}
      </nav>
      <div className="sidebar-footer">
        <div className={`sidebar-health${hasError || diagnostics.isError ? ' error' : hasWarn ? ' warn' : ''}`}>
          <i />
          <div>
            <strong>{healthTitle}</strong>
            <span>{healthDetail}</span>
          </div>
        </div>
      </div>
    </aside>
  )
}
