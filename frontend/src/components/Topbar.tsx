import { type RefObject } from 'react'
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { Icon } from './Icon'
import { findNavItem } from '../lib/nav'
import { useDashboard, useJobs, useNotifications, useSetupStatus } from '../hooks/api'
import { initials, profileName } from '../lib/format'

const SETTINGS_TAB_LABELS: Record<string, string> = {
  account: 'Cuenta Zajuna',
  capture: 'Capturas',
  storage: 'Almacenamiento',
  backup: 'Copias de seguridad',
  notifications: 'Notificaciones',
  about: 'Acerca de',
}

interface TopbarProps {
  mobileMenuOpen: boolean
  onToggleMobileMenu: () => void
  menuButtonRef: RefObject<HTMLButtonElement | null>
}

export function Topbar({ mobileMenuOpen, onToggleMobileMenu, menuButtonRef }: TopbarProps) {
  const location = useLocation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const navItem = findNavItem(location.pathname)
  const { data: jobs } = useJobs()
  const { data: setup } = useSetupStatus()
  const { data: dashboard } = useDashboard()
  const { data: notifications } = useNotifications()

  const needsReview = (jobs || []).some((job) => job.status === 'waiting_user')
  const unreadNotifications = (notifications || []).filter((item) => !item.readAt).length

  let mid: string | null = null
  // Every page that shows data of the active ficha says which one it is.
  const fichaScoped = navItem?.group === 'Operación' && !['/resumen', '/fichas'].includes(navItem?.path || '')
  if (fichaScoped && dashboard?.ficha) {
    mid = `Ficha ${dashboard.ficha.externalId}`
  } else if (location.pathname.startsWith('/configuracion')) {
    const tab = searchParams.get('tab') || 'account'
    mid = SETTINGS_TAB_LABELS[tab] || null
  }

  const accountName = profileName(setup)
  const accountLabel = `Cuenta · ${accountName}`

  return (
    <header className="content-header">
      <button
        ref={menuButtonRef}
        className="mobile-nav-toggle"
        type="button"
        aria-label={mobileMenuOpen ? 'Cerrar navegación' : 'Abrir navegación'}
        aria-expanded={mobileMenuOpen}
        aria-controls="primary-nav"
        onClick={onToggleMobileMenu}
      >
        <span aria-hidden="true">☰</span>
      </button>
      <nav className="breadcrumb" aria-label="Ubicación actual">
        {navItem?.path === '/resumen' ? (
          <span>{navItem?.group || 'Operación'}</span>
        ) : (
          <Link className="breadcrumb-link" to="/resumen">{navItem?.group || 'Operación'}</Link>
        )}
        <span aria-hidden="true">/</span>
        {mid && (
          <>
            {fichaScoped ? (
              <Link className="mid breadcrumb-link" to="/fichas">{mid}</Link>
            ) : (
              <span className="mid">{mid}</span>
            )}
            <span aria-hidden="true">/</span>
          </>
        )}
        <span className="current" aria-current="page">{navItem?.label || 'Resumen'}</span>
      </nav>
      <div className="header-actions">
        <button className="header-icon" type="button" aria-label={unreadNotifications ? `Notificaciones · ${unreadNotifications} sin leer` : needsReview ? 'Notificaciones · hay trabajos por revisar' : 'Notificaciones'} onClick={() => navigate('/notificaciones')}>
          <Icon name="bell" size={15} />
          <i id="notif-alert-dot" hidden={!unreadNotifications && !needsReview} aria-hidden="true" />
        </button>
        <span className="header-avatar" title={accountName} aria-label={accountName}>{initials(accountName)}</span>
      </div>
      <span className="sr-only">{accountLabel}</span>
    </header>
  )
}
