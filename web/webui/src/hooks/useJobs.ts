import { useEffect, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { fetchJobs, fetchJobDetails, cancelJob } from '@/api/client'
import { usePreferences } from './usePreferences'
import { useJobModal } from '@/contexts/JobModalContext'
const POLL_INTERVAL = 10 * 1000

export function useJobList() {
  const { showAllUsers, historyTruncateAt } = usePreferences()
  const { activeJobId } = useJobModal()
  const sessionLoadTime = useRef(Date.now()).current

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['jobs', showAllUsers],
    queryFn: () => fetchJobs({ all: showAllUsers }),
    refetchInterval: POLL_INTERVAL,
    staleTime: 0,
  })

  const jobs = data?.jobs ?? []

  // When a job completes with reloadRequired (e.g. config/backend), reload the page.
  // Only reload for jobs that completed during this session to avoid loops.
  // Skip the auto-reload while a job modal is open — the modal's own onClose
  // handler will trigger the reload so the user can view the output first.
  useEffect(() => {
    if (!jobs.length) return
    if (activeJobId) return // modal open — defer reload to modal close
    for (const job of jobs) {
      if (
        job.reloadRequired &&
        ['completed', 'failed', 'error'].includes(job.status) &&
        job.completedAt &&
        new Date(job.completedAt).getTime() >= sessionLoadTime
      ) {
        window.location.reload()
        return
      }
    }
  }, [jobs, sessionLoadTime, activeJobId])

  const truncateMs = historyTruncateAt
    ? new Date(historyTruncateAt).getTime()
    : 0
  const filteredJobs = truncateMs
    ? jobs.filter((j) => new Date(j.createdAt).getTime() >= truncateMs)
    : jobs

  const runningCount = filteredJobs.filter((j) =>
    ['pending', 'running'].includes(j.status)
  ).length
  const failedCount = filteredJobs.filter((j) =>
    ['failed', 'error'].includes(j.status)
  ).length
  const hasRunning = runningCount > 0
  const hasFailed = failedCount > 0

  return {
    jobs: filteredJobs,
    runningCount,
    failedCount,
    hasRunning,
    hasFailed,
    isLoading,
    error,
    refetch,
  }
}

export function useJobDetails(jobId: string | null) {
  const { data: job, isLoading } = useQuery({
    queryKey: ['job', jobId],
    queryFn: () => fetchJobDetails(jobId!),
    enabled: !!jobId,
    staleTime: 0,
  })

  return { job: job ?? null, isLoading }
}

export function useCancelJob() {
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: ({ jobId, force }: { jobId: string; force?: boolean }) =>
      cancelJob(jobId, force),
    onSuccess: (_, { jobId }) => {
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
      queryClient.invalidateQueries({ queryKey: ['job', jobId] })
    },
  })

  const cancel = (jobId: string, force?: boolean) =>
    mutation.mutateAsync({ jobId, force })

  return {
    cancel,
    isCancelling: mutation.isPending,
    error: mutation.error,
  }
}
