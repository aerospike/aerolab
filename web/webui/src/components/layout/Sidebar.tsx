import { NavLink, useLocation } from 'react-router-dom'
import { ChevronDown, ChevronRight, Database, ListTodo } from 'lucide-react'
import { useState } from 'react'
import { useCommandTree } from '@/hooks/useCommands'
import { usePreferences } from '@/hooks/usePreferences'
import { getIcon } from '@/utils/icons'
import { cn } from '@/utils/cn'
import type { CommandInfo } from '@/api/types'

interface SidebarProps {
  collapsed: boolean
  onToggleCollapsed: () => void
}

/** Renders tree-line guides for nested sidebar items */
function TreeGuides({ depth }: { depth: number }) {
  if (depth === 0) return null
  return (
    <>
      {Array.from({ length: depth }, (_, i) => (
        <span
          key={i}
          className="absolute top-0 bottom-0 w-px bg-slate-400/70"
          style={{ left: `${14 + i * 28}px` }}
        />
      ))}
    </>
  )
}

function SidebarItem({
  cmd,
  basePath,
  simpleMode,
  depth,
}: {
  cmd: CommandInfo
  basePath: string
  simpleMode: boolean
  depth: number
}) {
  const location = useLocation()
  const hasChildren = cmd.hasChildren && (cmd.children?.length ?? 0) > 0
  const [expanded, setExpanded] = useState(() =>
    location.pathname.startsWith(`/commands/${cmd.path}`)
  )

  if (cmd.hidden || cmd.webHidden) return null
  if (simpleMode && !cmd.simpleMode) return null

  const filteredChildren = (cmd.children ?? []).filter(
    (c) => !c.hidden && !c.webHidden && (!simpleMode || c.simpleMode)
  )

  const Icon = getIcon(cmd.icon ?? '')
  const indent = 8 + depth * 28

  if (hasChildren && filteredChildren.length > 0) {
    return (
      <div className="select-none">
        <button
          type="button"
          onClick={() => setExpanded((e) => !e)}
          className={cn(
            'relative flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm transition-colors',
            'hover:bg-slate-700/50 dark:hover:bg-slate-600/30',
            'text-slate-200 dark:text-slate-300'
          )}
          style={{ paddingLeft: `${indent}px` }}
        >
          <TreeGuides depth={depth} />
          {expanded ? (
            <ChevronDown className="h-4 w-4 shrink-0" />
          ) : (
            <ChevronRight className="h-4 w-4 shrink-0" />
          )}
          <Icon className="h-4 w-4 shrink-0" />
          <span className="truncate">{cmd.displayName ?? cmd.name}</span>
        </button>
        {expanded && (
          <div className="relative mt-0.5">
            {filteredChildren.map((child) => (
              <SidebarItem
                key={child.path}
                cmd={child}
                basePath={basePath}
                simpleMode={simpleMode}
                depth={depth + 1}
              />
            ))}
          </div>
        )}
      </div>
    )
  }

  return (
    <NavLink
      to={`/commands/${cmd.path}`}
      className={({ isActive: active }) =>
        cn(
          'relative flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm transition-colors',
          active
            ? 'bg-blue-600/30 text-white dark:bg-blue-500/30'
            : 'text-slate-200 hover:bg-slate-700/50 dark:text-slate-300 dark:hover:bg-slate-600/30',
          'hover:text-white'
        )
      }
      style={{ paddingLeft: `${indent}px` }}
      title={cmd.description}
    >
      <TreeGuides depth={depth} />
      <Icon className="h-4 w-4 shrink-0" />
      <span className="truncate">{cmd.displayName ?? cmd.name}</span>
    </NavLink>
  )
}

function renderCommandTree(cmd: CommandInfo, simpleMode: boolean) {
  const children = (cmd.children ?? []).filter(
    (c) => !c.hidden && !c.webHidden && (!simpleMode || c.simpleMode)
  )
  return children.map((child) => (
    <SidebarItem
      key={child.path}
      cmd={child}
      basePath=""
      simpleMode={simpleMode}
      depth={0}
    />
  ))
}

function CollapsedIconItem({ to, icon: Icon, label }: { to: string; icon: React.ComponentType<{ className?: string }>; label: string }) {
  return (
    <NavLink
      to={to}
      className="flex items-center justify-center p-2 rounded-lg text-slate-200 hover:bg-slate-700/50 dark:text-slate-300 dark:hover:bg-slate-600/30 transition-colors"
      title={label}
    >
      <Icon className="h-5 w-5" />
    </NavLink>
  )
}

export default function Sidebar({ collapsed, onToggleCollapsed: _onToggleCollapsed }: SidebarProps) {
  const { data: commandTree, isLoading } = useCommandTree()
  const { simpleMode } = usePreferences()

  return (
    <aside
      className={cn(
        'flex flex-col border-r border-slate-700 bg-slate-900 dark:bg-slate-950 transition-all duration-200 shrink-0',
        collapsed ? 'w-16' : 'w-64'
      )}
    >
      <div className={cn('flex h-14 items-center border-b border-slate-700', collapsed ? 'justify-center px-0' : 'px-4')}>
        {!collapsed ? (
          <div className="flex items-center gap-2">
            <img src="/aerolab.png" alt="AeroLab" className="h-8 w-8" />
            <span className="font-semibold text-white text-lg truncate">
              AeroLab
            </span>
          </div>
        ) : (
          <img src="/aerolab.png" alt="AeroLab" className="h-7 w-7" title="AeroLab" />
        )}
      </div>
      <nav className="flex-1 overflow-y-auto p-2 space-y-0.5">
        {collapsed ? (
          <>
            <CollapsedIconItem to="/inventory" icon={Database} label="Inventory" />
            <CollapsedIconItem to="/jobs" icon={ListTodo} label="Jobs" />
            {commandTree &&
              (commandTree.children ?? [])
                .filter((c) => !c.hidden && !c.webHidden && (!simpleMode || c.simpleMode))
                .map((child) => {
                  const Icon = getIcon(child.icon ?? '')
                  return (
                    <NavLink
                      key={child.path}
                      to={`/commands/${child.path}`}
                      className={({ isActive }) =>
                        cn(
                          'flex items-center justify-center p-2 rounded-lg transition-colors',
                          isActive
                            ? 'bg-blue-600/30 text-white'
                            : 'text-slate-200 hover:bg-slate-700/50 dark:text-slate-300 dark:hover:bg-slate-600/30'
                        )
                      }
                      title={child.displayName ?? child.name}
                    >
                      <Icon className="h-5 w-5" />
                    </NavLink>
                  )
                })}
          </>
        ) : (
          <>
            <NavLink
              to="/inventory"
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm transition-colors',
                  isActive
                    ? 'bg-blue-600/30 text-white'
                    : 'text-slate-200 hover:bg-slate-700/50 dark:text-slate-300 dark:hover:bg-slate-600/30'
                )
              }
              title="Inventory"
            >
              <Database className="h-4 w-4 shrink-0" />
              <span className="font-medium">Inventory</span>
            </NavLink>
            <NavLink
              to="/jobs"
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm transition-colors',
                  isActive
                    ? 'bg-blue-600/30 text-white'
                    : 'text-slate-200 hover:bg-slate-700/50 dark:text-slate-300 dark:hover:bg-slate-600/30'
                )
              }
              title="Jobs"
            >
              <ListTodo className="h-4 w-4 shrink-0" />
              <span className="font-medium">Jobs</span>
            </NavLink>
            {commandTree && (
              <div className="mt-2">
                {renderCommandTree(commandTree, simpleMode)}
              </div>
            )}
            {isLoading && (
              <div className="animate-pulse py-2 text-slate-400 text-sm">
                Loading commands...
              </div>
            )}
          </>
        )}
      </nav>
    </aside>
  )
}
