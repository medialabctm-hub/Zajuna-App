import { useEffect, useState, type FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { backupDownloadUrl } from '../api/client'
import { PageError, PageSkeleton } from '../components/AsyncState'
import { useAppInfo, useResetApp, useBackups, useCleanupBackups, useCreateBackup, useDashboard, useDeleteBackup, useFichas, useRestoreBackup, useClearEvidences, useSaveSettings, useSaveSetup, useSettings, useSetupStatus, useZajunaConnectionTest } from '../hooks/api'
import { useToast } from '../hooks/useToast'
import { connectionStatus } from '../lib/connectionStatus'
import { friendlyError } from '../lib/friendlyError'
import type { AppSettings } from '../types'

type SettingsTabId = 'account' | 'capture' | 'storage' | 'backup' | 'notifications' | 'about'
type DocumentType = 'CC' | 'TI' | 'CE'

const TABS: Array<{ id: SettingsTabId; label: string }> = [
  { id: 'account', label: 'Cuenta Zajuna' },
  { id: 'capture', label: 'Capturas' },
  { id: 'storage', label: 'Almacenamiento' },
  { id: 'backup', label: 'Copias de seguridad' },
  { id: 'notifications', label: 'Notificaciones' },
  { id: 'about', label: 'Acerca de' },
]

const DEFAULT_SETTINGS: AppSettings = {
  session: { autoRenew: true },
  capture: { fullPage: true, reuseSession: true, motion: true },
  notifications: { jobCompleted: true, needsReview: true },
  storage: { retentionKeep: 5, retentionDays: 30 },
}

function formatBytes(value: number) {
  if (!value) return '0 B'
  if (value < 1024 * 1024) return `${Math.max(1, Math.round(value / 1024))} KB`
  return `${(value / (1024 * 1024)).toFixed(1)} MB`
}

export function Settings() {
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const activeTab: SettingsTabId = TABS.some((tab) => tab.id === tabParam) ? (tabParam as SettingsTabId) : 'account'

  const setupQuery = useSetupStatus()
  const setup = setupQuery.data
  const { data: fichas } = useFichas()
  const { data: dashboard } = useDashboard()
  const backupsQuery = useBackups()
  const backups = backupsQuery.data
  const settingsQuery = useSettings()
  const settings = settingsQuery.data
  const saveSetup = useSaveSetup()
  const clearEvidences = useClearEvidences()
  const saveSettings = useSaveSettings()
  const createBackup = useCreateBackup()
  const deleteBackup = useDeleteBackup()
  const cleanupBackups = useCleanupBackups()
  const restoreBackup = useRestoreBackup()
  const appInfoQuery = useAppInfo()
  const resetApp = useResetApp()
  const connectionTest = useZajunaConnectionTest(setup?.zajunaUsername)
  const toast = useToast()
  const [resetBackupFirst, setResetBackupFirst] = useState(true)
  const [resetForgetCredentials, setResetForgetCredentials] = useState(false)
  const [resetDone, setResetDone] = useState<{ restarting: boolean; backupName?: string } | null>(null)

  const [documentType, setDocumentType] = useState<DocumentType>(setup?.zajunaDocumentType || 'CC')
  const [username, setUsername] = useState(setup?.zajunaUsername || '')
  const [password, setPassword] = useState('')
  const [preferences, setPreferences] = useState<AppSettings>(DEFAULT_SETTINGS)

  useEffect(() => {
    if (setup) {
      setDocumentType(setup.zajunaDocumentType || 'CC')
      setUsername(setup.zajunaUsername || '')
    }
  }, [setup])

  useEffect(() => {
    if (settings) setPreferences(settings)
  }, [settings])

  if (resetDone) {
    return (
      <section className="card onboarding-card" role="status" aria-live="polite">
        <div className="card-pad">
          <div className="eyebrow">Restablecimiento en curso</div>
          <h2 style={{ marginTop: 7 }}>Estamos dejando Zajuna App como recién instalada</h2>
          <p className="helper" style={{ marginTop: 8 }}>
            {resetDone.restarting
              ? 'La aplicación se reinicia sola y abrirá una pestaña nueva en unos segundos, con el checklist en 0 y sin evidencias, actividades ni trabajos anteriores. Ya puedes cerrar esta pestaña.'
              : 'Cierra Zajuna App y vuelve a abrirla: al iniciar se borrarán los datos anteriores y verás la aplicación como recién instalada.'}
          </p>
          {resetDone.backupName ? (
            <p className="helper" style={{ marginTop: 8 }}>
              Guardamos una copia de seguridad previa ({resetDone.backupName}). Puedes restaurarla desde Configuración › Copias de seguridad.
            </p>
          ) : null}
        </div>
      </section>
    )
  }

  if (setupQuery.isLoading || settingsQuery.isLoading) return <PageSkeleton label="Cargando configuración local" />
  if (setupQuery.isError || settingsQuery.isError) return <PageError message="No pudimos cargar las preferencias locales." action={<button className="button" onClick={() => { setupQuery.refetch(); settingsQuery.refetch() }}>Reintentar</button>} />

  function setTab(tab: SettingsTabId) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        next.set('tab', tab)
        return next
      },
      { replace: true },
    )
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    try {
      await saveSetup.mutateAsync({ zajunaUsername: username.trim(), zajunaDocumentType: documentType, zajunaPassword: password })
      connectionTest.forget()
      toast('Credenciales guardadas. Pulsa “Probar conexión” para confirmar que Zajuna las acepta.')
      setPassword('')
    } catch (err) {
      const message = err instanceof Error ? err.message : 'No se pudo guardar la conexión.'
      toast(friendlyError(message), true)
    }
  }

  async function handleTestConnection() {
    try {
      await connectionTest.start()
      toast('Probando la conexión con Zajuna. El resultado aparece aquí en unos segundos.')
    } catch (err) {
      const message = err instanceof Error ? err.message : 'No se pudo iniciar la prueba de conexión.'
      toast(friendlyError(message), true)
    }
  }

  async function handleBackup() {
    try {
      await createBackup.mutateAsync()
      toast('Copia de seguridad creada correctamente.')
    } catch (err) {
      const message = err instanceof Error ? err.message : 'No se pudo crear la copia de seguridad.'
      toast(friendlyError(message), true)
    }
  }

  async function handleDeleteBackup(name: string) {
    if (!window.confirm(`¿Eliminar la copia ${name}? Esta acción no se puede deshacer.`)) return
    try {
      await deleteBackup.mutateAsync(name)
      toast('Copia eliminada correctamente.')
    } catch (err) {
      const message = err instanceof Error ? err.message : 'No se pudo eliminar la copia de seguridad.'
      toast(friendlyError(message), true)
    }
  }

  async function handleRestoreBackup(name: string) {
    if (!window.confirm(`Se creará una copia de seguridad de protección y se preparará ${name} para restaurarla al reiniciar. ¿Continuar?`)) return
    try {
      const result = await restoreBackup.mutateAsync(name)
      toast(`Restauración preparada. Reinicia la aplicación para aplicar la copia; respaldo de protección: ${result.safetyBackup}.`)
    } catch (err) {
      const message = err instanceof Error ? err.message : 'No se pudo preparar la restauración.'
      toast(friendlyError(message), true)
    }
  }

  async function handleCleanupBackups() {
    const { retentionKeep, retentionDays } = preferences.storage
    if (!window.confirm(`Se conservarán las ${retentionKeep} copias más recientes y se eliminarán solo las de más de ${retentionDays} días. ¿Continuar?`)) return
    try {
      const result = await cleanupBackups.mutateAsync({ keep: retentionKeep, olderThanDays: retentionDays })
      toast(result.deleted.length ? `Se eliminaron ${result.deleted.length} copias antiguas.` : 'No hay copias antiguas para limpiar.')
    } catch (err) {
      const message = err instanceof Error ? err.message : 'No se pudieron limpiar las copias antiguas.'
      toast(friendlyError(message), true)
    }
  }

  function handleResetApp() {
    const summary = [
      'Se borrarán de este equipo: el avance del checklist, las evidencias, las actividades seleccionadas, las rutas revisadas, los reportes, los trabajos y los avisos.',
      resetBackupFirst ? 'Antes se creará una copia de seguridad para poder volver atrás.' : 'No se creará copia de seguridad: no podrás recuperar estos datos.',
      resetForgetCredentials ? 'También se olvidará tu contraseña de Zajuna.' : 'Tu cuenta de Zajuna tendrá que configurarse de nuevo al abrir.',
      'La aplicación se reiniciará. ¿Continuar?',
    ].join('\n\n')
    if (!window.confirm(summary)) return
    resetApp.mutate(
      { backupFirst: resetBackupFirst, forgetCredentials: resetForgetCredentials },
      {
        onSuccess: (result) => setResetDone({ restarting: result.restarting, backupName: result.backupName }),
        onError: (error) => toast(friendlyError(error.message), true),
      },
    )
  }

  async function updatePreferences(next: AppSettings) {
    const previous = preferences
    setPreferences(next)
    try {
      await saveSettings.mutateAsync(next)
    } catch (err) {
      setPreferences(previous)
      const message = err instanceof Error ? err.message : 'No se pudieron guardar las preferencias.'
      toast(friendlyError(message), true)
    }
  }

  function togglePreference(section: 'session' | 'capture' | 'notifications', key: string) {
    const current = preferences[section] as Record<string, boolean>
    updatePreferences({ ...preferences, [section]: { ...current, [key]: !current[key] } } as AppSettings)
  }

  function Toggle({ pressed, label, onClick }: { pressed: boolean; label: string; onClick: () => void }) {
    return (
      <button className={`toggle settings-toggle${pressed ? ' active' : ''}`} type="button" role="switch" aria-checked={pressed} aria-label={label} onClick={onClick} disabled={saveSettings.isPending}>
        <i />
      </button>
    )
  }

  const evidenceCount = (dashboard?.items ?? []).reduce((sum, item) => sum + (Number(item.evidenceCount) || 0), 0)
  const connection = connectionStatus(setup, connectionTest.job)

  return (
    <div className="settings-layout">
      <div className="settings-tabs" role="tablist">
        {TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            className={`settings-tab${activeTab === tab.id ? ' active' : ''}`}
            role="tab"
            aria-selected={activeTab === tab.id}
            aria-controls={`settings-panel-${tab.id}`}
            id={`settings-tab-${tab.id}`}
            tabIndex={activeTab === tab.id ? 0 : -1}
            onKeyDown={(event) => {
              if (!['ArrowRight', 'ArrowLeft', 'Home', 'End'].includes(event.key)) return
              event.preventDefault()
              const currentIndex = TABS.findIndex((entry) => entry.id === tab.id)
              const nextIndex = event.key === 'Home' ? 0 : event.key === 'End' ? TABS.length - 1 : (currentIndex + (event.key === 'ArrowRight' ? 1 : -1) + TABS.length) % TABS.length
              const nextTab = TABS[nextIndex]
              setTab(nextTab.id)
              document.getElementById(`settings-tab-${nextTab.id}`)?.focus()
            }}
            onClick={() => setTab(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === 'account' && (
        <div id="settings-panel-account" className="settings-grid" role="tabpanel" aria-labelledby="settings-tab-account" tabIndex={0}>
          <section className="card settings-section">
            <div className="settings-section-head">
              <h3>Credenciales de Zajuna</h3>
              <p className="helper">Zajuna App no tiene una cuenta propia. Estas credenciales solo abren la sesión contra el campus.</p>
            </div>
            <div className="card-pad">
              <form onSubmit={handleSubmit}>
                <div className="field">
                  <label htmlFor="settings-document-type">Tipo de documento</label>
                  <select
                    id="settings-document-type"
                    name="documentType"
                    value={documentType}
                    onChange={(event) => setDocumentType(event.target.value as DocumentType)}
                  >
                    <option value="CC">Cédula de ciudadanía (CC)</option>
                    <option value="TI">Tarjeta de identidad (TI)</option>
                    <option value="CE">Cédula de extranjería (CE)</option>
                  </select>
                </div>
                <div className="field">
                  <label htmlFor="settings-username">Número de documento</label>
                  <input
                    id="settings-username"
                    name="username"
                    inputMode="numeric"
                    autoComplete="username"
                    value={username}
                    onChange={(event) => setUsername(event.target.value)}
                    required
                  />
                </div>
                <div className="field">
                  <label htmlFor="settings-password">Nueva contraseña de Zajuna</label>
                  <input
                    id="settings-password"
                    name="password"
                    type="password"
                    autoComplete="new-password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    required
                  />
                </div>
                <div className="form-actions">
                  <button className="button" type="submit" disabled={saveSetup.isPending}>
                    {saveSetup.isPending ? 'Guardando…' : 'Guardar conexión'}
                  </button>
                </div>
              </form>
            </div>
          </section>
          <div className="grid">
            <section className="card settings-section">
              <div className="settings-section-head">
                <h3>Conexión y sesión</h3>
                <p className="helper">Comprueba que Zajuna acepta tus credenciales antes de sincronizar o capturar.</p>
              </div>
              <div className="settings-row" aria-live="polite">
                <div>
                  <strong>Estado de conexión</strong>
                  <span>{connection.detail}</span>
                  {connectionTest.job ? <Link className="settings-inline-link" to={`/trabajos/${encodeURIComponent(connectionTest.job.id)}`}>Ver detalle de la prueba</Link> : null}
                </div>
                <span className={`status-chip ${connection.tone}`}>{connection.label}</span>
              </div>
              <div className="card-pad">
                <button
                  className="button"
                  type="button"
                  onClick={handleTestConnection}
                  disabled={!setup?.setupComplete || connection.testing || connectionTest.isStarting}
                >
                  {connection.testing || connectionTest.isStarting ? 'Probando conexión…' : 'Probar conexión'}
                </button>
              </div>
              <div className="settings-row">
                <div>
                  <strong>Renovar la sesión automáticamente</strong>
                  <span>Si Zajuna cierra la sesión mientras se preparan evidencias, se inicia sesión de nuevo y esa evidencia se reintenta una vez.</span>
                </div>
                <Toggle pressed={preferences.session.autoRenew} label="Renovar la sesión automáticamente" onClick={() => togglePreference('session', 'autoRenew')} />
              </div>
            </section>
          </div>
        </div>
      )}

      {activeTab === 'capture' && (
        <div id="settings-panel-capture" className="settings-grid" role="tabpanel" aria-labelledby="settings-tab-capture" tabIndex={0}>
          <section className="card settings-section">
            <div className="settings-section-head">
              <h3>Preferencias de captura</h3>
              <p className="helper">Se aplican la próxima vez que prepares evidencias. Las evidencias ya guardadas no cambian.</p>
            </div>
            <div className="settings-row">
              <div>
                <strong>Página completa en perfil y cronogramas</strong>
                <span>El perfil del instructor y los cronogramas se capturan con toda la página. Si lo desactivas, solo se guarda el bloque detectado.</span>
              </div>
              <Toggle pressed={preferences.capture.fullPage} label="Capturar página completa en perfil y cronogramas" onClick={() => togglePreference('capture', 'fullPage')} />
            </div>
            <div className="settings-row">
              <div>
                <strong>Reutilizar la sesión entre evidencias</strong>
                <span>Una misma sesión de Zajuna sirve para varias evidencias. Si lo desactivas, cada evidencia inicia sesión por separado: es más lento y Zajuna puede limitar los accesos.</span>
              </div>
              <Toggle pressed={preferences.capture.reuseSession} label="Reutilizar la sesión entre evidencias" onClick={() => togglePreference('capture', 'reuseSession')} />
            </div>
            <div className="settings-row">
              <div>
                <strong>Animaciones de carga</strong>
                <span>Muestra una animación mientras llegan los datos. Solo afecta a esta interfaz, no a las capturas.</span>
              </div>
              <Toggle pressed={preferences.capture.motion} label="Mostrar animaciones de carga" onClick={() => togglePreference('capture', 'motion')} />
            </div>
          </section>
          <section className="card settings-section">
            <div className="settings-section-head">
              <h3>Cómo se capturan</h3>
              <p className="helper">Reglas fijas que no dependen de estas preferencias.</p>
            </div>
            <div className="card-pad">
              <div className="route-note">
                <strong>Cronogramas:</strong> se abren con 2560 px de ancho para que la tabla completa quede en la imagen.
              </div>
              <div className="route-note">
                <strong>Fechas:</strong> se toma la fecha del equipo al crear cada evidencia.
              </div>
            </div>
          </section>
        </div>
      )}

      {activeTab === 'storage' && (
        <div id="settings-panel-storage" className="settings-grid" role="tabpanel" aria-labelledby="settings-tab-storage" tabIndex={0}>
          <section className="card settings-section">
            <div className="settings-section-head">
              <h3>Datos de esta instalación</h3>
              <p className="helper">Tus fichas, evidencias y reportes permanecen en este equipo.</p>
            </div>
            <div className="settings-row">
              <div>
                <strong>Fichas</strong>
                <span>Información sincronizada disponible localmente.</span>
              </div>
              <span className="status-chip ok">{fichas?.length ?? 0}</span>
            </div>
            <div className="settings-row">
              <div>
                <strong>Evidencias relacionadas</strong>
                <span>Archivos que pueden previsualizarse desde Evidencias.</span>
              </div>
              <span className="status-chip ok">{evidenceCount}</span>
            </div>
            <div className="settings-row">
              <div>
                <strong>Credenciales</strong>
                <span>No se incluyen en PDF ni respaldos.</span>
              </div>
              <span className="status-chip ok">Protegidas</span>
            </div>
          </section>
          <section className="card settings-section">
            <div className="settings-section-head">
              <h3>Ubicación local</h3>
              <p className="helper">La carpeta se administra desde el instalador y el equipo del usuario.</p>
            </div>
            <div className="card-pad">
              <div className="route-note">Almacenamiento local de evidencias y reportes activo.</div>
              <div className="settings-row" style={{ marginTop: 16 }}>
                <div>
                  <strong>Borrar evidencias locales</strong>
                  <span>Actualizar la aplicación no borra evidencias. Usa esta acción solo si quieres empezar de cero en este equipo.</span>
                </div>
                <button
                  className="button ghost danger-outline"
                  type="button"
                  disabled={clearEvidences.isPending}
                  onClick={() => {
                    if (!window.confirm('¿Borrar todas las evidencias guardadas en este equipo? Esta acción no se puede deshacer.')) return
                    clearEvidences.mutate(undefined, {
                      onSuccess: () => toast('Evidencias locales borradas.'),
                      onError: (error) => toast(friendlyError(error instanceof Error ? error.message : String(error)), true),
                    })
                  }}
                >
                  {clearEvidences.isPending ? 'Borrando…' : 'Borrar evidencias'}
                </button>
              </div>
            </div>
          </section>
          {appInfoQuery.data?.resetPending ? (
            <div className="activity-status warn" role="alert">
              Hay un restablecimiento pendiente que no se pudo aplicar porque otro proceso tenía abiertos los datos. Cierra
              Zajuna App por completo (también desde la bandeja del sistema) y vuelve a abrirla.
            </div>
          ) : null}
          <section className="card settings-section">
            <div className="settings-section-head">
              <h3>Restablecer la aplicación</h3>
              <p className="helper">
                Deja Zajuna App como recién instalada: checklist en 0 %, sin evidencias, actividades, trabajos ni avisos
                anteriores. Instalar una versión nueva ya lo hace automáticamente; usa esta opción para empezar de cero sin reinstalar.
              </p>
            </div>
            <div className="card-pad">
              <label className="settings-row" style={{ cursor: 'pointer' }}>
                <div>
                  <strong>Crear una copia de seguridad antes</strong>
                  <span>Recomendado. Podrás restaurarla desde Copias de seguridad si cambias de opinión.</span>
                </div>
                <input type="checkbox" checked={resetBackupFirst} onChange={(event) => setResetBackupFirst(event.target.checked)} />
              </label>
              <label className="settings-row" style={{ cursor: 'pointer' }}>
                <div>
                  <strong>Olvidar también mi contraseña de Zajuna</strong>
                  <span>Útil si otra persona va a usar este equipo.</span>
                </div>
                <input type="checkbox" checked={resetForgetCredentials} onChange={(event) => setResetForgetCredentials(event.target.checked)} />
              </label>
              <div className="settings-row" style={{ marginTop: 8 }}>
                <div>
                  <strong>Borrar datos y reiniciar</strong>
                  <span>Las copias de seguridad existentes se conservan.</span>
                </div>
                <button className="button danger" type="button" onClick={handleResetApp} disabled={resetApp.isPending}>
                  {resetApp.isPending ? (resetBackupFirst ? 'Creando copia…' : 'Restableciendo…') : 'Restablecer datos'}
                </button>
              </div>
            </div>
          </section>
        </div>
      )}

      {activeTab === 'backup' && (
        <div id="settings-panel-backup" className="settings-grid" role="tabpanel" aria-labelledby="settings-tab-backup" tabIndex={0}>
          <section className="card settings-section">
            <div className="settings-section-head">
              <h3>Copias de seguridad</h3>
              <p className="helper">Guarda una copia local para recuperar la base y los archivos si cambias de equipo.</p>
            </div>
            <div className="card-pad">
              <button className="button" type="button" onClick={handleBackup} disabled={createBackup.isPending}>
                {createBackup.isPending ? 'Creando copia…' : 'Crear copia ahora'}
              </button>
              <button className="button ghost" type="button" onClick={handleCleanupBackups} disabled={cleanupBackups.isPending} style={{ marginLeft: 8 }}>
                {cleanupBackups.isPending ? 'Limpiando…' : 'Limpiar antiguas'}
              </button>
              <div className="route-note" style={{ marginTop: 14 }}>
                Las credenciales de Zajuna no se incluyen en el archivo.
              </div>
            </div>
            <div className="settings-row">
              <div>
                <strong>Copias recientes a conservar</strong>
                <span>“Limpiar antiguas” nunca borra las copias más recientes que este número.</span>
              </div>
              <input
                className="retention-input"
                type="number"
                min={1}
                max={1000}
                value={preferences.storage.retentionKeep}
                aria-label="Copias recientes a conservar"
                onChange={(event) => setPreferences({ ...preferences, storage: { ...preferences.storage, retentionKeep: Math.min(1000, Math.max(1, Number(event.target.value) || 1)) } })}
                onBlur={() => updatePreferences(preferences)}
                disabled={saveSettings.isPending}
              />
            </div>
            <div className="settings-row">
              <div>
                <strong>Antigüedad mínima para limpiar</strong>
                <span>“Limpiar antiguas” solo borra copias con más días que este número. No hay limpieza automática.</span>
              </div>
              <div className="retention-input-suffix">
                <input
                  className="retention-input"
                  type="number"
                  min={1}
                  max={3650}
                  value={preferences.storage.retentionDays}
                  aria-label="Antigüedad mínima de copias en días"
                  onChange={(event) => setPreferences({ ...preferences, storage: { ...preferences.storage, retentionDays: Math.min(3650, Math.max(1, Number(event.target.value) || 1)) } })}
                  onBlur={() => updatePreferences(preferences)}
                  disabled={saveSettings.isPending}
                />
                <span>días</span>
              </div>
            </div>
            <div className="card-pad">
              <div className="backup-list">
                <strong className="eyebrow">Copias disponibles</strong>
                {backupsQuery.isError ? (
                  <p className="helper" style={{ marginTop: 10 }}>No pudimos cargar las copias de seguridad.</p>
                ) : backups?.length ? backups.slice(0, 5).map((backup) => (
                  <div className="backup-row" key={backup.name}>
                    <span><b>{backup.name}</b><small>{formatBytes(backup.sizeBytes)} · {new Date(backup.createdAt).toLocaleString('es-CO')}</small></span>
                    <span className="backup-actions">
                      <a className="button ghost small" href={backupDownloadUrl(backup.name)} download aria-label={`Descargar ${backup.name}`}>Descargar</a>
                      <button className="button ghost small" type="button" onClick={() => handleRestoreBackup(backup.name)} disabled={restoreBackup.isPending}>Restaurar</button>
                      <button className="button ghost small danger-outline" type="button" onClick={() => handleDeleteBackup(backup.name)} disabled={deleteBackup.isPending}>Eliminar</button>
                    </span>
                  </div>
                )) : <p className="helper" style={{ marginTop: 10 }}>Todavía no hay copias publicadas.</p>}
              </div>
            </div>
          </section>
          <section className="card settings-section">
            <div className="settings-section-head">
              <h3>Qué incluye</h3>
            </div>
            <div className="settings-row">
              <div>
                <strong>Estado del Checklist</strong>
                <span>Los 62 puntos y sus decisiones.</span>
              </div>
              <span className="status-chip ok">Incluido</span>
            </div>
            <div className="settings-row">
              <div>
                <strong>Evidencias</strong>
                <span>Capturas y archivos manuales asociados.</span>
              </div>
              <span className="status-chip ok">Incluido</span>
            </div>
          </section>
        </div>
      )}

      {activeTab === 'notifications' && (
        <div id="settings-panel-notifications" className="settings-grid" role="tabpanel" aria-labelledby="settings-tab-notifications" tabIndex={0}>
          <section className="card settings-section">
            <div className="settings-section-head">
              <h3>Avisos de la aplicación</h3>
              <p className="helper">Los mensajes aparecen en este equipo y no dependen de correo o Slack.</p>
            </div>
            <div className="settings-row">
              <div>
                <strong>Trabajos terminados</strong>
                <span>Te avisamos cuando una sincronización o captura finalice.</span>
              </div>
              <Toggle pressed={preferences.notifications.jobCompleted} label="Avisar cuando un trabajo termine" onClick={() => togglePreference('notifications', 'jobCompleted')} />
            </div>
            <div className="settings-row">
              <div>
                <strong>Revisión necesaria</strong>
                <span>Te avisamos si una ruta necesita confirmación.</span>
              </div>
              <Toggle pressed={preferences.notifications.needsReview} label="Avisar cuando una ruta necesite revisión" onClick={() => togglePreference('notifications', 'needsReview')} />
            </div>
          </section>
        </div>
      )}

      {activeTab === 'about' && (
        <div id="settings-panel-about" className="settings-grid" role="tabpanel" aria-labelledby="settings-tab-about" tabIndex={0}>
          <section className="card settings-section">
            <div className="settings-section-head">
              <h3>Zajuna App</h3>
              <p className="helper">Herramienta local para revisar fichas y preparar evidencias.</p>
            </div>
            <div className="settings-row">
              <div>
                <strong>Versión instalada</strong>
                <span>Compárala con la última publicada si algo no se ve como esperas.</span>
              </div>
              <span className="status-chip ok">{appInfoQuery.data?.version || '—'}</span>
            </div>
            <div className="settings-row">
              <div>
                <strong>Carpeta de datos</strong>
                <span className="mono" style={{ wordBreak: 'break-all' }}>{appInfoQuery.data?.dataDir || '—'}</span>
              </div>
            </div>
            <div className="settings-row">
              <div>
                <strong>Procesamiento</strong>
                <span>Trabajos ejecutados en este equipo.</span>
              </div>
              <span className="status-chip ok">Local</span>
            </div>
            <div className="settings-row">
              <div>
                <strong>Interfaz</strong>
                <span>Se abre en tu navegador desde este mismo equipo.</span>
              </div>
              <span className="status-chip ok">Activa</span>
            </div>
          </section>
        </div>
      )}
    </div>
  )
}
