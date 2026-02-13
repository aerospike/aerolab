import React, { useState, useMemo } from 'react'
import { useInventory, useInventorySchema } from '@/hooks/useInventory'
import { usePreferences } from '@/hooks/usePreferences'
import { useCommandVisibility } from '@/hooks/useCommandVisibility'
import { InventoryTable } from './InventoryTable'
import { InventoryActions } from './InventoryActions'
import { ConnectButton, AttachButton } from './ConnectButton'
import type { InventoryColumn } from './InventoryTable'
import { cn } from '@/utils/cn'
import { formatExpiresIn } from '@/utils/formatTime'

/**
 * Each tab maps to one or more command paths.  If ANY mapped command is
 * visible the tab stays visible.  'subnets' has no associated command so
 * it is always shown (when the backend supports it).
 */
const TABS: readonly { id: string; label: string; commands?: string[] }[] = [
  { id: 'clusters', label: 'Clusters', commands: ['cluster'] },
  { id: 'clients', label: 'Clients', commands: ['client'] },
  { id: 'agi', label: 'AGI', commands: ['agi'] },
  { id: 'templates', label: 'Templates', commands: ['template'] },
  { id: 'agi-templates', label: 'AGI Templates', commands: ['agi/template'] },
  { id: 'volumes', label: 'Volumes', commands: ['volumes'] },
  { id: 'firewalls', label: 'Firewalls', commands: ['config'] },
  { id: 'subnets', label: 'Subnets' },
  { id: 'expiry', label: 'Expiry', commands: ['config'] },
  { id: 'instance-types', label: 'Instance Types' },
] as const

export function InventoryPageContent() {
  const { inventoryTab, setInventoryTab } = usePreferences()
  const { schema, backend, isLoading: schemaLoading } = useInventorySchema()
  const { data, isLoading: dataLoading, error: dataError, refetch } = useInventory(inventoryTab)
  const [selectedRows, setSelectedRows] = useState<Record<string, unknown>[]>([])

  const isAllowed = useCommandVisibility()

  const visibleTabs = useMemo(() => {
    let tabs = [...TABS]
    // Filter by backend capabilities
    if (backend === 'docker') {
      tabs = tabs.filter((t) => !['volumes', 'firewalls', 'subnets', 'expiry', 'instance-types'].includes(t.id))
    } else if (backend === 'gcp') {
      tabs = tabs.filter((t) => t.id !== 'subnets')
    }
    // Filter by simple mode: hide tabs whose commands are all blocked
    tabs = tabs.filter((t) => {
      if (!t.commands || t.commands.length === 0) return true
      return t.commands.some((cmd) => isAllowed(cmd))
    })
    return tabs
  }, [backend, isAllowed])

  const entitySchema = schema?.entities?.[inventoryTab] as Array<{ name: string; field: string }> | undefined
  const columns: InventoryColumn[] = useMemo(() => {
    if (!entitySchema) return []
    let cols = entitySchema.map((col) => {
      const base = { name: col.name, field: col.field }
      if (col.field === 'firewalls') {
        return {
          ...base,
          render: (val: unknown) => {
            if (Array.isArray(val)) return val.join('\n')
            return String(val ?? '')
          },
        }
      }
      if (
        col.field === 'estimatedCost' ||
        col.field.includes('Cost') ||
        col.field.includes('Price')
      ) {
        return {
          ...base,
          render: (val: unknown) => {
            const n = Number(val ?? 0)
            if (n === 0 || Number.isNaN(n)) return '-'
            return `$${n.toFixed(2)}`
          },
        }
      }
      if (col.field === 'lifeCycleState' && ['clusters', 'clients', 'agi'].includes(inventoryTab)) {
        return {
          ...base,
          render: (val: unknown) => {
            const state = String(val ?? '')
            const lower = state.toLowerCase()
            const color =
              lower === 'running'
                ? 'text-green-600 dark:text-green-400'
                : lower === 'stopped'
                  ? 'text-red-600 dark:text-red-400'
                  : 'text-amber-600 dark:text-amber-400'
            return (
              <span className={`inline-flex items-center gap-1.5 font-medium ${color}`}>
                <span
                  className={`h-2 w-2 rounded-full ${
                    lower === 'running'
                      ? 'bg-green-500'
                      : lower === 'stopped'
                        ? 'bg-red-500'
                        : 'bg-amber-500'
                  }`}
                />
                {state}
              </span>
            )
          },
        }
      }
      if (col.field.toLowerCase().includes('status') && inventoryTab === 'agi') {
        return {
          ...base,
          render: (val: unknown) => {
            const raw = String(val ?? '')
            const s = raw.toLowerCase()
            // Not yet fetched
            if (!raw || raw === 'undefined' || raw === 'null') {
              return (
                <span className="inline-flex items-center gap-1 text-slate-400 dark:text-slate-500">
                  <span className="h-2 w-2 rounded-full bg-slate-400 animate-pulse" /> checking...
                </span>
              )
            }
            // READY with no issues - green
            if (s === 'ready') {
              return (
                <span className="inline-flex items-center gap-1 text-green-600 dark:text-green-400">
                  <span className="h-2 w-2 rounded-full bg-green-500" /> READY
                </span>
              )
            }
            // READY with warnings (HasErrors) - amber/warning
            if (s.startsWith('ready')) {
              return (
                <span className="inline-flex items-center gap-1 text-amber-600 dark:text-amber-400">
                  <span className="h-2 w-2 rounded-full bg-amber-500" /> {raw}
                </span>
              )
            }
            // ERR: states - red
            if (s.startsWith('err:') || s === 'unreachable') {
              return (
                <span className="inline-flex items-center gap-1 text-red-600 dark:text-red-400">
                  <span className="h-2 w-2 rounded-full bg-red-500" /> {raw}
                </span>
              )
            }
            // Progress states: (1/6) INIT, (2/6) DOWNLOAD 45%, etc. - amber/in-progress
            return (
              <span className="inline-flex items-center gap-1 text-amber-600 dark:text-amber-400">
                <span className="h-2 w-2 rounded-full bg-amber-500 animate-pulse" /> {raw}
              </span>
            )
          },
        }
      }
      if (col.field === 'expires') {
        return {
          ...base,
          render: (val: unknown) => {
            const { label, color } = formatExpiresIn(val)
            return <span className={color}>{label}</span>
          },
        }
      }
      return base
    })
    // Pull out instanceId column so we can push it to the very end
    const instanceIdCol = cols.find((c) => c.field === 'instanceId')
    if (instanceIdCol) {
      cols = cols.filter((c) => c.field !== 'instanceId')
    }
    if (inventoryTab === 'clusters') {
      const attachCol: InventoryColumn = {
        name: 'Attach',
        field: '_attach',
        render: (_val: unknown, row: Record<string, unknown>): React.ReactNode => {
          const running = String(row.lifeCycleState ?? '').toLowerCase() === 'running'
          const name = String(row.clusterName ?? row.name ?? '')
          const nodeNo = Number(row.nodeNo ?? row.node ?? 0)
          return <AttachButton type="cluster" name={name} nodeNo={nodeNo} isRunning={running} />
        },
      }
      cols = [...cols, attachCol]
    }
    if (inventoryTab === 'clients') {
      const attachCol: InventoryColumn = {
        name: 'Attach',
        field: '_attach',
        render: (_val: unknown, row: Record<string, unknown>): React.ReactNode => {
          const running = String(row.lifeCycleState ?? '').toLowerCase() === 'running'
          const name = String(row.clusterName ?? row.name ?? '')
          const nodeNo = Number(row.nodeNo ?? row.node ?? 0)
          return <AttachButton type="client" name={name} nodeNo={nodeNo} isRunning={running} />
        },
      }
      cols = [...cols, attachCol]
    }
    if (inventoryTab === 'agi') {
      const attachCol: InventoryColumn = {
        name: 'Attach',
        field: '_attach',
        render: (_val: unknown, row: Record<string, unknown>): React.ReactNode => {
          const running = String(row.lifeCycleState ?? '').toLowerCase() === 'running'
          const name = String(row.clusterName ?? row.name ?? '')
          return (
            <span className="inline-flex items-center gap-1">
              <AttachButton type="agi" name={name} nodeNo={1} isRunning={running} />
              <ConnectButton type="agi" name={name} nodeNo={1} isRunning={running} />
            </span>
          )
        },
      }
      cols = [...cols, attachCol]
    }
    if (inventoryTab === 'clients') {
      const connectCol: InventoryColumn = {
        name: 'Connect',
        field: 'accessURL',
        render: (val: unknown, row: Record<string, unknown>): React.ReactNode => {
          const tags = (row.tags ?? {}) as Record<string, string>
          const clientType = String(tags['aerolab.client.type'] ?? '')
          const running = String(row.lifeCycleState ?? '').toLowerCase() === 'running'
          const name = String(row.clusterName ?? row.name ?? '')
          const nodeNo = Number(row.nodeNo ?? row.node ?? 0)
          if (clientType === 'trino') {
            return <ConnectButton type="trino" name={name} nodeNo={nodeNo} isRunning={running} />
          }
          if (clientType === 'graph') {
            return <ConnectButton type="graph" name={name} nodeNo={nodeNo} isRunning={running} />
          }
          const url = String(row.accessURL ?? val ?? '')
          if (url) {
            return (
              <a href={url} target="_blank" rel="noreferrer" className="text-blue-600 hover:underline dark:text-blue-400">
                {url}
              </a>
            )
          }
          return '-'
        },
      }
      cols = [...cols, connectCol]
    }
    // Instance ID is always the very last column
    if (instanceIdCol) {
      cols = [...cols, instanceIdCol]
    }
    return cols
  }, [entitySchema, inventoryTab])

  const activeTab = visibleTabs.some((t) => t.id === inventoryTab)
    ? inventoryTab
    : visibleTabs[0]?.id ?? 'clusters'

  const handleTabChange = (tabId: string) => {
    setInventoryTab(tabId)
    setSelectedRows([])
  }

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">
        Inventory
      </h1>

      <div className="border-b border-slate-200 dark:border-slate-700">
        <nav className="-mb-px flex gap-1" aria-label="Inventory tabs">
          {visibleTabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              onClick={() => handleTabChange(tab.id)}
              className={cn(
                'border-b-2 px-4 py-2 text-sm font-medium transition-colors',
                activeTab === tab.id
                  ? 'border-blue-500 text-blue-600 dark:border-blue-400 dark:text-blue-400'
                  : 'border-transparent text-slate-500 hover:border-slate-300 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-300'
              )}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <InventoryActions
          activeTab={activeTab}
          selectedRows={selectedRows}
          onRefresh={() => refetch()}
          backend={backend}
        />
      </div>

      {dataError && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-4 dark:border-red-800 dark:bg-red-900/20">
          <p className="mb-3 text-red-800 dark:text-red-200">
            Failed to load inventory: {String(dataError instanceof Error ? dataError.message : dataError)}
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
          >
            Retry
          </button>
        </div>
      )}
      <InventoryTable
        data={data}
        columns={columns}
        selectedRows={selectedRows}
        onSelectionChange={setSelectedRows}
        isLoading={schemaLoading || dataLoading}
        onRefresh={() => refetch()}
      />
    </div>
  )
}
