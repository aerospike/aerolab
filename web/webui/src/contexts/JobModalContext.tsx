import { createContext, useContext, useState, useCallback } from 'react'

interface JobModalContextValue {
  activeJobId: string | null
  activeJobIds: string[]
  openJobModal: (jobId: string, allJobIds?: string[]) => void
  closeJobModal: () => void
  nextJob: () => void
  prevJob: () => void
  currentJobIndex: number
}

const JobModalContext = createContext<JobModalContextValue>({
  activeJobId: null,
  activeJobIds: [],
  openJobModal: () => {},
  closeJobModal: () => {},
  nextJob: () => {},
  prevJob: () => {},
  currentJobIndex: 0,
})

export function useJobModal() {
  const ctx = useContext(JobModalContext)
  if (!ctx) {
    throw new Error('useJobModal must be used within JobModalProvider')
  }
  return ctx
}

export function JobModalProvider({ children }: { children: React.ReactNode }) {
  const [activeJobIds, setActiveJobIds] = useState<string[]>([])
  const [currentJobIndex, setCurrentJobIndex] = useState(0)

  const activeJobId = activeJobIds[currentJobIndex] ?? null

  const openJobModal = useCallback((jobId: string, allJobIds?: string[]) => {
    const ids = allJobIds?.length ? allJobIds : [jobId]
    setActiveJobIds(ids)
    setCurrentJobIndex(0)
  }, [])

  const closeJobModal = useCallback(() => {
    setActiveJobIds([])
    setCurrentJobIndex(0)
  }, [])

  const nextJob = useCallback(() => {
    setCurrentJobIndex((i) => Math.min(i + 1, activeJobIds.length - 1))
  }, [activeJobIds.length])

  const prevJob = useCallback(() => {
    setCurrentJobIndex((i) => Math.max(i - 1, 0))
  }, [])

  return (
    <JobModalContext.Provider
      value={{
        activeJobId,
        activeJobIds,
        openJobModal,
        closeJobModal,
        nextJob,
        prevJob,
        currentJobIndex,
      }}
    >
      {children}
    </JobModalContext.Provider>
  )
}
