import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react'
import { getConfig } from '@/utils/config'

const STORAGE_KEYS = {
  simpleMode: 'aerolab_simple_mode',
  shortSwitches: 'aerolab_short_switches',
  defaultSwitches: 'aerolab_default_switches',
  showAllUsers: 'aerolab_show_all_users',
  inventoryTab: 'aerolab_inventory_tab',
  darkMode: 'aerolab_dark_mode',
  historyTruncate: 'aerolab_history_truncate',
  historyTruncateAt: 'aerolab_history_truncate_at',
  sidebarCollapsed: 'aerolab_sidebar_collapsed',
} as const

function getStoredBool(key: string, defaultVal: boolean): boolean {
  if (typeof window === 'undefined') return defaultVal
  const v = localStorage.getItem(key)
  if (v === null) return defaultVal
  return v === 'true'
}

function getStoredString(key: string, defaultVal: string): string {
  if (typeof window === 'undefined') return defaultVal
  const v = localStorage.getItem(key)
  return v ?? defaultVal
}

function setStoredBool(key: string, val: boolean) {
  localStorage.setItem(key, String(val))
}

function setStoredString(key: string, val: string) {
  localStorage.setItem(key, val)
}

interface PreferencesValue {
  simpleMode: boolean
  setSimpleMode: (v: boolean) => void
  forceSimpleMode: boolean
  shortSwitches: boolean
  setShortSwitches: (v: boolean) => void
  defaultSwitches: boolean
  setDefaultSwitches: (v: boolean) => void
  showAllUsers: boolean
  setShowAllUsers: (v: boolean) => void
  inventoryTab: string
  setInventoryTab: (v: string) => void
  darkMode: boolean
  setDarkMode: (v: boolean) => void
  historyTruncate: boolean
  setHistoryTruncate: (v: boolean) => void
  historyTruncateAt: string | null
  setHistoryTruncateAt: (v: string | null) => void
  sidebarCollapsed: boolean
  setSidebarCollapsed: (v: boolean) => void
}

const PreferencesContext = createContext<PreferencesValue | null>(null)

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const forceSimpleMode = getConfig().forceSimpleMode

  const [simpleMode, setSimpleModeState] = useState(() =>
    forceSimpleMode ? true : getStoredBool(STORAGE_KEYS.simpleMode, true)
  )
  const [shortSwitches, setShortSwitchesState] = useState(() =>
    getStoredBool(STORAGE_KEYS.shortSwitches, false)
  )
  const [defaultSwitches, setDefaultSwitchesState] = useState(() =>
    getStoredBool(STORAGE_KEYS.defaultSwitches, false)
  )
  const [showAllUsers, setShowAllUsersState] = useState(() =>
    getStoredBool(STORAGE_KEYS.showAllUsers, false)
  )
  const [inventoryTab, setInventoryTabState] = useState(() =>
    getStoredString(STORAGE_KEYS.inventoryTab, 'clusters')
  )
  const [darkMode, setDarkModeState] = useState(() =>
    getStoredBool(STORAGE_KEYS.darkMode, false)
  )
  const [historyTruncate, setHistoryTruncateState] = useState(() =>
    getStoredBool(STORAGE_KEYS.historyTruncate, false)
  )
  const [historyTruncateAt, setHistoryTruncateAtState] = useState<string | null>(() =>
    getStoredString(STORAGE_KEYS.historyTruncateAt, '') || null
  )
  const [sidebarCollapsed, setSidebarCollapsedState] = useState(() => {
    const stored = getStoredString(STORAGE_KEYS.sidebarCollapsed, '')
    if (stored === 'true' || stored === 'false') return stored === 'true'
    if (typeof window !== 'undefined' && window.innerWidth < 768) return true
    return false
  })

  const setSimpleMode = useCallback((v: boolean) => {
    if (forceSimpleMode) return // simple mode is enforced, cannot be changed
    setSimpleModeState(v)
    setStoredBool(STORAGE_KEYS.simpleMode, v)
  }, [forceSimpleMode])

  const setShortSwitches = useCallback((v: boolean) => {
    setShortSwitchesState(v)
    setStoredBool(STORAGE_KEYS.shortSwitches, v)
  }, [])

  const setDefaultSwitches = useCallback((v: boolean) => {
    setDefaultSwitchesState(v)
    setStoredBool(STORAGE_KEYS.defaultSwitches, v)
  }, [])

  const setShowAllUsers = useCallback((v: boolean) => {
    setShowAllUsersState(v)
    setStoredBool(STORAGE_KEYS.showAllUsers, v)
  }, [])

  const setInventoryTab = useCallback((v: string) => {
    setInventoryTabState(v)
    setStoredString(STORAGE_KEYS.inventoryTab, v)
  }, [])

  const setDarkMode = useCallback((v: boolean) => {
    setDarkModeState(v)
    setStoredBool(STORAGE_KEYS.darkMode, v)
    if (typeof document !== 'undefined') {
      document.documentElement.classList.toggle('dark', v)
    }
  }, [])

  const setHistoryTruncate = useCallback((v: boolean) => {
    setHistoryTruncateState(v)
    setStoredBool(STORAGE_KEYS.historyTruncate, v)
  }, [])

  const setHistoryTruncateAt = useCallback((v: string | null) => {
    setHistoryTruncateAtState(v)
    setStoredString(STORAGE_KEYS.historyTruncateAt, v ?? '')
  }, [])

  const setSidebarCollapsed = useCallback((v: boolean) => {
    setSidebarCollapsedState(v)
    setStoredBool(STORAGE_KEYS.sidebarCollapsed, v)
  }, [])

  useEffect(() => {
    document.documentElement.classList.toggle('dark', darkMode)
  }, [darkMode])

  return (
    <PreferencesContext.Provider
      value={{
        simpleMode,
        setSimpleMode,
        forceSimpleMode,
        shortSwitches,
        setShortSwitches,
        defaultSwitches,
        setDefaultSwitches,
        showAllUsers,
        setShowAllUsers,
        inventoryTab,
        setInventoryTab,
        darkMode,
        setDarkMode,
        historyTruncate,
        setHistoryTruncate,
        historyTruncateAt,
        setHistoryTruncateAt,
        sidebarCollapsed,
        setSidebarCollapsed,
      }}
    >
      {children}
    </PreferencesContext.Provider>
  )
}

export function usePreferences(): PreferencesValue {
  const ctx = useContext(PreferencesContext)
  if (!ctx) {
    throw new Error('usePreferences must be used within PreferencesProvider')
  }
  return ctx
}
