import { Link } from 'react-router-dom'
import { MissingActiveFicha, PageError, PageSkeleton } from '../components/AsyncState'
import { useActivities, useDashboard, isNotFound } from '../hooks/api'
import { RouteDiscoveryAction } from '../components/RouteDiscoveryAction'
import { ActivitySelector } from '../components/ActivitySelector'

export function Activities() {
  const dashboardQuery = useDashboard()
  const dashboard = dashboardQuery.data
  const activeFichaId = dashboard?.activeFichaId
  const activitiesQuery = useActivities(activeFichaId)
  const data = activitiesQuery.data

  if (dashboardQuery.isLoading || (activeFichaId && activitiesQuery.isLoading)) return <PageSkeleton label="Cargando actividades" />
  if (dashboardQuery.isError && isNotFound(dashboardQuery.error)) {
    return <MissingActiveFicha message="Las actividades se consultan por ficha. Selecciona un curso para continuar." />
  }
  if (dashboardQuery.isError) return <PageError message="No pudimos cargar la ficha activa." action={<Link className="button" to="/fichas">Elegir una ficha</Link>} />
  if (!activeFichaId) {
    return (
      <section className="card onboarding-card">
        <div className="card-pad">
          <div className="eyebrow">Actividades</div>
          <h2 style={{ marginTop: 7 }}>Elige una ficha para continuar</h2>
          <p className="helper" style={{ marginTop: 8 }}>Las actividades se consultan por ficha. Sincroniza tus fichas y selecciona una antes de definir qué evidencias preparar.</p>
          <Link className="button primary" to="/fichas" style={{ marginTop: 18 }}>Ver fichas</Link>
        </div>
      </section>
    )
  }
  if (activitiesQuery.isError) return <PageError message="No pudimos cargar las actividades de esta ficha." action={<button className="button" onClick={() => activitiesQuery.refetch()}>Reintentar</button>} />
  if (!data) return <div className="empty">Todavía no hay una respuesta de actividades para esta ficha.</div>
  if (data.mapReady === false) {
    return (
      <section className="card onboarding-card">
        <div className="card-pad">
          <div className="eyebrow">Rutas del curso</div>
          <h2 style={{ marginTop: 7 }}>Primero busca las rutas del curso</h2>
          <p className="helper" style={{ marginTop: 8 }}>{data.discovery?.message || 'Necesitamos leer el contenido del curso para mostrarte las actividades disponibles.'}</p>
          <div className="inline" style={{ marginTop: 18 }}>
            <RouteDiscoveryAction variant="primary" />
            <Link className="button ghost" to="/checklist">Ir al checklist</Link>
          </div>
        </div>
      </section>
    )
  }

  return (
    <div className="grid">
      <ActivitySelector key={activeFichaId} data={data} fichaId={activeFichaId} fichaCode={dashboardQuery.data?.ficha?.externalId} />
    </div>
  )
}
