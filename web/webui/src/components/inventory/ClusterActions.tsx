import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { ChevronDown, Plus, Trash2 } from 'lucide-react'
import { useInventoryAction } from '@/hooks/useInventory'
import { useJobModal } from '@/contexts/JobModalContext'
import { useCommandVisibility } from '@/hooks/useCommandVisibility'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { ExpiryPrompt } from './ExpiryPrompt'
import { cn } from '@/utils/cn'

interface ClusterActionsProps {
  selectedRows: Record<string, unknown>[]
  onRefresh: () => void
  backend: string
}

const CONFIGURE_NAV = [
  { label: 'Rack ID', path: 'conf/rackid' },
  { label: 'Namespace Memory', path: 'conf/namespace-memory' },
  { label: 'Adjust conf', path: 'conf/adjust' },
  { label: 'Fix HB Mesh', path: 'conf/fix-mesh' },
] as const

export function ClusterActions({
  selectedRows,
  onRefresh,
  backend,
}: ClusterActionsProps) {
  const cmd = useCommandVisibility()
  const [nodesOpen, setNodesOpen] = useState(false)
  const [aerospikeOpen, setAerospikeOpen] = useState(false)
  const [configureOpen, setConfigureOpen] = useState(false)
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
        type: 'cluster',
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

  const handleGrow = () => {
    if (selectedRows.length !== 1) {
      toast.error('Select exactly one row.')
      return
    }
    const name = String(selectedRows[0].clusterName ?? selectedRows[0].name ?? '')
    navigate(`/commands/cluster/grow?ClusterName=${encodeURIComponent(name)}`)
  }

  const handleConfigure = (path: string) => {
    if (items.length === 0) {
      toast.error('Select one or more rows.')
      return
    }
    const clusters = [...new Set(items.map((i) => i.clusterName))]
    if (clusters.length > 1) {
      toast.error('All selected nodes must belong to the same cluster.')
      return
    }
    const clusterName = items[0].clusterName
    const nodes = items.map((i) => i.nodeNo).join(',')
    navigate(`/commands/${path}?ClusterName=${encodeURIComponent(clusterName)}&Nodes=${nodes}`)
    setConfigureOpen(false)
  }

  // Map aerospike action names to command paths
  const AEROSPIKE_ACTIONS = [
    { action: 'aerospikeStart', label: 'Start', path: 'aerospike/start' },
    { action: 'aerospikeStop', label: 'Stop', path: 'aerospike/stop' },
    { action: 'aerospikeRestart', label: 'Restart', path: 'aerospike/restart' },
    { action: 'aerospikeStatus', label: 'Status', path: 'aerospike/status' },
  ] as const
  const visibleAerospikeActions = AEROSPIKE_ACTIONS.filter((a) => cmd(a.path))
  const visibleConfigureItems = CONFIGURE_NAV.filter((c) => cmd(c.path))
  const showNodeStart = cmd('cluster/start')
  const showNodeStop = cmd('cluster/stop')

  return (
    <div className="flex flex-wrap items-center gap-2">
      {cmd('cluster/create') && (
        <button
          type="button"
          onClick={() => navigate('/commands/cluster/create')}
          className="flex items-center gap-1 rounded-md bg-green-600 px-3 py-2 text-sm font-medium text-white hover:bg-green-700"
        >
          <Plus className="h-4 w-4" />
          Create
        </button>
      )}
      {cmd('cluster/grow') && (
        <button
          type="button"
          onClick={handleGrow}
          className="flex items-center gap-1 rounded-md bg-green-600 px-3 py-2 text-sm font-medium text-white hover:bg-green-700"
        >
          Grow
        </button>
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

      {visibleAerospikeActions.length > 0 && (
        <div className="relative">
          <button
            type="button"
            onClick={() => setAerospikeOpen(!aerospikeOpen)}
            className="flex items-center gap-1 rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600"
          >
            Aerospike
            <ChevronDown className="h-4 w-4" />
          </button>
          {aerospikeOpen && (
            <>
              <div
                className="fixed inset-0 z-40"
                onClick={() => setAerospikeOpen(false)}
                aria-hidden="true"
              />
              <div className="absolute left-0 top-full z-50 mt-1 min-w-[120px] rounded-md border border-slate-200 bg-white py-1 shadow-lg dark:border-slate-700 dark:bg-slate-800">
                {visibleAerospikeActions.map((a) => (
                  <button
                    key={a.action}
                    type="button"
                    className="block w-full px-4 py-2 text-left text-sm capitalize hover:bg-slate-100 dark:hover:bg-slate-700"
                    onClick={() => {
                      runBulkAction(a.action)
                      setAerospikeOpen(false)
                    }}
                  >
                    {a.label}
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      {visibleConfigureItems.length > 0 && (
        <div className="relative">
          <button
            type="button"
            onClick={() => setConfigureOpen(!configureOpen)}
            className="flex items-center gap-1 rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600"
          >
            Configure
            <ChevronDown className="h-4 w-4" />
          </button>
          {configureOpen && (
            <>
              <div
                className="fixed inset-0 z-40"
                onClick={() => setConfigureOpen(false)}
                aria-hidden="true"
              />
              <div className="absolute left-0 top-full z-50 mt-1 min-w-[160px] rounded-md border border-slate-200 bg-white py-1 shadow-lg dark:border-slate-700 dark:bg-slate-800">
                {visibleConfigureItems.map(({ label, path }) => (
                  <button
                    key={path}
                    type="button"
                    className="block w-full px-4 py-2 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700"
                    onClick={() => handleConfigure(path)}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      {backend !== 'docker' && cmd('cluster/add/expiry') && (
        <button
          type="button"
          onClick={() => selectedRows.length > 0 && setExpiryPromptOpen(true)}
          disabled={selectedRows.length === 0}
          className="rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600 disabled:opacity-50"
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

      {cmd('cluster/destroy') && (
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
        message={`Destroy ${selectedRows.length} node(s)?`}
        confirmText="Destroy"
        danger
      />
    </div>
  )
}
