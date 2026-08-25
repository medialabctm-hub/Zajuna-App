import { useEffect, useRef, useState } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { Topbar } from './Topbar'
import { findNavItem } from '../lib/nav'

export function AppShell() {
  const location = useLocation()
  const navItem = findNavItem(location.pathname)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const menuButtonRef = useRef<HTMLButtonElement>(null)

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

  return (
    <div className="app-layout">
      <a className="skip-link" href="#dashboard-main">Saltar al contenido</a>
      <Sidebar open={mobileNavOpen} onClose={() => { setMobileNavOpen(false); menuButtonRef.current?.focus() }} />
      {mobileNavOpen && <button className="mobile-nav-backdrop" type="button" aria-label="Cerrar navegación" onClick={() => { setMobileNavOpen(false); menuButtonRef.current?.focus() }} />}
      <div className="app-content">
        <Topbar mobileMenuOpen={mobileNavOpen} onToggleMobileMenu={() => setMobileNavOpen((open) => !open)} menuButtonRef={menuButtonRef} />
        <main id="dashboard-main" className="shell app-main" aria-labelledby="dashboard-title" tabIndex={-1}>
          <h1 id="dashboard-title" className="sr-only">
            Espacio de trabajo de Zajuna App
          </h1>
          {navItem?.showGenericHeader && (
            <section className="page-head">
              <div>
                <div className="eyebrow">{navItem.eyebrow}</div>
                <h1>{navItem.label}</h1>
                <p>{navItem.description}</p>
              </div>
            </section>
          )}
          <div id="workspace">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
