import { describe, expect, it } from 'vitest'
import { approvedItemsPercent, countByTab, filterMissingItems, filterReviewEntries, groupReviewByItem } from './Review'
import type { EvidenceReviewEntry } from '../types'

const entry = (overrides: Partial<EvidenceReviewEntry>): EvidenceReviewEntry => ({
  evidenceId: 'e',
  itemCode: '1.1',
  status: 'approved',
  source: 'auto',
  reasons: [],
  ...overrides,
})

const entries = [
  entry({ evidenceId: 'a', itemCode: '10.1.2', status: 'pending', slotNumber: 2, itemDescription: 'Sesiones en línea' }),
  entry({ evidenceId: 'b', itemCode: '2.1', status: 'approved' }),
  entry({ evidenceId: 'c', itemCode: '10.1.2', status: 'rejected', slotNumber: 1 }),
  entry({ evidenceId: 'd', itemCode: '7.4.2', status: 'pending', itemDescription: 'Foro técnico' }),
]

describe('filterReviewEntries', () => {
  it('filtra por estado y por búsqueda sin distinguir tildes', () => {
    expect(filterReviewEntries(entries, 'pending', '').map((e) => e.evidenceId)).toEqual(['a', 'd'])
    expect(filterReviewEntries(entries, 'all', 'tecnico').map((e) => e.evidenceId)).toEqual(['d'])
    expect(filterReviewEntries(entries, 'all', '10.1').map((e) => e.evidenceId)).toEqual(['a', 'c'])
    expect(filterReviewEntries(entries, 'rejected', 'foro')).toEqual([])
  })
})

describe('groupReviewByItem', () => {
  it('agrupa por ítem en orden natural y ordena por espacio', () => {
    const groups = groupReviewByItem(entries)
    expect(groups.map((g) => g.itemCode)).toEqual(['2.1', '7.4.2', '10.1.2'])
    expect(groups[2].entries.map((e) => e.evidenceId)).toEqual(['c', 'a'])
    expect(groups[2].itemDescription).toBe('Sesiones en línea')
  })
})

describe('countByTab', () => {
  it('cuenta evidencias por estado', () => {
    expect(countByTab(entries)).toEqual({ pending: 2, approved: 1, rejected: 1, all: 4 })
  })
})

describe('approvedItemsPercent', () => {
  it('incluye los ítems sin evidencia en el total', () => {
    expect(approvedItemsPercent({ itemsApproved: 20, itemsWithEvidence: 61, itemsMissing: 1 })).toBe(32)
    expect(approvedItemsPercent({ itemsApproved: 0, itemsWithEvidence: 0, itemsMissing: 0 })).toBe(0)
    expect(approvedItemsPercent(undefined)).toBe(0)
  })
})

describe('filterMissingItems', () => {
  it('busca por código o descripción', () => {
    const items = [{ itemCode: '11.2.3', description: 'Cierre' }, { itemCode: '3.1', description: 'Perfil' }]
    expect(filterMissingItems(items, 'perfil').map((i) => i.itemCode)).toEqual(['3.1'])
    expect(filterMissingItems(items, '')).toHaveLength(2)
  })
})
