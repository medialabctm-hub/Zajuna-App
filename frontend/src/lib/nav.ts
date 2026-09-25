export interface NavItem {
  path: string
  label: string
  icon: string
  group: 'Operación' | 'Sistema'
  eyebrow: string
  description: string
  showGenericHeader: boolean
}

export const NAV_ITEMS: NavItem[] = [
  { path: '/resumen', label: 'Resumen', icon: 'overview', group: 'Operación', eyebrow: 'Operación', description: 'Consulta de forma general el avance y el estado de tu trabajo.', showGenericHeader: false },
  { path: '/fichas', label: 'Fichas', icon: 'folder', group: 'Operación', eyebrow: 'Tus fichas', description: 'Elige una ficha, revisa su estado y actualiza la información desde Zajuna.', showGenericHeader: true },
  { path: '/checklist', label: 'Checklist', icon: 'check', group: 'Operación', eyebrow: 'Checklist de la ficha', description: 'Revisa los puntos, define tus actividades y confirma las rutas que servirán como evidencia.', showGenericHeader: false },
  { path: '/actividades', label: 'Actividades', icon: 'spark', group: 'Operación', eyebrow: 'Actividades del instructor', description: 'Selecciona las actividades que corresponden a tu trabajo antes de preparar las evidencias.', showGenericHeader: true },
  { path: '/evidencias', label: 'Evidencias', icon: 'image', group: 'Operación', eyebrow: 'Tus evidencias', description: 'Consulta, previsualiza y agrega archivos sin repetir información en el reporte.', showGenericHeader: true },
  { path: '/revision', label: 'Revisión', icon: 'shield', group: 'Operación', eyebrow: 'Paso 5 · Revisión', description: 'Aprueba las evidencias correctas y corrige solo las que tienen problemas.', showGenericHeader: true },
  { path: '/trabajos', label: 'Trabajos', icon: 'activity', group: 'Operación', eyebrow: 'Actividad reciente', description: 'Consulta qué ha hecho la aplicación y recupera cualquier proceso que necesite atención.', showGenericHeader: true },
  { path: '/reportes', label: 'Reportes', icon: 'report', group: 'Operación', eyebrow: 'Tus entregables', description: 'Genera el PDF final y conserva una copia local de tu trabajo.', showGenericHeader: true },
  { path: '/configuracion', label: 'Configuración', icon: 'settings', group: 'Sistema', eyebrow: 'Preferencias', description: 'Actualiza la conexión de Zajuna y mantén tus datos protegidos en este equipo.', showGenericHeader: true },
  { path: '/diagnostico', label: 'Diagnóstico', icon: 'help', group: 'Sistema', eyebrow: 'Estado local', description: 'Revisa que la aplicación esté lista y encuentra respuestas rápidas para continuar.', showGenericHeader: false },
  { path: '/notificaciones', label: 'Notificaciones', icon: 'bell', group: 'Sistema', eyebrow: 'Centro local', description: 'Revisa los avisos generados por los trabajos que ejecuta esta instalación.', showGenericHeader: true },
]

export const OPERATION_ITEMS = NAV_ITEMS.filter((item) => item.group === 'Operación')
export const SYSTEM_ITEMS = NAV_ITEMS.filter((item) => item.group === 'Sistema')

export function findNavItem(pathname: string): NavItem | undefined {
  return NAV_ITEMS.find((item) => pathname.startsWith(item.path))
}
