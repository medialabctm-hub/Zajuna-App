import { Link } from 'react-router-dom'
import { PageError, PageSkeleton } from '../components/AsyncState'
import { Icon } from '../components/Icon'
import { useMarkAllNotificationsRead, useMarkNotificationRead, useNotifications } from '../hooks/api'
import { useToast } from '../hooks/useToast'
import { friendlyError } from '../lib/friendlyError'
import { formatDate } from '../lib/format'

export function Notifications() {
  const query = useNotifications()
  const markRead = useMarkNotificationRead()
  const markAllRead = useMarkAllNotificationsRead()
  const toast = useToast()

  if (query.isLoading) return <PageSkeleton label="Cargando notificaciones locales" />
  if (query.isError || !query.data) return <PageError message="No pudimos cargar el centro de notificaciones." action={<button className="button ghost small" onClick={() => query.refetch()}>Reintentar</button>} />

  const unread = query.data.filter((item) => !item.readAt).length

  return (
    <div className="notifications-layout">
      <section className="card">
        <div className="card-pad">
          <div className="section-toolbar">
            <div>
              <div className="eyebrow">Bandeja de avisos</div>
              <h3 style={{ marginTop: 7 }}>Avisos recientes</h3>
              <p className="helper">Avisos generados por trabajos y diagnósticos en este equipo.</p>
            </div>
            <button className="button ghost small" type="button" onClick={() => markAllRead.mutate(undefined, { onError: (error) => toast(friendlyError(error.message), true) })} disabled={!unread || markAllRead.isPending}>
              {markAllRead.isPending ? 'Guardando…' : 'Marcar todo leído'}
            </button>
          </div>

          {query.data.length === 0 ? (
            <div className="empty-state compact"><Icon name="bell" size={20} /><p>No hay notificaciones nuevas.</p></div>
          ) : (
            <div className="notification-list">
              {query.data.map((item) => (
                <article className={`notification-row${item.readAt ? '' : ' unread'}`} key={item.id}>
                  <span className={`notification-icon ${item.kind === 'job_failed' ? 'error' : 'ok'}`} aria-hidden="true"><Icon name={item.kind === 'job_failed' ? 'warning' : 'check'} size={15} /></span>
                  <div className="notification-copy">
                    <strong>{item.title}</strong>
                    <p>{item.message}</p>
                    <small>{formatDate(item.createdAt)}</small>
                  </div>
                  <div className="notification-actions">
                    {item.jobId ? <Link className="button ghost small" to={`/trabajos/${encodeURIComponent(item.jobId)}`}>Ver trabajo</Link> : null}
                    {!item.readAt ? <button className="button ghost small" type="button" onClick={() => markRead.mutate(item.id, { onError: (error) => toast(friendlyError(error.message), true) })} disabled={markRead.isPending}>Marcar leído</button> : <span className="status-chip ok">Leída</span>}
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
      </section>
    </div>
  )
}
