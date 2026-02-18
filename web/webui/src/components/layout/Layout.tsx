import { useState, useEffect, useCallback } from 'react'
import { Outlet } from 'react-router-dom'
import { usePreferences } from '@/hooks/usePreferences'
import { useHealth } from '@/hooks/useHealth'
import { useJobModal } from '@/contexts/JobModalContext'
import { ServerBrowseContext } from '@/contexts/ServerBrowseContext'
import { getConfig } from '@/utils/config'
import Sidebar from './Sidebar'
import Navbar from './Navbar'
import Footer from './Footer'
import OverlayDisconnect from '@/components/common/OverlayDisconnect'
import JobLogModal from '@/components/jobs/JobLogModal'
import { ArrowUp } from 'lucide-react'

export default function Layout() {
  const { sidebarCollapsed, setSidebarCollapsed } = usePreferences()
  const { isHealthy, refetch, allowServerBrowse, version: serverVersion } = useHealth()
  const {
    activeJobId,
    activeJobIds,
    openJobModal,
    closeJobModal,
    nextJob,
    prevJob,
    currentJobIndex,
  } = useJobModal()

  const [showBackToTop, setShowBackToTop] = useState(false)

  // Reload when daemon version changes (e.g. after upgrade) so UI stays in sync
  useEffect(() => {
    if (!serverVersion) return
    const loadedVersion = getConfig().version
    if (loadedVersion !== '' && serverVersion !== loadedVersion) {
      window.location.reload()
    }
  }, [serverVersion])

  // Refetch health when user returns to the tab so we detect version mismatch quickly
  useEffect(() => {
    const handleVisibility = () => {
      if (document.visibilityState === 'visible') refetch()
    }
    document.addEventListener('visibilitychange', handleVisibility)
    return () => document.removeEventListener('visibilitychange', handleVisibility)
  }, [refetch])

  useEffect(() => {
    const handleScroll = () => {
      setShowBackToTop(window.scrollY > 200)
    }
    window.addEventListener('scroll', handleScroll, { passive: true })
    handleScroll()
    return () => window.removeEventListener('scroll', handleScroll)
  }, [])

  const scrollToTop = useCallback(() => {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }, [])

  return (
    <ServerBrowseContext.Provider value={allowServerBrowse}>
    <div className="flex min-h-screen flex-col bg-slate-50 dark:bg-gray-900">
      <div className="flex flex-1 overflow-hidden">
        <Sidebar
          collapsed={sidebarCollapsed}
          onToggleCollapsed={() => setSidebarCollapsed(!sidebarCollapsed)}
        />
        <div className="flex flex-1 flex-col min-w-0">
          <Navbar
            sidebarCollapsed={sidebarCollapsed}
            onToggleSidebar={() => setSidebarCollapsed(!sidebarCollapsed)}
            onOpenJob={openJobModal}
          />
          <main className="flex-1 overflow-auto bg-slate-50 dark:bg-gray-900 p-6">
            <Outlet />
          </main>
          <Footer />
        </div>
      </div>
      {showBackToTop && (
        <button
          type="button"
          onClick={scrollToTop}
          style={{
            position: 'fixed',
            top: '5rem',
            right: '2rem',
            zIndex: 9999,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: '2.5rem',
            height: '2.5rem',
            borderRadius: '9999px',
            backgroundColor: '#2563eb',
            color: '#fff',
            boxShadow: '0 4px 14px rgba(0,0,0,0.3)',
            border: 'none',
            cursor: 'pointer',
          }}
          title="Back to top"
        >
          <ArrowUp style={{ width: '1.25rem', height: '1.25rem' }} />
        </button>
      )}
      {!isHealthy && (
        <OverlayDisconnect onReconnect={refetch} />
      )}
      {activeJobId && (
        <JobLogModal
          jobId={activeJobId}
          onClose={closeJobModal}
          onNext={nextJob}
          onPrev={prevJob}
          currentJobIndex={currentJobIndex}
          totalJobs={activeJobIds.length}
        />
      )}
    </div>
    </ServerBrowseContext.Provider>
  )
}
