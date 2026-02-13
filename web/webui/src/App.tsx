import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Toaster } from 'sonner'
import { getRootPath } from '@/utils/config'
import { PreferencesProvider } from '@/hooks/usePreferences'
import { JobModalProvider } from '@/contexts/JobModalContext'
import Layout from '@/components/layout/Layout'
import CommandPage from '@/pages/CommandPage'
import InventoryPage from '@/pages/InventoryPage'
import JobsPage from '@/pages/JobsPage'

function App() {
  const basename = getRootPath() || '/'

  return (
    <PreferencesProvider>
    <BrowserRouter basename={basename}>
      <JobModalProvider>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Navigate to="/inventory" replace />} />
          <Route path="commands/*" element={<CommandPage />} />
          <Route path="inventory" element={<InventoryPage />} />
          <Route path="jobs" element={<JobsPage />} />
        </Route>
      </Routes>
      <Toaster position="bottom-right" theme="system" richColors />
      </JobModalProvider>
    </BrowserRouter>
    </PreferencesProvider>
  )
}

export default App
