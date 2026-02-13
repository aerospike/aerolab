import { useState, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { ChevronDown, Trash2, Copy, ExternalLink } from 'lucide-react'
import { useInventoryAction } from '@/hooks/useInventory'
import { useJobModal } from '@/contexts/JobModalContext'
import { useCommandVisibility } from '@/hooks/useCommandVisibility'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { Modal } from '@/components/common/Modal'
import { ExpiryPrompt } from './ExpiryPrompt'
import { cn } from '@/utils/cn'
import { fetchJobDetails, fetchJobLogs } from '@/api/client'

interface AGIActionsProps {
  selectedRows: Record<string, unknown>[]
  onRefresh: () => void
  backend: string
}

const NODE_NAV = [
  { label: 'Status', path: 'agi/status' },
  { label: 'Get share link', path: 'agi/add-auth-token' },
  { label: 'Change label', path: 'agi/change-label' },
  { label: 'Rerun ingest', path: 'agi/run-ingest' },
] as const

export function AGIActions({
  selectedRows,
  onRefresh,
  backend,
}: AGIActionsProps) {
  const cmd = useCommandVisibility()
  const [nodeOpen, setNodeOpen] = useState(false)
  const [expiryPromptOpen, setExpiryPromptOpen] = useState(false)
  const [destroyConfirmOpen, setDestroyConfirmOpen] = useState(false)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)
  const [shareLinkLoading, setShareLinkLoading] = useState(false)
  const [shareLinkUrl, setShareLinkUrl] = useState('')
  const [shareLinkOpen, setShareLinkOpen] = useState(false)
  const [shareLinkCopied, setShareLinkCopied] = useState(false)
  const shareLinkAbort = useRef<AbortController | null>(null)
  const [changeLabelOpen, setChangeLabelOpen] = useState(false)
  const [changeLabelValue, setChangeLabelValue] = useState('')
  const [changeLabelName, setChangeLabelName] = useState('')
  const { mutateAsync: runAction } = useInventoryAction()
  const { openJobModal } = useJobModal()
  const navigate = useNavigate()

  const items = selectedRows.map((r) => ({
    clusterName: String(r.clusterName ?? r.name ?? ''),
    nodeNo: 1,
  }))

  const runBulkAction = async (
    action: string,
    params?: Record<string, unknown>
  ) => {
    try {
      const { jobId, jobIds } = await runAction({
        items,
        action,
        type: 'agi',
        params,
      })
      openJobModal(jobId, jobIds)
      if (jobIds && jobIds.length > 1) {
        toast.info(`Started ${jobIds.length} jobs. Use arrows in the modal to navigate between them.`)
      }
      onRefresh()
    } catch (err) {
      toast.error(String(err instanceof Error ? err.message : err))
    }
  }

  const handleGetShareLink = useCallback(async () => {
    if (selectedRows.length !== 1) {
      toast.error('Select exactly one row.')
      return
    }
    const name = String(selectedRows[0].clusterName ?? selectedRows[0].name ?? '')
    setNodeOpen(false)
    setShareLinkLoading(true)
    setShareLinkCopied(false)

    const abort = new AbortController()
    shareLinkAbort.current = abort

    try {
      const { jobId } = await runAction({
        items: [{ clusterName: name, nodeNo: 1 }],
        action: 'getShareLink',
        type: 'agi',
      })

      // Poll until the job completes
      const maxAttempts = 120
      for (let i = 0; i < maxAttempts; i++) {
        if (abort.signal.aborted) return
        await new Promise((r) => setTimeout(r, 1000))
        const job = await fetchJobDetails(jobId)
        if (['completed', 'failed', 'error'].includes(job.status)) {
          if (job.status !== 'completed') {
            throw new Error(job.error || 'Failed to generate share link')
          }
          break
        }
      }

      // Fetch the logs which contain the URL
      const logsResp = await fetchJobLogs(jobId)
      const lines = logsResp.logs.trim().split('\n')
      const url = lines
        .map((l) => l.trim())
        .reverse()
        .find((l) => l.startsWith('http://') || l.startsWith('https://'))
      if (!url) {
        throw new Error('No URL returned from add-auth-token')
      }

      setShareLinkUrl(url)
      setShareLinkOpen(true)
    } catch (err) {
      if (!abort.signal.aborted) {
        toast.error(String(err instanceof Error ? err.message : err))
      }
    } finally {
      setShareLinkLoading(false)
      shareLinkAbort.current = null
    }
  }, [selectedRows, runAction])

  const handleCopyShareLink = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(shareLinkUrl)
      setShareLinkCopied(true)
      toast.success('URL copied to clipboard')
      setTimeout(() => setShareLinkCopied(false), 2000)
    } catch {
      toast.error('Failed to copy to clipboard')
    }
  }, [shareLinkUrl])

  const handleChangeLabel = useCallback((newLabel: string) => {
    setChangeLabelOpen(false)
    if (!newLabel.trim()) {
      toast.error('Label cannot be empty.')
      return
    }
    runBulkAction('changeLabel', { label: newLabel.trim() })
  }, [runBulkAction])

  const handleNodeAction = (path: string) => {
    if (path === 'agi/add-auth-token') {
      handleGetShareLink()
      return
    }
    if (path === 'agi/change-label') {
      if (selectedRows.length !== 1) {
        toast.error('Select exactly one row.')
        return
      }
      const name = String(selectedRows[0].clusterName ?? selectedRows[0].name ?? '')
      setChangeLabelName(name)
      setChangeLabelValue('')
      setChangeLabelOpen(true)
      setNodeOpen(false)
      return
    }
    if (selectedRows.length !== 1) {
      toast.error('Select exactly one row.')
      return
    }
    const name = String(selectedRows[0].clusterName ?? selectedRows[0].name ?? '')
    const params = new URLSearchParams({ name })
    navigate(`/commands/${path}?${params}`)
    setNodeOpen(false)
  }

  const visibleNodeItems = NODE_NAV.filter(({ path }) => cmd(path))

  return (
    <div className="flex flex-wrap items-center gap-2">
      {cmd('agi/create') && (
        <button
          type="button"
          onClick={() => navigate('/commands/agi/create')}
          className="rounded-md bg-green-600 px-3 py-2 text-sm font-medium text-white hover:bg-green-700"
        >
          Create
        </button>
      )}

      {cmd('agi/start') && (
        <button
          type="button"
          onClick={() => runBulkAction('start')}
          disabled={selectedRows.length === 0}
          className="rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600 disabled:opacity-50"
        >
          Start
        </button>
      )}
      {cmd('agi/stop') && (
        <button
          type="button"
          onClick={() => runBulkAction('stop')}
          disabled={selectedRows.length === 0}
          className="rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600 disabled:opacity-50"
        >
          Stop
        </button>
      )}

      {backend !== 'docker' && cmd('instances/change-expiry') && (
        <button
          type="button"
          onClick={() => setExpiryPromptOpen(true)}
          className="rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600"
        >
          Change Expiry
        </button>
      )}

      <button
        type="button"
        onClick={onRefresh}
        className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-200 dark:hover:bg-slate-600"
      >
        Refresh
      </button>

      {visibleNodeItems.length > 0 && (
        <div className="relative">
          <button
            type="button"
            onClick={() => setNodeOpen(!nodeOpen)}
            className="flex items-center gap-1 rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600"
          >
            Node
            <ChevronDown className="h-4 w-4" />
          </button>
          {nodeOpen && (
            <>
              <div
                className="fixed inset-0 z-40"
                onClick={() => setNodeOpen(false)}
                aria-hidden="true"
              />
              <div className="absolute left-0 top-full z-50 mt-1 min-w-[140px] rounded-md border border-slate-200 bg-white py-1 shadow-lg dark:border-slate-700 dark:bg-slate-800">
                {visibleNodeItems.map(({ label, path }) => (
                  <button
                    key={path}
                    type="button"
                    className="block w-full px-4 py-2 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700"
                    onClick={() => handleNodeAction(path)}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      {cmd('agi/destroy') && (
        <button
          type="button"
          onClick={() => selectedRows.length > 0 && setDestroyConfirmOpen(true)}
          disabled={selectedRows.length === 0}
          className={cn(
            'rounded-md px-3 py-2 text-sm font-medium text-white',
            'bg-red-600 hover:bg-red-700 disabled:opacity-50'
          )}
        >
          <Trash2 className="inline h-4 w-4" /> Destroy
        </button>
      )}
      {cmd('agi/delete') && (
        <button
          type="button"
          onClick={() => selectedRows.length > 0 && setDeleteConfirmOpen(true)}
          disabled={selectedRows.length === 0}
          className={cn(
            'rounded-md px-3 py-2 text-sm font-medium text-white',
            'bg-red-600 hover:bg-red-700 disabled:opacity-50'
          )}
        >
          Delete
        </button>
      )}

      <ExpiryPrompt
        isOpen={expiryPromptOpen}
        onClose={() => setExpiryPromptOpen(false)}
        onSubmit={(expiry) => {
          runBulkAction('extendExpiry', { expiry })
          setExpiryPromptOpen(false)
        }}
      />

      <ConfirmDialog
        isOpen={destroyConfirmOpen}
        onClose={() => setDestroyConfirmOpen(false)}
        onConfirm={() => runBulkAction('destroy')}
        title="Destroy"
        message={`Destroy ${selectedRows.length} AGI instance(s)?`}
        confirmText="Destroy"
        danger
      />
      <ConfirmDialog
        isOpen={deleteConfirmOpen}
        onClose={() => setDeleteConfirmOpen(false)}
        onConfirm={() => runBulkAction('delete')}
        title="Delete"
        message={`Delete ${selectedRows.length} AGI instance(s) and their persistent volumes?`}
        confirmText="Delete"
        danger
      />

      {shareLinkLoading && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 dark:bg-black/50">
          <div className="rounded-lg bg-white px-6 py-4 shadow-xl dark:bg-slate-800">
            <div className="flex items-center gap-3">
              <div className="h-5 w-5 animate-spin rounded-full border-2 border-slate-300 border-t-blue-600" />
              <span className="text-sm text-slate-700 dark:text-slate-300">Generating share link...</span>
            </div>
          </div>
        </div>
      )}

      <Modal
        isOpen={changeLabelOpen}
        onClose={() => setChangeLabelOpen(false)}
        title="Change AGI Label"
        footer={
          <>
            <button
              type="button"
              onClick={() => setChangeLabelOpen(false)}
              className="rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-200 dark:hover:bg-slate-600"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => handleChangeLabel(changeLabelValue)}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
            >
              OK
            </button>
          </>
        }
      >
        <p className="mb-3 text-sm text-slate-600 dark:text-slate-400">
          Enter a new label for AGI instance <span className="font-medium text-slate-800 dark:text-slate-200">{changeLabelName}</span>:
        </p>
        <input
          type="text"
          value={changeLabelValue}
          onChange={(e) => setChangeLabelValue(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') handleChangeLabel(changeLabelValue) }}
          placeholder="New label"
          autoFocus
          className="w-full rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-800 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-200"
        />
      </Modal>

      <Modal
        isOpen={shareLinkOpen}
        onClose={() => setShareLinkOpen(false)}
        title="AGI Share Link"
        size="lg"
        footer={
          <>
            <button
              type="button"
              onClick={() => setShareLinkOpen(false)}
              className="rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-200 dark:hover:bg-slate-600"
            >
              Close
            </button>
            <a
              href={shareLinkUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
            >
              Open <ExternalLink className="h-3.5 w-3.5" />
            </a>
          </>
        }
      >
        <p className="mb-3 text-sm text-slate-600 dark:text-slate-400">
          A new authentication token has been created. Use the URL below to access this AGI instance:
        </p>
        <div className="flex items-center gap-2">
          <input
            type="text"
            readOnly
            value={shareLinkUrl}
            className="flex-1 rounded-md border border-slate-300 bg-slate-50 px-3 py-2 text-sm text-slate-800 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-200"
            onClick={(e) => (e.target as HTMLInputElement).select()}
          />
          <button
            type="button"
            onClick={handleCopyShareLink}
            className={cn(
              'rounded-md px-3 py-2 text-sm font-medium text-white',
              shareLinkCopied
                ? 'bg-green-600'
                : 'bg-blue-600 hover:bg-blue-700'
            )}
          >
            <Copy className="h-4 w-4" />
          </button>
        </div>
      </Modal>
    </div>
  )
}
