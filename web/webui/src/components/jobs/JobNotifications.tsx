import { useState, useRef, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { Bell, Loader2, CheckCircle, XCircle } from 'lucide-react'
import { useJobList } from '@/hooks/useJobs'
import ExportMenu from '@/components/common/ExportMenu'
import { usePreferences } from '@/hooks/usePreferences'
import type { Job } from '@/api/types'
import { formatTimeAgo } from '@/utils/formatTime'
import { cn } from '@/utils/cn'

interface JobNotificationsProps {
  onOpenJob: (jobId: string) => void
}

const maxJobs = 20

function StatusIcon({ status }: { status: string }) {
  if (['pending', 'running'].includes(status)) {
    return (
      <Loader2 className="h-4 w-4 flex-shrink-0 animate-spin text-blue-500" />
    )
  }
  if (['failed', 'error'].includes(status)) {
    return <XCircle className="h-4 w-4 flex-shrink-0 text-red-500" />
  }
  return <CheckCircle className="h-4 w-4 flex-shrink-0 text-green-500" />
}

export default function JobNotifications({ onOpenJob }: JobNotificationsProps) {
  const [open, setOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)
  const { jobs, runningCount, hasFailed, isLoading } = useJobList()
  const { showAllUsers, setShowAllUsers, setHistoryTruncateAt } = usePreferences()

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node)
      ) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const recentJobs = jobs.slice(0, maxJobs)
  const badgeCount = runningCount
  const badgeVariant = hasFailed ? 'failure' : 'normal'

  const handleClearHistory = () => {
    setHistoryTruncateAt(new Date().toISOString())
    setOpen(false)
  }

  const handleJobClick = (jobId: string) => {
    onOpenJob(jobId)
    setOpen(false)
  }

  return (
    <div ref={dropdownRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="relative flex rounded-lg p-2 text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700"
        aria-label="Job notifications"
      >
        <Bell className="h-5 w-5" />
        {badgeCount > 0 && (
          <span
            className={cn(
              'absolute -right-0.5 -top-0.5 flex h-4 w-4 min-w-[1rem] items-center justify-center rounded-full text-[10px] font-medium text-white',
              badgeVariant === 'failure'
                ? 'bg-red-600'
                : 'bg-blue-600'
            )}
          >
            {badgeCount > 9 ? '9+' : badgeCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-2 w-80 rounded-lg border border-slate-200 bg-white shadow-xl dark:border-slate-600 dark:bg-slate-800">
          <div className="border-b border-slate-200 p-3 dark:border-slate-600">
            <label className="flex cursor-pointer items-center gap-2 text-sm text-slate-600 dark:text-slate-400">
              <input
                type="checkbox"
                checked={showAllUsers}
                onChange={(e) => setShowAllUsers(e.target.checked)}
                className="h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-700"
              />
              Show all users
            </label>
          </div>

          <div className="max-h-80 overflow-y-auto">
            {isLoading ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-slate-400" />
              </div>
            ) : recentJobs.length === 0 ? (
              <div className="py-8 text-center text-sm text-slate-500 dark:text-slate-400">
                No recent jobs
              </div>
            ) : (
              <ul className="divide-y divide-slate-200 dark:divide-slate-600">
                {recentJobs.map((job: Job) => (
                  <li key={job.id}>
                    <button
                      type="button"
                      onClick={() => handleJobClick(job.id)}
                      className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-slate-50 dark:hover:bg-slate-700"
                    >
                      <StatusIcon status={job.status} />
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm text-slate-800 dark:text-slate-200">
                          {job.commandPath}
                        </div>
                        <div className="text-xs text-slate-500 dark:text-slate-400">
                          {formatTimeAgo(job.createdAt)}
                        </div>
                      </div>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="border-t border-slate-200 p-2 dark:border-slate-600">
            <div className="mb-2 flex justify-center">
              <ExportMenu jobs={jobs} />
            </div>
            <Link
              to="/jobs"
              onClick={() => setOpen(false)}
              className="block rounded px-3 py-2 text-sm text-slate-600 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-700"
            >
              View all jobs
            </Link>
            <button
              type="button"
              onClick={handleClearHistory}
              className="block w-full rounded px-3 py-2 text-left text-sm text-slate-600 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-700"
            >
              Clear History
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
