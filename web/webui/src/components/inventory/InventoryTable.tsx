import { useState, useMemo } from 'react'
import { Search, RefreshCw } from 'lucide-react'
import { cn } from '@/utils/cn'

export interface InventoryColumn {
  name: string
  field: string
  render?: (value: unknown, row: Record<string, unknown>) => React.ReactNode
}

interface InventoryTableProps {
  data: Record<string, unknown>[]
  columns: InventoryColumn[]
  selectedRows: Record<string, unknown>[]
  onSelectionChange: (rows: Record<string, unknown>[]) => void
  isLoading: boolean
  onRefresh: () => void
}

function getNestedValue(obj: Record<string, unknown>, path: string): unknown {
  const parts = path.split('.')
  let current: unknown = obj
  for (const part of parts) {
    if (current == null) return undefined
    current = (current as Record<string, unknown>)[part]
  }
  return current
}

export function InventoryTable({
  data,
  columns,
  selectedRows,
  onSelectionChange,
  isLoading,
  onRefresh,
}: InventoryTableProps) {
  const [search, setSearch] = useState('')
  const [sortConfig, setSortConfig] = useState<{ field: string; asc: boolean } | null>(null)

  const filteredData = useMemo(() => {
    if (!search.trim()) return data
    const lower = search.toLowerCase()
    return data.filter((row) => {
      return columns.some((col) => {
        const val = getNestedValue(row, col.field)
        return String(val ?? '').toLowerCase().includes(lower)
      })
    })
  }, [data, search, columns])

  const sortedData = useMemo(() => {
    if (!sortConfig) return filteredData
    return [...filteredData].sort((a, b) => {
      const aVal = getNestedValue(a, sortConfig.field)
      const bVal = getNestedValue(b, sortConfig.field)
      const aStr = String(aVal ?? '')
      const bStr = String(bVal ?? '')
      const cmp = aStr.localeCompare(bStr, undefined, { numeric: true })
      return sortConfig.asc ? cmp : -cmp
    })
  }, [filteredData, sortConfig])

  const handleSort = (field: string) => {
    setSortConfig((prev) =>
      prev?.field === field ? { field, asc: !prev.asc } : { field, asc: true }
    )
  }

  const getRowKey = (row: Record<string, unknown>) =>
    JSON.stringify({
      clusterName: row.clusterName ?? row.name ?? row.Name,
      nodeNo: row.nodeNo ?? row.node,
      name: row.name ?? row.Name,
      imageId: row.imageId,
      subnetId: row.subnetId,
      backendType: row.backendType,
      zone: row.zone,
      version: row.version,
      region: row.region ?? row.Region,
    })

  const isSelected = (row: Record<string, unknown>) =>
    selectedRows.some((r) => getRowKey(r) === getRowKey(row))

  const toggleRow = (row: Record<string, unknown>) => {
    const key = getRowKey(row)
    const selectedSet = new Set(selectedRows.map(getRowKey))
    if (selectedSet.has(key)) {
      onSelectionChange(selectedRows.filter((r) => getRowKey(r) !== key))
    } else {
      onSelectionChange([...selectedRows, row])
    }
  }

  const toggleAll = () => {
    if (selectedRows.length === sortedData.length) {
      onSelectionChange([])
    } else {
      onSelectionChange([...sortedData])
    }
  }

  const allSelected = sortedData.length > 0 && selectedRows.length === sortedData.length
  const someSelected = selectedRows.length > 0

  return (
    <div className="overflow-hidden rounded-lg border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
      <div className="flex items-center gap-4 border-b border-slate-200 px-4 py-3 dark:border-slate-700">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            placeholder="Search..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full rounded-md border border-slate-300 py-2 pl-9 pr-3 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-100"
          />
        </div>
        <button
          type="button"
          onClick={onRefresh}
          disabled={isLoading}
          className="flex items-center gap-2 rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-200 dark:hover:bg-slate-600"
        >
          <RefreshCw className={cn('h-4 w-4', isLoading && 'animate-spin')} />
          Refresh
        </button>
      </div>

      <div className="overflow-x-auto">
        {isLoading ? (
          <div className="space-y-3 p-6">
            {[1, 2, 3, 4, 5].map((i) => (
              <div key={i} className="flex gap-4">
                <div className="h-4 w-8 animate-pulse rounded bg-slate-200 dark:bg-slate-600" />
                <div className="h-4 flex-1 animate-pulse rounded bg-slate-200 dark:bg-slate-600" />
                <div className="h-4 w-24 animate-pulse rounded bg-slate-200 dark:bg-slate-600" />
              </div>
            ))}
          </div>
        ) : sortedData.length === 0 ? (
          <div className="py-12 text-center text-slate-500 dark:text-slate-400">
            No data to display
          </div>
        ) : (
          <table className="w-full min-w-max border-collapse">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 dark:border-slate-700 dark:bg-slate-900/50">
                <th className="sticky left-0 z-10 border-b border-r border-slate-200 bg-slate-50 px-4 py-3 text-left dark:border-slate-700 dark:bg-slate-900/50">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    ref={(el) => {
                      if (el) el.indeterminate = someSelected && !allSelected
                    }}
                    onChange={toggleAll}
                    className="rounded border-slate-300"
                  />
                </th>
                {columns.map((col) => (
                  <th
                    key={col.field}
                    className="cursor-pointer border-b border-slate-200 px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
                    onClick={() => handleSort(col.field)}
                  >
                    <span className="flex items-center gap-1">
                      {col.name}
                      {sortConfig?.field === col.field && (
                        <span className="text-slate-400">{sortConfig.asc ? '↑' : '↓'}</span>
                      )}
                    </span>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sortedData.map((row, idx) => {
                const selected = isSelected(row)
                return (
                  <tr
                    key={idx}
                    className={cn(
                      'border-b border-slate-100 transition-colors dark:border-slate-700/50',
                      idx % 2 === 0 ? 'bg-white dark:bg-slate-800' : 'bg-slate-50/50 dark:bg-slate-800/50',
                      selected && 'border-l-4 border-l-blue-500 bg-blue-50/30 dark:bg-blue-900/20',
                      'hover:bg-slate-100 dark:hover:bg-slate-700/50'
                    )}
                  >
                    <td className="sticky left-0 z-10 border-r border-slate-200 bg-inherit px-4 py-2 dark:border-slate-700">
                      <input
                        type="checkbox"
                        checked={selected}
                        onChange={() => toggleRow(row)}
                        className="rounded border-slate-300"
                      />
                    </td>
                    {columns.map((col) => {
                      const val = getNestedValue(row, col.field)
                      const cell = col.render ? col.render(val, row) : String(val ?? '')
                      return (
                        <td
                          key={col.field}
                          className="whitespace-nowrap px-4 py-2 text-sm text-slate-700 dark:text-slate-300"
                        >
                          {cell}
                        </td>
                      )
                    })}
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
