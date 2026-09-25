import { Navigate, Route, Routes } from 'react-router-dom'
import { useSetupStatus } from './hooks/api'
import { ApiError, LOCAL_SESSION_REQUIRED } from './api/client'
import { Setup } from './pages/Setup'
import { AppShell } from './components/AppShell'
import { PageSkeleton, PageError } from './components/AsyncState'
import {
  Activities,
  Checklist,
  ChecklistItemDetail,
  Diagnostics,
  Evidences,
  Fichas,
  JobDetail,
  Notifications,
  Overview,
  Processes,
  Reports,
  Review,
  Settings,
} from './pages/lazy'

function App() {
  const { data: setup, isLoading, isError, error, refetch } = useSetupStatus()

  if (isLoading) {
    return <PageSkeleton label="Comprobando configuración local" />
  }

  if (error instanceof ApiError && error.status === LOCAL_SESSION_REQUIRED) {
    return <PageError message="Esta pestaña no tiene una sesión local válida. Vuelve a abrir Zajuna App desde su acceso directo para continuar." />
  }

  if (isError) {
    return <PageError message="No pudimos contactar a la aplicación local." action={<button className="button" onClick={() => refetch()}>Reintentar</button>} />
  }

  if (!setup?.setupComplete) {
    return <Setup />
  }

  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/resumen" replace />} />
        <Route path="/resumen" element={<Overview />} />
        <Route path="/fichas" element={<Fichas />} />
        <Route path="/checklist" element={<Checklist />} />
        <Route path="/checklist/:itemCode" element={<ChecklistItemDetail />} />
        <Route path="/actividades" element={<Activities />} />
        <Route path="/evidencias" element={<Evidences />} />
        <Route path="/revision" element={<Review />} />
        <Route path="/trabajos" element={<Processes />} />
        <Route path="/trabajos/:jobId" element={<JobDetail />} />
        <Route path="/reportes" element={<Reports />} />
        <Route path="/configuracion" element={<Settings />} />
        <Route path="/diagnostico" element={<Diagnostics />} />
        <Route path="/notificaciones" element={<Notifications />} />
        <Route path="*" element={<Navigate to="/resumen" replace />} />
      </Route>
    </Routes>
  )
}

export default App
