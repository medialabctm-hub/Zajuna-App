import { describe, expect, it } from 'vitest'
import { approvedItemsNotMarked, computeWorkflow, currentWorkflowStep, pendingAutomaticStep, visibleWorkflowSteps } from './workflow'

const base = { fichasCount: 0, hasActiveFicha: false, syncRunning: false, mapReady: false, discoverRunning: false, selectedActivities: 0, evidenceCount: 0, captureRunning: false }

describe('computeWorkflow', () => {
  it('starts at step 1 with nothing done', () => {
    expect(currentWorkflowStep(computeWorkflow(base))?.key).toBe('sync')
  })

  it('points to exactly one next step in order', () => {
    const steps = computeWorkflow({ ...base, fichasCount: 3, hasActiveFicha: true, mapReady: true })
    expect(steps.map((step) => step.state)).toEqual(['done', 'done', 'current', 'pending', 'pending'])
  })

  it('shows a running step instead of the next one', () => {
    const steps = computeWorkflow({ ...base, fichasCount: 3, hasActiveFicha: true, discoverRunning: true })
    expect(currentWorkflowStep(steps)?.key).toBe('routes')
    expect(steps[1].state).toBe('running')
  })

  it('numbers only the steps the person acts on', () => {
    const steps = computeWorkflow(base)
    expect(steps.filter((step) => step.automatic).map((step) => [step.key, step.number])).toEqual([['sync', 0], ['routes', 0]])
    expect(visibleWorkflowSteps(steps).map((step) => [step.key, step.number])).toEqual([['activities', 1], ['capture', 2], ['review', 3]])
  })

  it('reports the automatic step still pending', () => {
    expect(pendingAutomaticStep(computeWorkflow(base))?.key).toBe('sync')
    expect(pendingAutomaticStep(computeWorkflow({ ...base, fichasCount: 3, hasActiveFicha: true, discoverRunning: true }))?.state).toBe('running')
    expect(pendingAutomaticStep(computeWorkflow({ ...base, fichasCount: 3, hasActiveFicha: true, mapReady: true }))).toBeUndefined()
  })

  it('finishes when every evidence is reviewed without problems', () => {
    const steps = computeWorkflow({ ...base, fichasCount: 1, hasActiveFicha: true, mapReady: true, selectedActivities: 4, evidenceCount: 20, reviewOpen: 0, reviewTotal: 20 })
    expect(steps.every((step) => step.state === 'done')).toBe(true)
    expect(currentWorkflowStep(steps)).toBeUndefined()
  })

  it('keeps review as the next step while evidences need attention', () => {
    const steps = computeWorkflow({ ...base, fichasCount: 1, hasActiveFicha: true, mapReady: true, selectedActivities: 4, evidenceCount: 20, reviewOpen: 7, reviewTotal: 20 })
    expect(currentWorkflowStep(steps)?.key).toBe('review')
  })
})

describe('revisión y checklist', () => {
  const base = { fichasCount: 1, hasActiveFicha: true, syncRunning: false, mapReady: true, discoverRunning: false, selectedActivities: 5, evidenceCount: 10, captureRunning: false, reviewOpen: 0, reviewTotal: 10 }

  it('el paso de revisión no está hecho mientras haya ítems aprobados sin marcar', () => {
    const steps = computeWorkflow({ ...base, unmarkedApproved: 54 })
    const review = steps.find((step) => step.key === 'review')
    expect(review?.state).toBe('current')
    expect(review?.hint).toContain('54 ítems aprobados')
    expect(computeWorkflow({ ...base, unmarkedApproved: 0 }).find((step) => step.key === 'review')?.state).toBe('done')
  })

  it('cuenta solo ítems con toda su evidencia aprobada y aún pendientes', () => {
    const evidences = [
      { itemCode: '1.1', status: 'approved' },
      { itemCode: '1.1', status: 'approved' },
      { itemCode: '2.1', status: 'approved' },
      { itemCode: '2.1', status: 'pending' },
      { itemCode: '3.1', status: 'approved' },
    ]
    const items = [{ itemCode: '1.1', status: 'PENDIENTE' }, { itemCode: '2.1', status: 'PENDIENTE' }, { itemCode: '3.1', status: 'NO' }]
    expect(approvedItemsNotMarked(evidences, items)).toEqual(['1.1'])
  })

  it('ignora la captura que reemplazó una subida aprobada del instructor', () => {
    const evidences = [
      { itemCode: '7.3.2', status: 'pending', superseded: true },
      { itemCode: '7.3.2', status: 'approved' },
    ]
    expect(approvedItemsNotMarked(evidences, [{ itemCode: '7.3.2', status: 'PENDIENTE' }])).toEqual(['7.3.2'])
  })
})
