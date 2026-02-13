import { useState } from 'react'
import { useJobList } from '@/hooks/useJobs'
import { useJobModal } from '@/contexts/JobModalContext'
import { usePreferences } from '@/hooks/usePreferences'
import { formatTimeAgo, formatDuration } from '@/utils/formatTime'
import { Loader2, CheckCircle, XCircle, Search } from 'lucide-react'
import ExportMenu from '@/components/common/ExportMenu'
import type { Job } from '@/api/types'
import { cn } from '@/utils/cn'

type StatusFilter = 'all' | 'running' | 'completed' | 'failed'

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

function statusMatches(job: Job, filter: StatusFilter): boolean {
  switch (filter) {
    case 'all':
      return true
    case 'running':
      return ['pending', 'running'].includes(job.status)
    case 'completed':
      return job.status === 'completed'
    case 'failed':
      return ['failed', 'error'].includes(job.status)
    default:
      return true
  }
}

export default function JobsPage() {
  const { jobs, isLoading, error, refetch } = useJobList()
  const { openJobModal } = useJobModal()
  const { showAllUsers, setShowAllUsers } = usePreferences()
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [search, setSearch] = useState('')

  const filteredJobs = jobs.filter((job: Job) => {
    if (!statusMatches(job, statusFilter)) return false
    if (search) {
      const q = search.toLowerCase()
      return job.commandPath.toLowerCase().includes(q)
    }
    return true
  })

  const tabs: { id: StatusFilter; label: string }[] = [
    { id: 'all', label: 'All' },
    { id: 'running', label: 'Running' },
    { id: 'completed', label: 'Completed' },
    { id: 'failed', label: 'Failed' },
  ]

  return (
    <div className="flex flex-col">
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">
          Jobs
        </h1>
        <div className="flex items-center gap-4">
          <ExportMenu jobs={jobs} />
          <label className="flex items-center gap-2 text-sm text-slate-600 dark:text-slate-400">
          <input
            type="checkbox"
            checked={showAllUsers}
            onChange={(e) => setShowAllUsers(e.target.checked)}
            className="h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-700"
          />
          Show all users
        </label>
        </div>
      </div>

      <div className="mb-4 flex flex-col gap-4 sm:flex-row sm:items-center">
        <div className="flex gap-1 rounded-lg border border-slate-200 bg-white p-1 dark:border-slate-600 dark:bg-slate-800">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              onClick={() => setStatusFilter(tab.id)}
              className={cn(
                'rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                statusFilter === tab.id
                  ? 'bg-slate-200 text-slate-900 dark:bg-slate-600 dark:text-slate-100'
                  : 'text-slate-600 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-700'
              )}
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="relative flex-1 sm:max-w-xs">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            placeholder="Filter by command path..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full rounded-lg border border-slate-200 bg-white py-2 pl-9 pr-3 text-sm dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100"
          />
        </div>
      </div>

      <div className="overflow-hidden rounded-lg border border-slate-200 bg-white dark:border-slate-600 dark:bg-slate-800">
        {error ? (
          <div className="flex flex-col items-center justify-center gap-4 py-16">
            <p className="text-center text-slate-600 dark:text-slate-400">
              Failed to load jobs: {String(error instanceof Error ? error.message : error)}
            </p>
            <button
              type="button"
              onClick={() => refetch()}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              Retry
            </button>
          </div>
        ) : isLoading ? (
          <div className="flex items-center justify-center py-16">
            <Loader2 className="h-8 w-8 animate-spin text-slate-400" />
          </div>
        ) : filteredJobs.length === 0 ? (
          <div className="py-16 text-center text-slate-500 dark:text-slate-400">
            No jobs found
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-slate-200 dark:divide-slate-600">
              <thead>
                <tr>
                  <th className="w-10 px-4 py-3 text-left text-xs font-medium text-slate-500 dark:text-slate-400">
                    Status
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-slate-500 dark:text-slate-400">
                    Command
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-slate-500 dark:text-slate-400">
                    User
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-slate-500 dark:text-slate-400">
                    Started
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-slate-500 dark:text-slate-400">
                    Duration
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-200 dark:divide-slate-600">
                {filteredJobs.map((job: Job) => (
                  <tr
                    key={job.id}
                    onClick={() => openJobModal(job.id)}
                    className="cursor-pointer hover:bg-slate-50 dark:hover:bg-slate-700/50"
                  >
                    <td className="px-4 py-3">
                      <StatusIcon status={job.status} />
                    </td>
                    <td className="px-4 py-3">
                      <span className="font-mono text-sm text-slate-800 dark:text-slate-200">
                        {job.commandPath}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-slate-600 dark:text-slate-400">
                      {job.user}
                    </td>
                    <td className="px-4 py-3 text-sm text-slate-600 dark:text-slate-400">
                      {formatTimeAgo(job.createdAt)}
                    </td>
                    <td className="px-4 py-3 text-sm text-slate-600 dark:text-slate-400">
                      {formatDuration(
                        job.startedAt ?? job.createdAt,
                        job.completedAt
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
