import { lazy } from 'react'

// Each page is its own chunk so the first paint only parses the shell and
// the page being opened. The loaders are kept so every page can be warmed up
// once the app is idle and navigation never waits for a download.
const loaders = {
  Overview: () => import('./Overview').then((module) => ({ default: module.Overview })),
  Fichas: () => import('./Fichas').then((module) => ({ default: module.Fichas })),
  Checklist: () => import('./Checklist').then((module) => ({ default: module.Checklist })),
  ChecklistItemDetail: () => import('./ChecklistItemDetail').then((module) => ({ default: module.ChecklistItemDetail })),
  Activities: () => import('./Activities').then((module) => ({ default: module.Activities })),
  Evidences: () => import('./Evidences').then((module) => ({ default: module.Evidences })),
  Processes: () => import('./Processes').then((module) => ({ default: module.Processes })),
  JobDetail: () => import('./JobDetail').then((module) => ({ default: module.JobDetail })),
  Reports: () => import('./Reports').then((module) => ({ default: module.Reports })),
  Settings: () => import('./Settings').then((module) => ({ default: module.Settings })),
  Diagnostics: () => import('./Diagnostics').then((module) => ({ default: module.Diagnostics })),
  Notifications: () => import('./Notifications').then((module) => ({ default: module.Notifications })),
  Review: () => import('./Review').then((module) => ({ default: module.Review })),
}

export const Overview = lazy(loaders.Overview)
export const Fichas = lazy(loaders.Fichas)
export const Checklist = lazy(loaders.Checklist)
export const ChecklistItemDetail = lazy(loaders.ChecklistItemDetail)
export const Activities = lazy(loaders.Activities)
export const Evidences = lazy(loaders.Evidences)
export const Processes = lazy(loaders.Processes)
export const JobDetail = lazy(loaders.JobDetail)
export const Reports = lazy(loaders.Reports)
export const Settings = lazy(loaders.Settings)
export const Diagnostics = lazy(loaders.Diagnostics)
export const Notifications = lazy(loaders.Notifications)
export const Review = lazy(loaders.Review)

/** Downloads every page chunk in the background (the browser caches them). */
export function preloadPages() {
  const run = () => Object.values(loaders).forEach((load) => void load().catch(() => undefined))
  if ('requestIdleCallback' in window) window.requestIdleCallback(run, { timeout: 3000 })
  else setTimeout(run, 1500)
}
