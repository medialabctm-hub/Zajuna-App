import { describe, expect, it } from 'vitest'
import { findNavItem, NAV_ITEMS, OPERATION_ITEMS, SYSTEM_ITEMS } from './nav'

describe('NAV_ITEMS', () => {
  it('incluye Evidencias y Configuración con copy en español', () => {
    const evidencias = NAV_ITEMS.find((item) => item.path === '/evidencias')
    expect(evidencias).toMatchObject({
      label: 'Evidencias',
      group: 'Operación',
      eyebrow: 'Tus evidencias',
      showGenericHeader: true,
    })
    expect(evidencias?.description).toMatch(/evidencias|reporte/i)

    const settings = NAV_ITEMS.find((item) => item.path === '/configuracion')
    expect(settings).toMatchObject({
      label: 'Configuración',
      group: 'Sistema',
      showGenericHeader: true,
    })
  })

  it('separa operación y sistema sin solapar rutas', () => {
    expect(OPERATION_ITEMS.every((item) => item.group === 'Operación')).toBe(true)
    expect(SYSTEM_ITEMS.every((item) => item.group === 'Sistema')).toBe(true)
    expect(OPERATION_ITEMS.length + SYSTEM_ITEMS.length).toBe(NAV_ITEMS.length)

    const paths = NAV_ITEMS.map((item) => item.path)
    expect(new Set(paths).size).toBe(paths.length)
    expect(paths).toContain('/evidencias')
    expect(paths).toContain('/configuracion')
  })
})

describe('findNavItem', () => {
  it('resuelve rutas exactas y anidadas usadas por el shell', () => {
    expect(findNavItem('/evidencias')?.label).toBe('Evidencias')
    expect(findNavItem('/checklist/ITEM-1')?.path).toBe('/checklist')
    expect(findNavItem('/trabajos/job-99')?.path).toBe('/trabajos')
    expect(findNavItem('/configuracion')?.label).toBe('Configuración')
    expect(findNavItem('/revision')?.label).toBe('Revisión')
  })

  it('ubica Revisión justo después de Evidencias en Operación', () => {
    const paths = OPERATION_ITEMS.map((item) => item.path)
    expect(paths.indexOf('/revision')).toBe(paths.indexOf('/evidencias') + 1)
    expect(findNavItem('/revision')).toMatchObject({ eyebrow: 'Paso 5 · Revisión', showGenericHeader: true })
  })

  it('no inventa entradas para rutas desconocidas', () => {
    expect(findNavItem('/ruta-inexistente')).toBeUndefined()
    expect(findNavItem('')).toBeUndefined()
  })
})
