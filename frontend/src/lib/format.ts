import type { DashboardItem, JobStatus, JobType, SetupStatus } from '../types'

export function formatDate(value?: string) {
  return value ? new Date(value).toLocaleString('es-CO', { dateStyle: 'short', timeStyle: 'short' }) : '—'
}

export function profileName(setup?: Pick<SetupStatus, 'profile' | 'zajunaUsername'>) {
  const profile = setup?.profile
  const name = [profile?.fullName, profile?.name, [profile?.firstName, profile?.lastName].filter(Boolean).join(' ')]
    .find((value) => String(value || '').trim())
  if (name) return String(name).trim()
  const username = String(setup?.zajunaUsername || '').trim()
  return /^\d+$/.test(username) || !username ? 'Cuenta Zajuna' : username
}

export function initials(value?: string) {
  const text = String(value || '').trim()
  if (!text || /^\d+$/.test(text)) return 'ZA'
  const words = text.split(/\s+/).filter(Boolean)
  if (words.length > 1) return `${words[0][0]}${words[words.length - 1][0]}`.toUpperCase()
  return text.slice(0, 2).toUpperCase()
}

export function statusLabel(value: string) {
  return value === 'SI' ? 'Cumplida' : value === 'NO' ? 'No cumplida' : 'Pendiente'
}

export function statusClass(value: string) {
  return String(value || 'PENDIENTE').toLowerCase()
}

const JOB_TYPE_LABELS: Record<JobType, string> = {
  'sync-fichas': 'Actualizar fichas',
  'test-zajuna-connection': 'Probar conexión con Zajuna',
  'discover-course-maps': 'Revisar contenido del curso',
  'capture-checklist': 'Preparar evidencias',
  'capture-evidence': 'Guardar evidencia',
  'capture-browser': 'Preparar captura',
  'export-report': 'Generar reporte',
  backup: 'Crear copia de respaldo',
}

export function friendlyJobType(value: JobType) {
  return JOB_TYPE_LABELS[value] || 'Proceso'
}

const JOB_STATUS_LABELS: Record<JobStatus, string> = {
  queued: 'En espera',
  running: 'En curso',
  waiting_user: 'Necesita tu revisión',
  retrying: 'Reintentando',
  completed: 'Listo',
  failed: 'No se pudo completar',
  cancelled: 'Cancelado',
}

export function friendlyJobStatus(value: JobStatus) {
  return JOB_STATUS_LABELS[value] || 'En curso'
}

const JOB_STATUS_CLASS: Record<JobStatus, string> = {
  queued: 'queued',
  running: 'running',
  waiting_user: 'review',
  retrying: 'review',
  completed: 'ok',
  failed: 'error',
  cancelled: 'muted',
}

export function jobStatusClass(value: JobStatus) {
  return JOB_STATUS_CLASS[value] || 'pending'
}

export function friendlyJobMessage(value?: string) {
  const raw = String(value || 'Preparando el proceso')
  if (/selector requerido|no apareció|selector no|candidatos=|cssselector|css selector/i.test(raw)) {
    return 'Algunas secciones necesitan una revisión guiada antes de volver a capturarse.'
  }
  if (/captura incompleta/i.test(raw)) {
    return 'Revisamos la ficha, pero algunas evidencias necesitan confirmación.'
  }
  return raw
    .replace(/discover-course-maps|discover/gi, 'revisando el contenido del curso')
    .replace(/capture-checklist|capture-evidence|capture-browser|capture/gi, 'preparando evidencias')
    .replace(/sync-fichas|sync/gi, 'actualizando fichas')
    .replace(/queued/gi, 'en espera')
    .replace(/running/gi, 'en curso')
    .replace(/completed/gi, 'listo')
    .replace(/failed/gi, 'no se pudo completar')
    .replace(/waiting_user/gi, 'necesita tu revisión')
    .replace(/retrying/gi, 'reintentando')
    .replace(/cancelled/gi, 'cancelado')
    .replace(/authenticating/gi, 'iniciando sesión')
    .replace(/discovering/gi, 'revisando el contenido')
    .replace(/capturing/gi, 'preparando evidencias')
    .replace(/exporting/gi, 'generando el reporte')
}


const JOB_STAGE_LABELS: Record<string, string> = {
  queued: 'En espera',
  running: 'En curso',
  starting: 'Iniciando',
  authenticating: 'Iniciando sesión en Zajuna',
  syncing: 'Sincronizando fichas',
  discovering: 'Revisando el contenido del curso',
  capturing: 'Preparando evidencias',
  capturing_checklist: 'Preparando evidencias del checklist',
  capturing_browser: 'Preparando la captura',
  exporting: 'Generando el reporte',
  backing_up: 'Creando la copia de respaldo',
  waiting_user: 'Necesita tu revisión',
  retrying: 'Reintentando',
  completed: 'Listo',
  failed: 'No se pudo completar',
  cancelled: 'Cancelado',
}

export function friendlyJobStage(value?: string) {
  const raw = String(value || '').trim()
  if (!raw) return 'Actualización'
  const key = raw.toLowerCase().replace(/[\s-]+/g, '_')
  if (JOB_STAGE_LABELS[key]) return JOB_STAGE_LABELS[key]
  if (JOB_STAGE_LABELS[raw]) return JOB_STAGE_LABELS[raw]
  return friendlyJobMessage(raw)
}

export function reportStatusLabel(value?: string) {
  const key = String(value || '').toLowerCase()
  if (key === 'completed' || key === 'ready' || key === 'listo') return 'Listo'
  if (key === 'pending' || key === 'queued') return 'En espera'
  if (key === 'running' || key === 'processing') return 'Preparando'
  if (key === 'failed' || key === 'error') return 'No se pudo generar'
  if (key === 'cancelled') return 'Cancelado'
  return 'En revisión'
}

export interface Confidence {
  key: 'manual' | 'high' | 'review' | 'empty'
  label: string
  detail: string
}

export function confidenceFor(item: Pick<DashboardItem, 'captureConfidence' | 'confidence' | 'evidenceCount'>): Confidence {
  const value = String(item.captureConfidence || item.confidence || '').toLowerCase()
  if (value.includes('manual') || value.includes('confirmed') || value.includes('confirmada')) {
    return { key: 'manual', label: 'Confirmada', detail: 'Ruta revisada por ti' }
  }
  if (value.includes('high') || value.includes('alta')) {
    return { key: 'high', label: 'Coincidencia alta', detail: 'La ruta coincide con lo esperado' }
  }
  if (value.includes('medium') || value.includes('review') || value.includes('revis')) {
    return { key: 'review', label: 'Por revisar', detail: 'Confirma que la captura corresponde' }
  }
  return item.evidenceCount
    ? { key: 'review', label: 'Por revisar', detail: 'Confirma esta evidencia antes de usarla' }
    : { key: 'empty', label: 'Sin evidencia', detail: 'Todavía no hay un archivo asociado' }
}

export function confidenceSummary(items: DashboardItem[]) {
  return items.reduce(
    (summary, item) => {
      const key = confidenceFor(item).key
      if (key === 'manual' || key === 'high') summary.high++
      else if (key === 'review') summary.review++
      else summary.empty++
      return summary
    },
    { high: 0, review: 0, empty: 0 },
  )
}

export const ROUTE_GROUP_LABELS: Record<string, string> = {
  cronograma_general: 'Cronograma general',
  cronograma_vigente: 'Cronograma de la fase vigente',
  perfil_instructor: 'Perfil del instructor',
  menu_curso: 'Menú principal del curso',
  subsecciones_fase: 'Subsecciones de la fase',
  subsecciones_mensuales: 'Subsecciones por mes',
  foros: 'Foros técnicos',
  anuncios_fase: 'Anuncios de la fase',
  anuncios_semanales: 'Anuncios semanales',
  conclusion_foros: 'Conclusiones de foros',
  sesiones_semanales: 'Sesiones y resúmenes',
  sesiones_linea: 'Sesiones en línea',
  documentos_retencion: 'Seguimiento y cierre',
  seguimiento_documentos: 'Seguimiento documental',
  seguimiento_evaluacion: 'Seguimiento y evaluación',
  evidencias_aprendizaje: 'Evidencias de aprendizaje',
  calificaciones: 'Calificaciones del curso',
  disponibilidad: 'Disponibilidad y acceso',
  configuracion: 'Configuración del curso',
  netiqueta: 'Netiqueta',
}

export const ROUTE_KIND_LABELS: Record<string, string> = {
  course: 'menú del curso',
  page: 'página informativa',
  phase: 'subsección de fase',
  forum: 'foro técnico',
  assign: 'actividad',
}

export function routeStatusLabel(value?: string) {
  return value === 'confirmed' ? 'Confirmada' : value === 'correction' ? 'Para corregir' : 'Por revisar'
}

export function routeStatusClass(value?: string) {
  return value === 'confirmed' ? 'ok' : value === 'correction' ? 'warn' : 'review'
}
