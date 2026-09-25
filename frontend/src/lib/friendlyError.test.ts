import { describe, expect, it } from 'vitest'
import { friendlyError } from './friendlyError'

describe('friendlyError', () => {
  it('tolera mensajes vacíos', () => {
    expect(friendlyError('')).toBe('')
    expect(friendlyError(undefined as unknown as string)).toBe('')
  })

  it('traduce fallos de autenticación, selector y WAF a lenguaje de usuario', () => {
    expect(friendlyError('autenticación rechazada: credenciales inválidas')).toBe(
      'No pudimos conectar con Zajuna: credenciales inválidas',
    )
    expect(friendlyError('selector de actividades no encontrado.')).toBe(
      'No encontramos la información esperada en esta página.',
    )
    expect(friendlyError('WAF detectado en la respuesta.')).toBe(
      'Zajuna bloqueó temporalmente esta consulta. Inténtalo de nuevo más tarde.',
    )
  })

  it('no expone términos internos ni la dirección loopback', () => {
    expect(friendlyError('falló el core local')).toBe('falló la aplicación local')
    expect(friendlyError('respuesta del núcleo local inválida')).toBe('respuesta de la aplicación local inválida')
    expect(friendlyError('sin respuesta en 127.0.0.1')).toBe('sin respuesta en aplicación local')
    expect(friendlyError('fetch failed')).toBe('No pudimos contactar la aplicación local.')
    expect(friendlyError('connect ECONNREFUSED')).toBe('connect No pudimos contactar la aplicación local.')
  })
})
