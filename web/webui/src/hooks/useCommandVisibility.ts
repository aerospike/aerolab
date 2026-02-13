import { useCallback, useMemo } from 'react'
import { useCommandTree } from './useCommands'
import { usePreferences } from './usePreferences'
import type { CommandInfo } from '@/api/types'

/**
 * Walk the command tree to find a node by slash-separated path and return
 * its simpleMode value.  Returns true (visible) when the tree hasn't loaded
 * yet or the node isn't found (fail-open).
 */
function isVisibleInTree(tree: CommandInfo | undefined, path: string): boolean {
  if (!tree?.children) return true
  const parts = path.split('/').filter(Boolean)
  let children: CommandInfo[] | undefined = tree.children
  let node: CommandInfo | undefined
  for (const part of parts) {
    node = children?.find((c) => c.name === part)
    if (!node) return true // node not in tree, fail-open
    children = node.children
  }
  return node?.simpleMode ?? true
}

/**
 * Returns a stable callback that checks whether a given command path
 * (slash-separated, e.g. "cluster/create") is visible under the current
 * simple-mode settings.
 *
 * When simple mode is off, every command is visible.
 * When simple mode is on, it walks the command tree (which already has
 * config-file overrides applied by the backend) and returns the node's
 * `simpleMode` field.
 */
export function useCommandVisibility(): (path: string) => boolean {
  const { data: tree } = useCommandTree()
  const { simpleMode } = usePreferences()

  return useCallback(
    (path: string) => {
      if (!simpleMode) return true
      return isVisibleInTree(tree, path)
    },
    [tree, simpleMode],
  )
}

/**
 * Convenience wrapper: returns true if the single command path is allowed.
 */
export function useIsCommandAllowed(path: string): boolean {
  const isAllowed = useCommandVisibility()
  return useMemo(() => isAllowed(path), [isAllowed, path])
}
