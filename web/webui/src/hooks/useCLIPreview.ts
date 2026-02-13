import { useState, useEffect, useCallback, useRef } from 'react'
import { generateCLI } from '@/api/client'

const DEBOUNCE_MS = 500

export function useCLIPreview(
  commandPath: string,
  formValues: Record<string, unknown>,
  enabled: boolean,
  shortSwitches: boolean,
  defaultSwitches: boolean
) {
  const [cli, setCli] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const mountedRef = useRef(true)

  const fetchCLI = useCallback(async () => {
    if (!enabled || !commandPath.trim()) {
      setCli('')
      return
    }

    setIsLoading(true)
    try {
      const { cli: result } = await generateCLI({
        commandPath,
        parameters: formValues,
        preferShort: shortSwitches,
        includeDefaults: defaultSwitches,
      })
      if (mountedRef.current) setCli(result)
    } catch {
      if (mountedRef.current) setCli('')
    } finally {
      if (mountedRef.current) setIsLoading(false)
    }
  }, [commandPath, formValues, shortSwitches, defaultSwitches, enabled])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  useEffect(() => {
    if (!enabled || !commandPath.trim()) {
      setCli('')
      setIsLoading(false)
      return
    }

    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current)
    }

    timeoutRef.current = setTimeout(() => {
      fetchCLI()
      timeoutRef.current = null
    }, DEBOUNCE_MS)

    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current)
      }
    }
  }, [commandPath, formValues, shortSwitches, defaultSwitches, enabled, fetchCLI])

  const refresh = useCallback(() => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current)
      timeoutRef.current = null
    }
    fetchCLI()
  }, [fetchCLI])

  return { cli, isLoading, refresh }
}
