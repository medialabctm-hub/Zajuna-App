import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useCompleteFirstRun, useCourseMaps, useDiscoverCourseMaps, useFichas, useJobs, useSetupStatus, useSyncFichas } from '../hooks/api'
import { useToast } from '../hooks/useToast'
import { firstRunPhase, type FirstRunPhase } from '../lib/firstRun'
import { friendlyError } from '../lib/friendlyError'
import { PageSkeleton } from './AsyncState'

const COPY: Record<'fichas' | 'routes', { title: string; detail: string }> = {
  fichas: {
    title: 'Buscando tus fichas',
    detail: 'Estamos trayendo tus cursos desde Zajuna. Solo pasa la primera vez.',
  },
  routes: {
    title: 'Preparando las rutas de tus cursos',
    detail: 'Localizamos las secciones de cada ficha para que después trabajes con la que elijas. Solo pasa la primera vez.',
  },
}

/**
 * Pantalla de carga del primer arranque. Detrás se sincronizan las fichas y se
 * buscan las rutas de todas ellas; después cada acción (buscar rutas, capturar,
 * revisar) es solo para la ficha elegida. Solo aparece mientras el core marque
 * el primer arranque como pendiente y se puede omitir sin perder nada.
 */
export function FirstRunGate({ children }: { children: ReactNode }) {
  const { data: setup } = useSetupStatus()
  if (!setup?.firstRunPending) return <>{children}</>
  return <FirstRunFlow>{children}</FirstRunFlow>
}

function FirstRunFlow({ children }: { children: ReactNode }) {
  const toast = useToast()
  const { data: setup } = useSetupStatus()
  const complete = useCompleteFirstRun()
  const completed = useRef(false)
  const fichas = useFichas()
  const jobs = useJobs()
  const courseMaps = useCourseMaps()
  const syncFichas = useSyncFichas()
  const discover = useDiscoverCourseMaps()
  const [skipped, setSkipped] = useState(false)
  const started = useRef(false)

  const derived = firstRunPhase({
    fichas: fichas.data,
    jobs: jobs.data,
    mapCourseIds: courseMaps.data?.map((item) => item.courseId),
  })
  // Si ni siquiera se pudo encolar la búsqueda, se ofrece reintentar en vez de esperar.
  const phase: FirstRunPhase = derived === 'routes-pending' && discover.isError ? 'routes-failed' : derived
  const account = { username: setup?.zajunaUsername || '', documentType: setup?.zajunaDocumentType || 'CC' }

  useEffect(() => {
    if (phase !== 'routes-pending' || started.current || skipped) return
    started.current = true
    discover.mutate({ ...account, allFichas: true }, { onError: (error) => toast(friendlyError(error.message), true) })
    // La búsqueda se lanza una sola vez (started); account no debe relanzarla.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [phase, skipped])

  const finished = skipped || phase === 'ready'
  useEffect(() => {
    if (!finished || completed.current) return
    completed.current = true
    complete.mutate()
    // complete es estable para este propósito: se apaga la marca una sola vez.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [finished])

  if (finished) return <>{children}</>
  if (phase === 'loading') return <PageSkeleton label="Preparando tu espacio de trabajo" />

  return (
    <>
      <header className="topbar">
        <div className="brand">
          <span className="brand-mark">Z</span>
          <span>
            Zajuna Sync
            <span className="brand-subtitle">Operación local</span>
          </span>
        </div>
      </header>
      <main className="shell setup-shell">
        <section className="card setup" aria-live="polite">
          <div className="card-pad">
            <FirstRunBody
              phase={phase}
              working={syncFichas.isPending || discover.isPending}
              onRetrySync={() =>
                syncFichas.mutate(account, { onError: (error) => toast(friendlyError(error.message), true) })
              }
              onRetryRoutes={() =>
                discover.mutate({ ...account, allFichas: true }, { onError: (error) => toast(friendlyError(error.message), true) })
              }
              onSkip={() => setSkipped(true)}
            />
          </div>
        </section>
      </main>
    </>
  )
}

function FirstRunBody({
  phase,
  working,
  onRetrySync,
  onRetryRoutes,
  onSkip,
}: {
  phase: Exclude<FirstRunPhase, 'loading' | 'ready'>
  working: boolean
  onRetrySync: () => void
  onRetryRoutes: () => void
  onSkip: () => void
}) {
  if (phase === 'fichas' || phase === 'routes' || phase === 'routes-pending') {
    const copy = COPY[phase === 'fichas' ? 'fichas' : 'routes']
    return (
      <>
        <div className="eyebrow">Primer arranque</div>
        <h1>{copy.title}</h1>
        <p className="muted">{copy.detail}</p>
        <div className="progress running" role="progressbar" aria-label={copy.title} style={{ marginTop: 20 }}>
          <i style={{ width: '100%' }} />
        </div>
        <div className="form-actions">
          <button className="button ghost" type="button" onClick={onSkip}>
            Continuar sin esperar
          </button>
        </div>
      </>
    )
  }

  const failedRoutes = phase === 'routes-failed'
  return (
    <>
      <div className="eyebrow">Primer arranque</div>
      <h1>{failedRoutes ? 'No pudimos preparar las rutas' : phase === 'fichas-failed' ? 'No pudimos traer tus fichas' : 'Todavía no tienes fichas'}</h1>
      <p className="muted">
        {failedRoutes
          ? 'Puedes reintentar ahora o seguir: las rutas también se buscan después, ficha por ficha, desde Resumen.'
          : 'Revisa tu conexión y tus credenciales, y vuelve a intentar. También puedes seguir y sincronizar más tarde desde Fichas.'}
      </p>
      <div className="form-actions">
        <button className="button primary" type="button" disabled={working} onClick={failedRoutes ? onRetryRoutes : onRetrySync}>
          {working ? 'Enviando…' : failedRoutes ? 'Reintentar rutas' : 'Buscar mis fichas'}
        </button>
        <button className="button ghost" type="button" onClick={onSkip}>
          Continuar sin esperar
        </button>
      </div>
    </>
  )
}
