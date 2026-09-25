import { Suspense, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { Topbar } from './Topbar'
import { PageSkeleton } from './AsyncState'
import { PageErrorBoundary } from './PageErrorBoundary'
import { preloadPages } from '../pages/lazy'
import { findNavItem } from '../lib/nav'
import { useSettings } from '../hooks/api'
import { WorkflowSteps } from './WorkflowSteps'

export function AppShell() {
  const location = useLocation()
  const navItem = findNavItem(location.pathname)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const menuButtonRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    preloadPages()
  }, [])
  const settingsQuery = useSettings()

  useEffect(() => {
    setMobileNavOpen(false)
  }, [location.pathname])

  useEffect(() => {
    if (!mobileNavOpen) return
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setMobileNavOpen(false)
        menuButtonRef.current?.focus()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [mobileNavOpen])

  useEffect(() => {
    document.body.classList.toggle('mobile-nav-open', mobileNavOpen)
    return () => document.body.classList.remove('mobile-nav-open')
  }, [mobileNavOpen])

  useEffect(() => {
    const motionEnabled = settingsQuery.data?.capture.motion !== false
    document.documentElement.toggleAttribute('data-motion-off', !motionEnabled)
    return () => document.documentElement.removeAttribute('data-motion-off')
  }, [settingsQuery.data?.capture.motion])

  return (
    <div className="app-layout">
      <a className="skip-link" href="#dashboard-main">Saltar al contenido</a>
      <Sidebar open={mobileNavOpen} onClose={() => { setMobileNavOpen(false); menuButtonRef.current?.focus() }} />
      {mobileNavOpen && <button className="mobile-nav-backdrop" type="button" aria-label="Cerrar navegación" onClick={() => { setMobileNavOpen(false); menuButtonRef.current?.focus() }} />}
      <div className="app-content">
        <Topbar mobileMenuOpen={mobileNavOpen} onToggleMobileMenu={() => setMobileNavOpen((open) => !open)} menuButtonRef={menuButtonRef} />
        <main id="dashboard-main" className="shell app-main" aria-labelledby={navItem?.showGenericHeader ? 'page-title' : 'dashboard-title'} tabIndex={-1}>
          {!navItem?.showGenericHeader && (
            <h1 id="dashboard-title" className="sr-only">
              {navItem?.label || 'Espacio de trabajo de Zajuna App'}
            </h1>
          )}
          {navItem?.group === 'Operación' ? <WorkflowSteps /> : null}
          {navItem?.showGenericHeader && (
            <section className="page-head">
              <div>
                <div className="eyebrow">{navItem.eyebrow}</div>
                <h1 id="page-title" className="page-head-title">{navItem.label}</h1>
                <p>{navItem.description}</p>
              </div>
            </section>
          )}
          <div id="workspace">
            {/* Keyed by route so leaving a failed page shows the next one. */}
            <PageErrorBoundary key={location.pathname}>
              <Suspense fallback={<PageSkeleton />}>
                <Outlet />
              </Suspense>
            </PageErrorBoundary>
          </div>
        </main>
      </div>
    </div>
  )
}
