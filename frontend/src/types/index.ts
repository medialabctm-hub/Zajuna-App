export interface SetupStatus {
  setupComplete: boolean
  zajunaUsername?: string
  zajunaDocumentType?: 'CC' | 'TI' | 'CE'
  hasZajunaPassword?: boolean
  /** Optional profile metadata returned after the first successful Zajuna session. */
  profile?: {
    fullName?: string
    name?: string
    firstName?: string
    lastName?: string
    email?: string
  }
}

export interface SetupSaveResponse {
  saved: boolean
  syncQueued?: boolean
}

export interface AppSettings {
  session: { autoRenew: boolean }
  capture: { fullPage: boolean; reuseSession: boolean; motion: boolean }
  notifications: { jobCompleted: boolean; needsReview: boolean }
  storage: { retentionKeep: number; retentionDays: number }
}

export interface AppInfo {
  version: string
  /** Environment-relative location (e.g. %LOCALAPPDATA%\ZajunaApp), never absolute. */
  dataDir: string
  supervised: boolean
  resetPending: boolean
}

export interface AppResetResult {
  staged: boolean
  restarting: boolean
  backupName?: string
}

export type DiagnosticStatus = 'ok' | 'warn' | 'error'

export interface DiagnosticCheck {
  id: string
  title: string
  description: string
  status: DiagnosticStatus
  detail?: string
  checkedAt?: string
}

export interface DiagnosticIncident {
  jobId: string
  type: string
  errorCode?: string
  updatedAt?: string
}

export interface Diagnostics {
  generatedAt: string
  checks: DiagnosticCheck[]
  incidents: DiagnosticIncident[]
}

export interface Backup {
  name: string
  createdAt: string
  sizeBytes: number
}

export interface Ficha {
  id: string
  externalId: string
  name: string
  courseId: string
  status?: string
  syncedAt?: string
  updatedAt: string
}

export type ItemStatus = 'SI' | 'NO' | 'PENDIENTE'

export interface Evidence {
  id: string
  fichaId?: string
  itemCode?: string
  name: string
  slotNumber?: number
  /** Opaque identifier of the stored file; equal keys mean the same file. */
  fileKey?: string
  format?: string
  source?: string
  sha256?: string
  capturedAt?: string
}

export interface DashboardItem {
  itemCode: string
  description: string
  categoryCode: string
  categoryLabel: string
  status: ItemStatus
  evidenceCount: number
  maxEvidences: number
  evidences: Evidence[]
  captureConfidence?: string
  confidence?: string
  updatedAt?: string
}

export interface DashboardCategory {
  code: string
  label: string
  total: number
  yes: number
  no: number
  pending: number
}

export interface DashboardSummary {
  yes: number
  no: number
  pending: number
  percentage: number
  total?: number
}

export interface DashboardFicha {
  name: string
  externalId: string
  courseId: string
  updatedAt: string
  phase?: string
}

export interface Dashboard {
  activeFichaId: string
  ficha: DashboardFicha
  summary: DashboardSummary
  items: DashboardItem[]
  categories: DashboardCategory[]
}

export interface ChecklistItemEvent {
  id: number
  fromStatus?: ItemStatus | string
  toStatus: ItemStatus | string
  source: string
  note?: string
  jobId?: string
  createdAt?: string
}

export interface ChecklistItemDetail {
  item: DashboardItem
  events: ChecklistItemEvent[]
}

export interface Activity {
  id: string
  title: string
  technical: boolean
  phaseName?: string
  phaseSection?: number
  selected: boolean
  /** false para competencias transversales: su evidencia es de otro instructor. */
  selectable?: boolean
  blockedReason?: string
}

export interface ActivitiesResponse {
  fichaId?: string
  courseId?: string
  mapReady?: boolean
  selectionConfigured?: boolean
  discovery?: {
    status?: 'required' | 'queued' | 'running' | 'completed' | 'failed' | string
    action?: string
    message?: string
  }
  activities: Activity[]
  selectedCount: number
  /** Máximo de evidencias por ítem ligado a actividades (6.1, 10.1.1, 10.1.2). */
  slotsPerItem?: number
}

export type JobStatus = 'queued' | 'running' | 'waiting_user' | 'retrying' | 'completed' | 'failed' | 'cancelled'

export type JobType =
  | 'sync-fichas'
  | 'test-zajuna-connection'
  | 'discover-course-maps'
  | 'capture-checklist'
  | 'capture-evidence'
  | 'capture-browser'
  | 'export-report'
  | 'backup'

export interface Job {
  id: string
  type: JobType
  status: JobStatus
  progress: number
  message?: string
  stage?: string
  attempt?: number
  maxAttempts?: number
  errorCode?: string
  errorMessage?: string
  createdAt?: string
  startedAt?: string
  finishedAt?: string
  updatedAt?: string
  result?: unknown
  /** El usuario lo quitó de “Requiere tu atención”; sigue visible en Trabajos. */
  dismissed?: boolean
  /** Ficha e ítems del input original (sin datos sensibles). */
  fichaId?: string
  itemCodes?: string[]
}

export interface JobEvent {
  jobId: string
  kind: string
  stage?: string
  progress?: number
  message?: string
  data?: unknown
  createdAt?: string
}

export interface Notification {
  id: string
  kind: string
  title: string
  message: string
  jobId?: string
  readAt?: string
  createdAt: string
}

export interface Schedule {
  id: string
  workerType: JobType | string
  input?: Record<string, unknown>
  intervalSeconds: number
  enabled: boolean
  nextRunAt: string
  lastRunAt?: string
  lastJobId?: string
  createdAt?: string
  updatedAt?: string
}

export interface EvidenceGroup {
  id?: string
  fichaId?: string
  groupKey?: string
  title?: string
  reason?: string
  confidence?: string
  itemCodes?: string[]
  evidenceIds?: string[]
  evidences: Evidence[]
}

export interface Report {
  id: string
  name?: string
  format?: string
  status: string
  updatedAt?: string
  createdAt?: string
}

export interface RouteTarget {
  itemCode: string
  coveredItemCodes?: string[]
  groupName: string
  url?: string
  routeKind: 'course' | 'page' | 'phase' | 'forum' | 'assign'
  name?: string
  activityTitle?: string
  cssSelector?: string
  revealSelectors?: string[]
  fullPage?: boolean
  ownerOnly?: boolean
  routeKey?: string
}

export interface TargetsResponse {
  fichaId?: string
  courseId?: string
  mapReady?: boolean
  selectionConfigured?: boolean
  discovery?: {
    status?: 'required' | 'queued' | 'running' | 'completed' | 'failed' | string
    action?: string
    message?: string
  }
  targets: RouteTarget[]
  summary?: {
    itemCount?: number
    resolvedItems?: number
    unresolvedItems?: number
    slotCount?: number
    maxSlotCount?: number
    captureUnitCount?: number
    coverageCount?: number
  }
}

export type RouteReviewStatus = 'confirmed' | 'correction' | 'review'

export interface RouteReview {
  routeKey: string
  status: RouteReviewStatus
  manualUrl?: string
  manualSelector?: string
}

export type EvidenceReviewStatus = 'approved' | 'pending' | 'rejected'

export interface EvidenceReviewReason {
  code: string
  message: string
}

export interface EvidenceReviewEntry {
  evidenceId: string
  itemCode: string
  itemDescription?: string
  slotNumber?: number
  name?: string
  status: EvidenceReviewStatus
  source: 'auto' | 'manual' | string
  note?: string
  reasons: EvidenceReviewReason[]
  width?: number
  height?: number
  sha256?: string
  sharedWith?: string[]
}

export interface EvidenceReviewMissingItem {
  itemCode: string
  description?: string
  reason?: string
}

export interface EvidenceReviewSummary {
  total: number
  approved: number
  pending: number
  rejected: number
  itemsWithEvidence: number
  itemsApproved: number
  itemsPending: number
  itemsMissing: number
}

export interface EvidenceReview {
  fichaId: string
  verifiedAt?: string
  summary: EvidenceReviewSummary
  evidences: EvidenceReviewEntry[]
  missingItems: EvidenceReviewMissingItem[]
}
