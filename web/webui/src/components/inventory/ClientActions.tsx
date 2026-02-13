import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { ChevronDown, Trash2 } from 'lucide-react'
import { useInventoryAction } from '@/hooks/useInventory'
import { useJobModal } from '@/contexts/JobModalContext'
import { useCommandVisibility } from '@/hooks/useCommandVisibility'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { ExpiryPrompt } from './ExpiryPrompt'
import { cn } from '@/utils/cn'

interface ClientActionsProps {
  selectedRows: Record<string, unknown>[]
  onRefresh: () => void
  backend: string
}

const CLIENT_TYPES = [
  { label: 'Vanilla', type: 'none' },
  { label: 'Base', type: 'base' },
  { label: 'AerospikeTools', type: 'tools' },
  { label: 'AMS', type: 'ams' },
  { label: 'VSCode', type: 'vscode' },
  { label: 'Trino', type: 'trino' },
  { label: 'ElasticSearch', type: 'elasticsearch' },
  { label: 'RestGateway', type: 'rest-gateway' },
  { label: 'Graph', type: 'graph' },
  { label: 'Vector', type: 'vector' },
  { label: 'EksCtl', type: 'eksctl' },
] as const

export function ClientActions({
  selectedRows,
  onRefresh,
  backend,
}: ClientActionsProps) {
  const cmd = useCommandVisibility()
  const [createOpen, setCreateOpen] = useState(false)
  const [growOpen, setGrowOpen] = useState(false)
  const [nodesOpen, setNodesOpen] = useState(false)
  const [expiryPromptOpen, setExpiryPromptOpen] = useState(false)
  const [destroyConfirmOpen, setDestroyConfirmOpen] = useState(false)
  const { mutateAsync: runAction } = useInventoryAction()
  const { openJobModal } = useJobModal()
  const navigate = useNavigate()

  const items = selectedRows.map((r) => ({
    clusterName: String(r.clusterName ?? r.name ?? ''),
    nodeNo: Number(r.nodeNo ?? r.node ?? 0),
  }))

  const runBulkAction = async (
    action: string,
    params?: Record<string, unknown>
  ) => {
    try {
      const { jobId, jobIds } = await runAction({
        items,
        action,
        type: 'client',
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

  const handleGrow = (type: string) => {
    if (selectedRows.length !== 1) {
      toast.error('Select exactly one row.')
      return
    }
    const name = String(selectedRows[0].clusterName ?? selectedRows[0].name ?? '')
    navigate(`/commands/client/grow/${type}?ClientName=${encodeURIComponent(name)}`)
    setGrowOpen(false)
  }

  const visibleCreateTypes = CLIENT_TYPES.filter(({ type }) => cmd(`client/create/${type}`))
  const visibleGrowTypes = CLIENT_TYPES.filter(({ type }) => cmd(`client/grow/${type}`))
  const showNodeStart = cmd('client/start')
  const showNodeStop = cmd('client/stop')

  return (
    <div className="flex flex-wrap items-center gap-2">
      {visibleCreateTypes.length > 0 && (
        <div className="relative">
          <button
            type="button"
            onClick={() => setCreateOpen(!createOpen)}
            className="flex items-center gap-1 rounded-md bg-green-600 px-3 py-2 text-sm font-medium text-white hover:bg-green-700"
          >
            Create
            <ChevronDown className="h-4 w-4" />
          </button>
          {createOpen && (
            <>
              <div
                className="fixed inset-0 z-40"
                onClick={() => setCreateOpen(false)}
                aria-hidden="true"
              />
              <div className="absolute left-0 top-full z-50 mt-1 max-h-64 min-w-[140px] overflow-y-auto rounded-md border border-slate-200 bg-white py-1 shadow-lg dark:border-slate-700 dark:bg-slate-800">
                {visibleCreateTypes.map(({ label, type }) => (
                  <button
                    key={type}
                    type="button"
                    className="block w-full px-4 py-2 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700"
                    onClick={() => {
                      navigate(`/commands/client/create/${type}`)
                      setCreateOpen(false)
                    }}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      {visibleGrowTypes.length > 0 && (
        <div className="relative">
          <button
            type="button"
            onClick={() => setGrowOpen(!growOpen)}
            className="flex items-center gap-1 rounded-md bg-green-600 px-3 py-2 text-sm font-medium text-white hover:bg-green-700"
          >
            Grow
            <ChevronDown className="h-4 w-4" />
          </button>
          {growOpen && (
            <>
              <div
                className="fixed inset-0 z-40"
                onClick={() => setGrowOpen(false)}
                aria-hidden="true"
              />
              <div className="absolute left-0 top-full z-50 mt-1 max-h-64 min-w-[140px] overflow-y-auto rounded-md border border-slate-200 bg-white py-1 shadow-lg dark:border-slate-700 dark:bg-slate-800">
                {visibleGrowTypes.map(({ label, type }) => (
                  <button
                    key={type}
                    type="button"
                    className="block w-full px-4 py-2 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700"
                    onClick={() => handleGrow(type)}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      {(showNodeStart || showNodeStop) && (
        <div className="relative">
          <button
            type="button"
            onClick={() => setNodesOpen(!nodesOpen)}
            className="flex items-center gap-1 rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600"
          >
            Nodes
            <ChevronDown className="h-4 w-4" />
          </button>
          {nodesOpen && (
            <>
              <div
                className="fixed inset-0 z-40"
                onClick={() => setNodesOpen(false)}
                aria-hidden="true"
              />
              <div className="absolute left-0 top-full z-50 mt-1 min-w-[100px] rounded-md border border-slate-200 bg-white py-1 shadow-lg dark:border-slate-700 dark:bg-slate-800">
                {showNodeStart && (
                  <button
                    type="button"
                    className="block w-full px-4 py-2 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700"
                    onClick={() => {
                      runBulkAction('start')
                      setNodesOpen(false)
                    }}
                  >
                    Start
                  </button>
                )}
                {showNodeStop && (
                  <button
                    type="button"
                    className="block w-full px-4 py-2 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700"
                    onClick={() => {
                      runBulkAction('stop')
                      setNodesOpen(false)
                    }}
                  >
                    Stop
                  </button>
                )}
              </div>
            </>
          )}
        </div>
      )}

      {backend !== 'docker' && cmd('client/configure/expiry') && (
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

      {cmd('client/destroy') && (
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
        message={`Destroy ${selectedRows.length} client(s)?`}
        confirmText="Destroy"
        danger
      />
    </div>
  )
}
