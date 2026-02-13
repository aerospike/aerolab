import { NavLink } from 'react-router-dom'
import { Menu, Sun, Moon } from 'lucide-react'
import { usePreferences } from '@/hooks/usePreferences'
import { getConfig } from '@/utils/config'
import { useInventorySchema } from '@/hooks/useInventory'
import JobNotifications from '@/components/jobs/JobNotifications'
import BackendIcon from '@/components/common/BackendIcon'

interface NavbarProps {
  sidebarCollapsed: boolean
  onToggleSidebar: () => void
  onOpenJob: (jobId: string) => void
}

const backendLabels: Record<string, string> = {
  aws: 'AWS',
  gcp: 'GCP',
  docker: 'Docker',
}

export default function Navbar({ sidebarCollapsed, onToggleSidebar, onOpenJob }: NavbarProps) {
  const { simpleMode, setSimpleMode, forceSimpleMode, darkMode, setDarkMode } = usePreferences()
  const config = getConfig()
  const { backend } = useInventorySchema()

  return (
    <header className="flex h-14 items-center gap-4 border-b border-slate-200 bg-white px-4 dark:border-slate-700 dark:bg-slate-800">
      <button
        type="button"
        onClick={onToggleSidebar}
        className="rounded-lg p-2 text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700"
        aria-label="Toggle sidebar"
      >
        <Menu className="h-5 w-5" />
      </button>
      <NavLink
        to="/commands/config/backend"
        className="rounded-lg p-1.5 text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700 transition-colors"
        title={`Backend: ${backendLabels[backend] ?? backend} — click to configure`}
      >
        <BackendIcon backend={backend} className="h-6 w-6" />
      </NavLink>
      {sidebarCollapsed && (
        <NavLink
          to="/inventory"
          className="font-semibold text-slate-800 dark:text-slate-100"
        >
          AeroLab
        </NavLink>
      )}
      <div className="flex-1" />
      <div className="flex items-center gap-2">
        <label
          className="flex items-center gap-2 text-sm text-slate-600 dark:text-slate-400"
          title={forceSimpleMode ? 'Simple mode is enforced by administrator' : undefined}
        >
          <span>Simple Mode{forceSimpleMode ? ' (enforced)' : ''}</span>
          <input
            type="checkbox"
            checked={simpleMode}
            onChange={(e) => setSimpleMode(e.target.checked)}
            disabled={forceSimpleMode}
            className="h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed dark:border-slate-600 dark:bg-slate-700"
          />
        </label>
        <button
          type="button"
          onClick={() => setDarkMode(!darkMode)}
          className="rounded-lg p-2 text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700"
          aria-label={darkMode ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {darkMode ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
        </button>
        <JobNotifications onOpenJob={onOpenJob} />
        <span className="text-sm text-slate-500 dark:text-slate-400">
          {config.version}
        </span>
      </div>
    </header>
  )
}
