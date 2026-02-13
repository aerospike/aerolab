import { createContext, useContext } from 'react'

export const ServerBrowseContext = createContext<boolean | undefined>(undefined)

export function useServerBrowse(): boolean {
  const value = useContext(ServerBrowseContext)
  // Default to true (show browse) when context is not provided
  return value ?? true
}
