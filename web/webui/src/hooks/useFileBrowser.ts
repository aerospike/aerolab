import { useState, useCallback } from 'react'
import { fetchFsHomedir, fetchFsLs } from '@/api/client'

export function useFileBrowser() {
  const [currentPath, setCurrentPath] = useState<string>('')
  const [dirs, setDirs] = useState<string[]>([])
  const [files, setFiles] = useState<string[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const navigate = useCallback(async (path: string) => {
    setError(null)
    setIsLoading(true)
    try {
      const res = await fetchFsLs(path)
      setCurrentPath(res.path)
      setDirs(res.dirs ?? [])
      setFiles(res.files ?? [])
      return res
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to list directory'
      setError(msg)
      setDirs([])
      setFiles([])
    } finally {
      setIsLoading(false)
    }
  }, [])

  const goUp = useCallback(() => {
    const parts = currentPath.replace(/\/+$/, '').split('/').filter(Boolean)
    if (parts.length <= 1) {
      navigate('/')
      return
    }
    parts.pop()
    const parent = '/' + parts.join('/')
    navigate(parent)
  }, [currentPath, navigate])

  const getHomedir = useCallback(async (initialPath?: string) => {
    setError(null)
    setIsLoading(true)
    try {
      const res = await fetchFsHomedir(initialPath)
      setCurrentPath(res.path)
      const ls = await fetchFsLs(res.path)
      setDirs(ls.dirs ?? [])
      setFiles(ls.files ?? [])
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to get home directory'
      setError(msg)
      setDirs([])
      setFiles([])
    } finally {
      setIsLoading(false)
    }
  }, [])

  return {
    currentPath,
    dirs,
    files,
    isLoading,
    error,
    navigate,
    goUp,
    getHomedir,
  }
}
