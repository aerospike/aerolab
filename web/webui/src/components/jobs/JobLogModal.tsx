import { useRef, useEffect, useCallback, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, X } from 'lucide-react'
import { useJobDetails } from '@/hooks/useJobs'
import { useJobStream } from '@/hooks/useJobStream'
import JobTerminal from './JobTerminal'
import type { JobTerminalRef } from './JobTerminal'
import JobAbortMenu from './JobAbortMenu'
import { toast } from 'sonner'
import { cn } from '@/utils/cn'

interface JobLogModalProps {
  jobId: string | null
  onClose: () => void
  onNext?: () => void
  onPrev?: () => void
  currentJobIndex?: number
  totalJobs?: number
}

const statusStyles: Record<string, string> = {
  pending:
    'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
  running:
    'bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300',
  completed:
    'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
  failed: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  error: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
}

const TERMINAL_STATUSES = ['completed', 'failed', 'error']

function StatusBadge({ status }: { status: string }) {
  const style = statusStyles[status] ?? statusStyles.pending
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium capitalize',
        style
      )}
    >
      {status}
    </span>
  )
}

export default function JobLogModal({
  jobId,
  onClose,
  onNext,
  onPrev,
  currentJobIndex = 0,
  totalJobs = 1,
}: JobLogModalProps) {
  const hasMultipleJobs = totalJobs > 1
  const canGoPrev = hasMultipleJobs && currentJobIndex > 0
  const canGoNext = hasMultipleJobs && currentJobIndex < totalJobs - 1
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const terminalRef = useRef<JobTerminalRef>(null)
  const [completedStatus, setCompletedStatus] = useState<string | null>(null)
  const [reloadOnClose, setReloadOnClose] = useState(false)

  // Track the latest job data from useJobDetails in a ref so the stable
  // handleComplete callback can read it without being recreated.
  const { job, isLoading } = useJobDetails(jobId)
  const jobRef = useRef(job)
  jobRef.current = job

  const handleClose = useCallback(() => {
    if (reloadOnClose) {
      window.location.reload()
      return
    }
    onClose()
  }, [reloadOnClose, onClose])

  const handleComplete = useCallback(
    (status: string, _error: string, reloadRequired?: boolean) => {
      setCompletedStatus(status)
      // Refresh inventory data when any job completes (success or failure)
      queryClient.invalidateQueries({ queryKey: ['inventory'] })
      if (reloadRequired) {
        // Only trigger reload for jobs that completed live while this modal
        // was open. If the job was already in a terminal state when fetched
        // by useJobDetails (i.e. we're viewing old history), skip the reload.
        const currentJob = jobRef.current
        if (currentJob?.completedAt && TERMINAL_STATUSES.includes(currentJob.status)) {
          return // historical job — don't prompt reload
        }
        setReloadOnClose(true)
        toast.info('Backend changed — page will reload when this dialog closes')
        return
      }
    },
    [queryClient]
  )

  const handleData = useCallback((data: string) => {
    terminalRef.current?.write(data)
  }, [])

  const { status: streamStatus, disconnect } = useJobStream(
    jobId,
    handleData,
    handleComplete
  )

  const effectiveStatus =
    (completedStatus || streamStatus || job?.status) || 'pending'
  const isRunning = ['pending', 'running'].includes(effectiveStatus)

  // Prevent closing while a config/backend job is still running — the user
  // must wait for it to complete or fail so the reload logic can fire properly.
  const isConfigBackendRunning =
    isRunning && job?.commandPath === 'config/backend'

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !isConfigBackendRunning) {
        handleClose()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [handleClose, isConfigBackendRunning])

  useEffect(() => {
    if (!jobId) return
    setCompletedStatus(null)
    setReloadOnClose(false)
    return () => {
      disconnect()
    }
  }, [jobId, disconnect])

  const handleCopyLog = () => {
    const content = terminalRef.current?.getContent()
    if (!content) {
      toast.error('No log content to copy')
      return
    }
    navigator.clipboard.writeText(content).then(
      () => toast.success('Log copied to clipboard'),
      () => toast.error('Failed to copy')
    )
  }

  const handleGoToInventory = () => {
    navigate('/inventory')
    onClose()
  }

  if (!jobId) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="job-modal-title"
    >
      <div className="flex h-[90vh] w-[90vw] max-w-6xl flex-col rounded-lg border border-slate-700 bg-slate-900 shadow-2xl">
        <header className="flex items-center justify-between gap-4 border-b border-slate-700 px-4 py-3">
          <div className="flex min-w-0 flex-1 items-center gap-3">
            {hasMultipleJobs && (
              <div className="flex items-center gap-1 rounded border border-slate-600 bg-slate-800">
                <button
                  type="button"
                  onClick={onPrev}
                  disabled={!canGoPrev}
                  className="rounded p-2 text-slate-400 hover:bg-slate-700 hover:text-slate-100 disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-slate-400"
                  aria-label="Previous job"
                >
                  <ChevronLeft className="h-4 w-4" />
                </button>
                <span className="px-2 py-1 text-sm text-slate-300">
                  Job {currentJobIndex + 1} of {totalJobs}
                </span>
                <button
                  type="button"
                  onClick={onNext}
                  disabled={!canGoNext}
                  className="rounded p-2 text-slate-400 hover:bg-slate-700 hover:text-slate-100 disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-slate-400"
                  aria-label="Next job"
                >
                  <ChevronRight className="h-4 w-4" />
                </button>
              </div>
            )}
            {isLoading ? (
              <span className="text-slate-400">Loading...</span>
            ) : (
              <>
                <h2
                  id="job-modal-title"
                  className="truncate text-lg font-semibold text-slate-100"
                >
                  aerolab {job?.commandPath ?? jobId}
                </h2>
                <StatusBadge status={effectiveStatus} />
                {isRunning && (
                  <JobAbortMenu jobId={jobId} isRunning={isRunning} />
                )}
              </>
            )}
          </div>
          <button
            type="button"
            onClick={handleClose}
            disabled={isConfigBackendRunning}
            className={cn(
              'rounded p-2 text-slate-400',
              isConfigBackendRunning
                ? 'cursor-not-allowed opacity-40'
                : 'hover:bg-slate-700 hover:text-slate-100'
            )}
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </header>

        <div className="flex flex-1 flex-col overflow-hidden p-4">
          {job?.cliCommand && (
            <div className="mb-3 rounded bg-slate-800 px-3 py-2 font-mono text-sm text-slate-300">
              $ {job.cliCommand}
            </div>
          )}
          <div className="min-h-0 flex-1">
            <JobTerminal ref={terminalRef} />
          </div>
        </div>

        <footer className="flex items-center justify-end gap-2 border-t border-slate-700 px-4 py-3">
          <button
            type="button"
            onClick={handleCopyLog}
            className="rounded-md border border-slate-600 bg-slate-800 px-4 py-2 text-sm font-medium text-slate-200 hover:bg-slate-700 dark:border-slate-500 dark:bg-slate-700 dark:hover:bg-slate-600"
          >
            Copy Log
          </button>
          {!reloadOnClose && !isConfigBackendRunning && (
            <button
              type="button"
              onClick={handleGoToInventory}
              className="rounded-md border border-slate-600 bg-slate-800 px-4 py-2 text-sm font-medium text-slate-200 hover:bg-slate-700 dark:border-slate-500 dark:bg-slate-700 dark:hover:bg-slate-600"
            >
              Go to Inventory
            </button>
          )}
          <button
            type="button"
            onClick={handleClose}
            disabled={isConfigBackendRunning}
            className={cn(
              'rounded-md px-4 py-2 text-sm font-medium text-white',
              isConfigBackendRunning
                ? 'cursor-not-allowed bg-blue-600/50'
                : 'bg-blue-600 hover:bg-blue-700'
            )}
          >
            {isConfigBackendRunning
              ? 'Waiting for backend change...'
              : reloadOnClose
                ? 'Close & Reload'
                : 'Close'}
          </button>
        </footer>
      </div>
    </div>
  )
}
