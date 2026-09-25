import type { Job, JobEvent } from '../types'

export type FailureActionKind = 'settings' | 'discover' | 'activities' | 'checklist' | 'diagnostics' | 'reports' | 'retry'

export interface FailureAction {
  kind: FailureActionKind
  label: string
  to?: string
}

export interface FailureExplanation {
  /** Qué pasó, en una frase. */
  title: string
  /** Por qué pasó, sin jerga técnica. */
  cause: string
  /** Qué debe hacer la persona ahora. */
  next: string
  actions: FailureAction[]
  /** Texto original del core, para soporte. */
  technical?: string
}

const RETRY: FailureAction = { kind: 'retry', label: 'Volver a intentar' }
const SETTINGS: FailureAction = { kind: 'settings', label: 'Revisar mi cuenta', to: '/configuracion?tab=account' }
const DIAGNOSTICS: FailureAction = { kind: 'diagnostics', label: 'Abrir diagnóstico', to: '/diagnostico' }
const DISCOVER: FailureAction = { kind: 'discover', label: 'Buscar rutas' }
const ACTIVITIES: FailureAction = { kind: 'activities', label: 'Elegir actividades', to: '/actividades' }

const NETWORK = /dial tcp|lookup |no such host|connection (refused|reset)|network is unreachable|conectar con Zajuna|\bEOF\b|i\/o timeout|TLS handshake/i
const TIMEOUT = /timeout|timed out|deadline exceeded|tardó demasiado/i
const REJECTED = /rechazad|HTTP 40[13]|credencial|contraseña|usuario o contraseña|invalid login/i
const BLOCKED = /WAF|bloquead/i
const CHALLENGE = /CAPTCHA|MFA/i
const SESSION = /sesión de Zajuna (vencida|expirada)|redirigió .*login|pantalla de login/i

function partialCounts(message: string) {
  const match = message.match(/(\d+)\s+guardadas,(?:\s+\d+\s+omitidas,)?\s+(\d+)\s+con error/i)
  const items = message.match(/\(ítems ([^)]+)\)/i)?.[1]
  return {
    saved: match ? Number(match[1]) : undefined,
    failed: match ? Number(match[2]) : undefined,
    items: items ? items.split(',').map((code) => code.trim()).filter(Boolean) : [],
  }
}

function connectionProblem(message: string): FailureExplanation | undefined {
  if (/no tiene publicaciones del instructor/i.test(message)) {
    return {
      title: 'No encontramos publicaciones tuyas en ese foro',
      cause: 'El foro existe, pero ninguna discusión o anuncio aparece con tu nombre como autor.',
      next: 'Si ya publicaste, confirma en Zajuna que lo hiciste con esta cuenta. Si aún no, publica y vuelve a preparar evidencias.',
      actions: [RETRY],
    }
  }
  if (CHALLENGE.test(message)) {
    return {
      title: 'Zajuna pidió una verificación adicional',
      cause: 'Zajuna mostró un CAPTCHA o pidió un código de verificación. La aplicación no puede resolverlo por ti.',
      next: 'Abre zajuna.sena.edu.co en tu navegador, inicia sesión y completa la verificación. Después vuelve aquí y pulsa “Volver a intentar”.',
      actions: [RETRY],
    }
  }
  if (BLOCKED.test(message)) {
    return {
      title: 'Zajuna bloqueó la consulta temporalmente',
      cause: 'El sitio de Zajuna rechazó las solicitudes por un tiempo, normalmente por muchas consultas seguidas.',
      next: 'Espera entre 10 y 15 minutos y vuelve a intentarlo. No es necesario cambiar nada en la aplicación.',
      actions: [RETRY],
    }
  }
  if (REJECTED.test(message)) {
    return {
      title: 'Zajuna no aceptó tu usuario o contraseña',
      cause: 'La contraseña guardada en este equipo no coincide con la de Zajuna (por ejemplo, si la cambiaste hace poco) o el tipo de documento no es el correcto.',
      next: 'Comprueba que puedes entrar a zajuna.sena.edu.co con tus datos. Luego ve a Configuración › Cuenta Zajuna, escribe la contraseña actual y guarda.',
      actions: [SETTINGS, RETRY],
    }
  }
  if (SESSION.test(message)) {
    return {
      title: 'La sesión de Zajuna se cerró durante el proceso',
      cause: 'Zajuna cerró la sesión antes de que el proceso terminara. Suele pasar en procesos largos o si iniciaste sesión en otro lugar.',
      next: 'Vuelve a intentarlo. Lo que ya se guardó se conserva.',
      actions: [RETRY],
    }
  }
  if (NETWORK.test(message)) {
    return {
      title: 'No hubo conexión con Zajuna',
      cause: 'El equipo no pudo comunicarse con zajuna.sena.edu.co: puede faltar internet o el sitio puede estar caído.',
      next: 'Revisa tu conexión a internet y que Zajuna abra en el navegador. Después vuelve a intentarlo.',
      actions: [RETRY],
    }
  }
  if (TIMEOUT.test(message)) {
    return {
      title: 'Zajuna tardó demasiado en responder',
      cause: 'La página no cargó a tiempo. Suele deberse a lentitud de Zajuna o de la conexión.',
      next: 'Vuelve a intentarlo en unos minutos. Si pasa siempre en la misma sección, revisa su ruta en el checklist (“Ver mapa”).',
      actions: [RETRY],
    }
  }
  return undefined
}

/**
 * Converts a failed job into "what happened / why / what to do next".
 * The core returns an errorCode plus a message that often carries raw Go or
 * Playwright text; the UI used to show that text as-is, which left users
 * without any next step.
 */
export function explainJobFailure(job: Pick<Job, 'type' | 'status' | 'errorCode' | 'errorMessage' | 'message'>): FailureExplanation {
  const code = String(job.errorCode || '').toLowerCase()
  const message = String(job.errorMessage || job.message || '')
  const technical = [job.errorCode, message].filter(Boolean).join(' · ') || undefined
  const withTech = (explanation: Omit<FailureExplanation, 'technical'>): FailureExplanation => ({ ...explanation, technical })

  if (job.status === 'cancelled' || code === 'capture_cancelled') {
    return withTech({
      title: 'El proceso se canceló',
      cause: 'Alguien detuvo el proceso o la aplicación se cerró mientras trabajaba.',
      next: 'Si todavía lo necesitas, vuelve a ejecutarlo. Lo que ya se guardó se conserva.',
      actions: [RETRY],
    })
  }

  switch (code) {
    case 'interrupted':
      return withTech({
        title: 'El proceso se interrumpió',
        cause: 'La aplicación se cerró o se reinició mientras el proceso estaba en marcha, y ya no quedaban reintentos automáticos.',
        next: 'Vuelve a ejecutarlo y deja la aplicación abierta hasta que termine. Lo que ya se guardó se conserva.',
        actions: [RETRY],
      })
    case 'browser_not_installed':
      return withTech({
        title: 'Falta el navegador interno de capturas',
        cause: 'La instalación de Zajuna App está incompleta: no se encontró el componente que toma las capturas.',
        next: 'Descarga e instala de nuevo la última versión de Zajuna App. Ten en cuenta que instalar una versión nueva empieza desde cero: genera antes el reporte PDF si lo necesitas.',
        actions: [DIAGNOSTICS],
      })
    case 'credential_unavailable':
      return withTech({
        title: 'No hay una contraseña de Zajuna guardada',
        cause: 'Este equipo no tiene guardada tu contraseña de Zajuna, o se borró al restablecer los datos.',
        next: 'Ve a Configuración › Cuenta Zajuna, escribe tu contraseña y guarda. Después vuelve a intentarlo.',
        actions: [SETTINGS],
      })
    case 'missing_username':
    case 'invalid_input':
      return withTech({
        title: 'Falta información de tu cuenta',
        cause: 'El proceso necesita tu usuario (documento) de Zajuna o una ficha activa, y no los encontró.',
        next: 'Revisa tu cuenta en Configuración y que haya una ficha seleccionada en Resumen. Luego vuelve a intentarlo.',
        actions: [SETTINGS],
      })
    case 'zajuna_challenge_required':
    case 'zajuna_session_expired':
    case 'zajuna_login_failed':
    case 'zajuna_browser_login_failed':
    case 'fichas_read_failed': {
      const specific = connectionProblem(message)
      if (specific) return withTech(specific)
      return withTech({
        title: 'No pudimos iniciar sesión en Zajuna',
        cause: 'Zajuna no permitió entrar con los datos guardados o no respondió como esperábamos.',
        next: 'Comprueba que puedes entrar a zajuna.sena.edu.co. Si cambiaste tu contraseña, actualízala en Configuración y vuelve a intentarlo.',
        actions: [SETTINGS, RETRY],
      })
    }
    case 'empty_fichas':
      return withTech({
        title: 'Zajuna no devolvió fichas para tu usuario',
        cause: 'La cuenta entró bien, pero no aparece ninguna ficha asignada como instructor.',
        next: 'Confirma en Zajuna que tienes fichas asignadas y que el tipo de documento en Configuración es el correcto.',
        actions: [SETTINGS, RETRY],
      })
    case 'course_map_not_found':
    case 'course_map_required':
      return withTech({
        title: 'Falta buscar las rutas del curso',
        cause: 'Para preparar evidencias primero hay que leer el contenido del curso de esta ficha.',
        next: 'Pulsa “Buscar rutas”, espera a que termine y vuelve a preparar las evidencias.',
        actions: [DISCOVER],
      })
    case 'checklist_map_empty':
    case 'checklist_map_invalid':
      return withTech({
        title: 'No encontramos secciones del curso para el checklist',
        cause: 'El contenido leído del curso no tiene rutas para los ítems solicitados, o está desactualizado.',
        next: 'Pulsa “Buscar rutas” para volver a leer el curso. Si sigue igual, comprueba que la ficha activa es la correcta.',
        actions: [DISCOVER],
      })
    case 'activities_not_selected':
      return withTech({
        title: 'Falta elegir tus actividades',
        cause: 'Las fechas límite y la retroalimentación se revisan solo en las actividades que tú orientas, y aún no las has marcado.',
        next: 'Ve a Actividades, marca las tuyas (o “Seleccionar todas” si orientas todas), guarda y vuelve a preparar evidencias.',
        actions: [ACTIVITIES],
      })
    case 'instructor_identity_unavailable':
      return withTech({
        title: 'No pudimos confirmar tu nombre en Zajuna',
        cause: 'Tu nombre se usa para capturar solo tus publicaciones en foros y anuncios, y Zajuna no lo mostró.',
        next: 'Vuelve a intentarlo. Si se repite, revisa en Zajuna que tu perfil tenga nombre y apellido.',
        actions: [RETRY],
      })
    case 'capture_partial_failure': {
      const { saved, failed, items } = partialCounts(message)
      const detail = connectionProblem(message)
      const itemText = items.length ? ` Revisa los ítems ${items.join(', ')}.` : ''
      return withTech({
        title: 'Algunas evidencias no se pudieron capturar',
        cause:
          (saved !== undefined ? `Se guardaron ${saved} evidencias y ${failed} fallaron.` : 'Una parte de las evidencias falló.') +
          (detail ? ` Motivo probable: ${detail.title.toLowerCase()}.` : ' Suele pasar cuando una sección cambió en Zajuna o no tiene contenido tuyo.'),
        next:
          'Lo que se guardó no se pierde.' +
          itemText +
          ' Abre el checklist, usa “Ver mapa” para confirmar o corregir la ruta de esos ítems y vuelve a preparar evidencias. Si el ítem no aplica a tu ficha, márcalo manualmente.',
        actions: [{ kind: 'checklist', label: 'Abrir checklist', to: '/checklist' }, RETRY],
      })
    }
    case 'ficha_not_found':
      return withTech({
        title: 'La ficha ya no está en este equipo',
        cause: 'El proceso pedía una ficha que no existe localmente, por ejemplo después de restablecer los datos o sincronizar de nuevo.',
        next: 'Sincroniza tus fichas, elige la ficha activa y vuelve a ejecutar el proceso.',
        actions: [{ kind: 'checklist', label: 'Ver mis fichas', to: '/fichas' }],
      })
    case 'invalid_zajuna_session':
      return withTech({
        title: 'La sesión de Zajuna no es válida',
        cause: 'Zajuna cerró o rechazó la sesión que usaba el proceso.',
        next: 'Vuelve a intentarlo. Si se repite, actualiza tu contraseña en Configuración.',
        actions: [RETRY, SETTINGS],
      })
    case 'course_map_failed':
    case 'course_map_persist_failed':
    case 'zajuna_courses_failed':
    case 'missing_courses': {
      const specific = connectionProblem(message)
      if (specific) return withTech(specific)
      return withTech({
        title: 'No pudimos leer el contenido del curso',
        cause: 'Zajuna no devolvió las secciones del curso de esta ficha, o no tienes acceso a él.',
        next: 'Comprueba que puedes abrir el curso en zajuna.sena.edu.co y vuelve a buscar las rutas.',
        actions: [DISCOVER],
      })
    }
  }

  if (/persist|prune|progress_failed|result_|retry_persist|evidence_group_failed/.test(code)) {
    return withTech({
      title: 'No pudimos guardar el resultado en este equipo',
      cause: 'La base de datos local estaba ocupada o sin espacio cuando el proceso intentó guardar.',
      next: 'Cierra y vuelve a abrir Zajuna App y repite el proceso. Si se repite, revisa el espacio libre en disco y el Diagnóstico.',
      actions: [RETRY, DIAGNOSTICS],
    })
  }
  if (/report/.test(code) || job.type === 'export-report') {
    return withTech({
      title: 'No se pudo generar el reporte',
      cause: 'El PDF no se pudo crear o guardar en este equipo.',
      next: 'Vuelve a intentarlo desde Reportes. Si se repite, revisa el Diagnóstico.',
      actions: [{ kind: 'reports', label: 'Ir a reportes', to: '/reportes' }, DIAGNOSTICS],
    })
  }

  const specific = connectionProblem(message)
  if (specific) return withTech(specific)

  return withTech({
    title: 'El proceso no se pudo completar',
    cause: 'Ocurrió un error que la aplicación no pudo resolver sola.',
    next: 'Vuelve a intentarlo. Si se repite, abre Diagnóstico y comparte el detalle técnico con soporte.',
    actions: [RETRY, DIAGNOSTICS],
  })
}

export interface FailedCaptureItem {
  itemCode: string
  slots: number[]
  /** Motivos en lenguaje claro, sin repetir. */
  reasons: string[]
}

const SELECTOR_MISSING = /selector requerido|no apareció|candidatos=/i

function friendlyCaptureReason(detail: string) {
  const known = connectionProblem(detail)
  if (known) return known.title
  if (SELECTOR_MISSING.test(detail)) return 'No encontramos la sección esperada en la página'
  if (/origen de URL no permitido|fuera del origen/i.test(detail)) return 'La ruta apunta fuera de Zajuna'
  return detail.length > 160 ? `${detail.slice(0, 157)}…` : detail
}

/**
 * Groups the `evidence_failed` events of a checklist capture by item. The
 * final job message only lists item codes and the first error; the events
 * keep every failed slot with its own reason.
 */
export function partialCaptureFailures(events: JobEvent[]): FailedCaptureItem[] {
  const byItem = new Map<string, FailedCaptureItem>()
  for (const event of events) {
    if (event.kind !== 'evidence_failed') continue
    const data = (event.data && typeof event.data === 'object' ? event.data : {}) as Record<string, unknown>
    const message = String(event.message || '')
    const itemCode = String(data.itemCode || message.split(':')[0] || '').trim()
    if (!itemCode) continue
    const detail = message.startsWith(`${itemCode}:`) ? message.slice(itemCode.length + 1).trim() : message
    const entry = byItem.get(itemCode) ?? { itemCode, slots: [], reasons: [] }
    const slot = Number(data.slotNumber)
    if (Number.isFinite(slot) && slot > 0 && !entry.slots.includes(slot)) entry.slots.push(slot)
    const reason = friendlyCaptureReason(detail)
    if (reason && !entry.reasons.includes(reason)) entry.reasons.push(reason)
    byItem.set(itemCode, entry)
  }
  return [...byItem.values()].map((entry) => ({ ...entry, slots: entry.slots.sort((a, b) => a - b) }))
}

const ACTIVE = ['queued', 'running', 'waiting_user', 'retrying']

function jobTime(job: Job) {
  return Date.parse(job.updatedAt || job.finishedAt || job.createdAt || '') || 0
}

/**
 * Failed jobs that still deserve attention: not dismissed, and not already
 * resolved by a later run of the same process that finished or is running.
 * Without this, weeks-old failures stayed in "Requiere tu atención" forever.
 */
export function unresolvedFailedJobs(jobs: Job[]) {
  return jobs.filter((job) => {
    if (job.status !== 'failed' || job.dismissed) return false
    const failedAt = jobTime(job)
    return !jobs.some(
      (other) =>
        other.id !== job.id &&
        other.type === job.type &&
        (!job.fichaId || !other.fichaId || other.fichaId === job.fichaId) &&
        (other.status === 'completed' || ACTIVE.includes(other.status)) &&
        jobTime(other) > failedAt,
    )
  })
}
