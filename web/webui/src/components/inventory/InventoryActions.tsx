import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { Trash2 } from 'lucide-react'
import { executeCommand } from '@/api/client'
import { useJobModal } from '@/contexts/JobModalContext'
import { useCommandVisibility } from '@/hooks/useCommandVisibility'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { ClusterActions } from './ClusterActions'
import { ClientActions } from './ClientActions'
import { AGIActions } from './AGIActions'

interface InventoryActionsProps {
  activeTab: string
  selectedRows: Record<string, unknown>[]
  onRefresh: () => void
  backend: string
}

export function InventoryActions({
  activeTab,
  selectedRows,
  onRefresh,
  backend,
}: InventoryActionsProps) {
  const navigate = useNavigate()
  const cmd = useCommandVisibility()

  switch (activeTab) {
    case 'clusters':
      return (
        <ClusterActions
          selectedRows={selectedRows}
          onRefresh={onRefresh}
          backend={backend}
        />
      )
    case 'clients':
      return (
        <ClientActions
          selectedRows={selectedRows}
          onRefresh={onRefresh}
          backend={backend}
        />
      )
    case 'agi':
      return (
        <AGIActions
          selectedRows={selectedRows}
          onRefresh={onRefresh}
          backend={backend}
        />
      )
    case 'templates':
      return (
        <TemplateActions
          selectedRows={selectedRows}
          onRefresh={onRefresh}
        />
      )
    case 'agi-templates':
      return (
        <AGITemplateActions
          selectedRows={selectedRows}
          onRefresh={onRefresh}
        />
      )
    case 'volumes':
      if (backend === 'docker') return <RefreshOnlyActions onRefresh={onRefresh} />
      return (
        <SimpleTabActions
          onCreate={cmd('volumes/create') ? () => navigate('/commands/volumes/create') : undefined}
          createLabel="Create"
          secondaryLabel={cmd('volumes/attach') ? 'Mount' : undefined}
          tertiaryLabel={cmd('volumes/grow') ? 'Grow' : undefined}
          onSecondary={cmd('volumes/attach') ? () => navigate('/commands/volumes/attach') : undefined}
          onTertiary={cmd('volumes/grow') ? () => navigate('/commands/volumes/grow') : undefined}
          onRefresh={onRefresh}
          onDelete={cmd('volumes/detach') ? () => navigate('/commands/volumes/detach') : undefined}
          deleteLabel="Detach"
          selectedRows={selectedRows}
        />
      )
    case 'firewalls':
      if (backend === 'docker') return <RefreshOnlyActions onRefresh={onRefresh} />
      {
        const createPath = backend === 'aws' ? 'config/aws/create-security-groups' : 'config/gcp/create-firewall-rules'
        const lockPath = backend === 'aws' ? 'config/aws/lock-security-groups' : 'config/gcp/lock-firewall-rules'
        const deletePath = backend === 'aws' ? 'config/aws/delete-security-groups' : 'config/gcp/delete-firewall-rules'
        return (
          <SimpleTabActions
            onCreate={cmd(createPath) ? () => navigate(`/commands/${createPath}`) : undefined}
            createLabel="Create"
            secondaryLabel={cmd(lockPath) ? 'Lock IP' : undefined}
            onSecondary={cmd(lockPath) ? () => navigate(`/commands/${lockPath}`) : undefined}
            onRefresh={onRefresh}
            onDelete={cmd(deletePath) ? () => navigate(`/commands/${deletePath}`) : undefined}
            selectedRows={selectedRows}
          />
        )
      }
    case 'subnets':
      return <RefreshOnlyActions onRefresh={onRefresh} />
    case 'instance-types':
      return <RefreshOnlyActions onRefresh={onRefresh} />
    case 'expiry':
      if (backend === 'docker') return <RefreshOnlyActions onRefresh={onRefresh} />
      {
        const installPath = backend === 'aws' ? 'config/aws/expiry-install' : 'config/gcp/expiry-install'
        const freqPath = backend === 'aws' ? 'config/aws/expiry-run-frequency' : 'config/gcp/expiry-run-frequency'
        const removePath = backend === 'aws' ? 'config/aws/expiry-remove' : 'config/gcp/expiry-remove'
        return (
          <SimpleTabActions
            onCreate={cmd(installPath) ? () => navigate(`/commands/${installPath}`) : undefined}
            createLabel="Create"
            secondaryLabel={cmd(freqPath) ? 'Change Frequency' : undefined}
            onSecondary={cmd(freqPath) ? () => navigate(`/commands/${freqPath}`) : undefined}
            onRefresh={onRefresh}
            onDelete={cmd(removePath) ? () => navigate(`/commands/${removePath}`) : undefined}
            deleteLabel="Remove"
            selectedRows={selectedRows}
          />
        )
      }
    default:
      return null
  }
}

function SimpleTabActions({
  onCreate,
  createLabel,
  secondaryLabel,
  tertiaryLabel,
  onSecondary,
  onTertiary,
  onRefresh,
  onDelete,
  deleteLabel = 'Delete',
  selectedRows,
}: {
  onCreate?: () => void
  createLabel: string
  secondaryLabel?: string
  tertiaryLabel?: string
  onSecondary?: () => void
  onTertiary?: () => void
  onRefresh: () => void
  onDelete?: () => void
  deleteLabel?: string
  selectedRows: Record<string, unknown>[]
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {onCreate && (
        <button
          type="button"
          onClick={onCreate}
          className="rounded-md bg-green-600 px-3 py-2 text-sm font-medium text-white hover:bg-green-700 dark:bg-green-500 dark:hover:bg-green-600"
        >
          {createLabel}
        </button>
      )}
      {secondaryLabel && onSecondary && (
        <button
          type="button"
          onClick={onSecondary}
          className="rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600 dark:bg-amber-600 dark:hover:bg-amber-700"
        >
          {secondaryLabel}
        </button>
      )}
      {tertiaryLabel && onTertiary && (
        <button
          type="button"
          onClick={onTertiary}
          className="rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600 dark:bg-amber-600 dark:hover:bg-amber-700"
        >
          {tertiaryLabel}
        </button>
      )}
      <button
        type="button"
        onClick={onRefresh}
        className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-200 dark:hover:bg-slate-600"
      >
        Refresh
      </button>
      {onDelete && (
        <button
          type="button"
          onClick={onDelete}
          disabled={selectedRows.length === 0}
          className="rounded-md bg-red-600 px-3 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50 dark:bg-red-500 dark:hover:bg-red-600"
        >
          {deleteLabel}
        </button>
      )}
    </div>
  )
}

function TemplateActions({
  selectedRows,
  onRefresh,
}: {
  selectedRows: Record<string, unknown>[]
  onRefresh: () => void
}) {
  const navigate = useNavigate()
  const cmd = useCommandVisibility()
  const { openJobModal } = useJobModal()
  const [destroyConfirmOpen, setDestroyConfirmOpen] = useState(false)

  const handleDestroy = async () => {
    if (selectedRows.length === 0) return
    let firstJobId: string | null = null
    for (const row of selectedRows) {
      const tags = (row.tags ?? {}) as Record<string, string>
      const params: Record<string, unknown> = {
        distro: String(row.osName ?? ''),
        'distro-version': String(row.osVersion ?? ''),
        arch: String(row.architecture ?? ''),
        'aerospike-version': String(tags['aerolab.soft.version'] ?? ''),
        force: true,
      }
      try {
        const res = await executeCommand('template/destroy', params)
        if (!firstJobId) firstJobId = res.jobId
      } catch (err) {
        toast.error(`Failed to delete template: ${err instanceof Error ? err.message : err}`)
      }
    }
    if (firstJobId) openJobModal(firstJobId)
    onRefresh()
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {cmd('template/create') && (
        <button
          type="button"
          onClick={() => navigate('/commands/template/create')}
          className="rounded-md bg-green-600 px-3 py-2 text-sm font-medium text-white hover:bg-green-700"
        >
          Create Template
        </button>
      )}
      {cmd('cluster/create') && (
        <button
          type="button"
          onClick={() => navigate('/commands/cluster/create')}
          className="rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600"
        >
          Create Cluster from template
        </button>
      )}
      <button
        type="button"
        onClick={onRefresh}
        className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-200 dark:hover:bg-slate-600"
      >
        Refresh
      </button>
      {cmd('template/destroy') && (
        <button
          type="button"
          onClick={() => selectedRows.length > 0 && setDestroyConfirmOpen(true)}
          disabled={selectedRows.length === 0}
          className="rounded-md bg-red-600 px-3 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
        >
          <Trash2 className="inline h-4 w-4" /> Delete
        </button>
      )}
      <ConfirmDialog
        isOpen={destroyConfirmOpen}
        onClose={() => setDestroyConfirmOpen(false)}
        onConfirm={handleDestroy}
        title="Delete Templates"
        message={`Delete ${selectedRows.length} template(s)? This cannot be undone.`}
        confirmText="Delete"
        danger
      />
    </div>
  )
}

function AGITemplateActions({
  selectedRows,
  onRefresh,
}: {
  selectedRows: Record<string, unknown>[]
  onRefresh: () => void
}) {
  const navigate = useNavigate()
  const cmd = useCommandVisibility()
  const { openJobModal } = useJobModal()
  const [destroyConfirmOpen, setDestroyConfirmOpen] = useState(false)

  const handleDestroy = async () => {
    if (selectedRows.length === 0) return
    setDestroyConfirmOpen(false)
    let firstJobId: string | null = null
    for (const row of selectedRows) {
      const tags = (row.tags ?? {}) as Record<string, string>
      const agiVer = tags['aerolab.agi.version']
      const params: Record<string, unknown> = {
        arch: String(row.architecture ?? ''),
        'aerospike-version': String(tags['aerolab.agi.aerospike'] ?? ''),
        'grafana-version': String(tags['aerolab.agi.grafana'] ?? ''),
        force: true,
      }
      if (agiVer && !Number.isNaN(Number(agiVer))) {
        params['agi-version'] = Number(agiVer)
      }
      try {
        const res = await executeCommand('agi/template/destroy', params)
        if (!firstJobId) firstJobId = res.jobId
      } catch (err) {
        toast.error(`Failed to delete AGI template: ${err instanceof Error ? err.message : err}`)
      }
    }
    if (firstJobId) openJobModal(firstJobId)
    onRefresh()
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {cmd('agi/template/create') && (
        <button
          type="button"
          onClick={() => navigate('/commands/agi/template/create')}
          className="rounded-md bg-green-600 px-3 py-2 text-sm font-medium text-white hover:bg-green-700 dark:bg-green-500 dark:hover:bg-green-600"
        >
          Create AGI Template
        </button>
      )}
      {cmd('agi/create') && (
        <button
          type="button"
          onClick={() => navigate('/commands/agi/create')}
          className="rounded-md bg-amber-500 px-3 py-2 text-sm font-medium text-white hover:bg-amber-600 dark:bg-amber-600 dark:hover:bg-amber-700"
        >
          Create AGI from template
        </button>
      )}
      <button
        type="button"
        onClick={onRefresh}
        className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-200 dark:hover:bg-slate-600"
      >
        Refresh
      </button>
      {cmd('agi/template/destroy') && (
        <button
          type="button"
          onClick={() => selectedRows.length > 0 && setDestroyConfirmOpen(true)}
          disabled={selectedRows.length === 0}
          className="rounded-md bg-red-600 px-3 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50 dark:bg-red-500 dark:hover:bg-red-600"
        >
          <Trash2 className="inline h-4 w-4" /> Delete
        </button>
      )}
      <ConfirmDialog
        isOpen={destroyConfirmOpen}
        onClose={() => setDestroyConfirmOpen(false)}
        onConfirm={handleDestroy}
        title="Delete AGI Templates"
        message={`Delete ${selectedRows.length} AGI template(s)? This cannot be undone.`}
        confirmText="Delete"
        danger
      />
    </div>
  )
}

function RefreshOnlyActions({ onRefresh }: { onRefresh: () => void }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <button
        type="button"
        onClick={onRefresh}
        className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-200 dark:hover:bg-slate-600"
      >
        Refresh
      </button>
    </div>
  )
}
