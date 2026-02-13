import { useState, useRef, useEffect } from 'react'
import { ChevronDown } from 'lucide-react'
import { cancelJob } from '@/api/client'
import { toast } from 'sonner'

interface JobAbortMenuProps {
  jobId: string
  isRunning: boolean
}

export default function JobAbortMenu({ jobId, isRunning }: JobAbortMenuProps) {
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const handleCancel = async (force: boolean) => {
    if (loading) return
    setLoading(true)
    try {
      await cancelJob(jobId, force)
      toast.success(force ? 'Job killed' : 'Clean terminate sent')
      setOpen(false)
    } catch (err) {
      toast.error(String(err instanceof Error ? err.message : err))
    } finally {
      setLoading(false)
    }
  }

  if (!isRunning) return null

  return (
    <div ref={menuRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        disabled={loading}
        className="inline-flex items-center gap-1 rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50 dark:bg-red-700 dark:hover:bg-red-600"
      >
        Abort
        <ChevronDown className="h-4 w-4" />
      </button>
      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 min-w-[140px] rounded-md border border-slate-200 bg-white py-1 shadow-lg dark:border-slate-600 dark:bg-slate-800">
          <button
            type="button"
            onClick={() => handleCancel(false)}
            disabled={loading}
            className="block w-full px-4 py-2 text-left text-sm text-slate-700 hover:bg-slate-100 dark:text-slate-200 dark:hover:bg-slate-700"
          >
            Clean Terminate (SIGTERM)
          </button>
          <button
            type="button"
            onClick={() => handleCancel(true)}
            disabled={loading}
            className="block w-full px-4 py-2 text-left text-sm text-red-600 hover:bg-slate-100 dark:text-red-400 dark:hover:bg-slate-700"
          >
            Kill (SIGKILL)
          </button>
        </div>
      )}
    </div>
  )
}
