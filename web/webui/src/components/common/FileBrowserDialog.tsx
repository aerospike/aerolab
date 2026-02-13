import { useEffect, useState } from 'react'
import { Folder, FileIcon, ChevronUp, Loader2 } from 'lucide-react'
import { Modal } from './Modal'
import { useFileBrowser } from '@/hooks/useFileBrowser'
import { cn } from '@/utils/cn'

interface FileBrowserDialogProps {
  isOpen: boolean
  onClose: () => void
  onSelect: (path: string) => void
  initialPath?: string
  title?: string
}

export function FileBrowserDialog({
  isOpen,
  onClose,
  onSelect,
  initialPath = '',
  title = 'Browse Files',
}: FileBrowserDialogProps) {
  const [pathInput, setPathInput] = useState('')
  const {
    currentPath,
    dirs,
    files,
    isLoading,
    error,
    navigate,
    goUp,
    getHomedir,
  } = useFileBrowser()

  useEffect(() => {
    if (isOpen) {
      setPathInput(initialPath || '')
      if (initialPath) {
        navigate(initialPath).catch(() => getHomedir(initialPath))
      } else {
        getHomedir()
      }
    }
  }, [isOpen, initialPath, getHomedir, navigate])

  useEffect(() => {
    setPathInput(currentPath)
  }, [currentPath])

  const handleDirClick = (name: string) => {
    const sep = currentPath.endsWith('/') ? '' : '/'
    const next = currentPath ? `${currentPath}${sep}${name}` : `/${name}`
    navigate(next)
  }

  const handleFileClick = (name: string) => {
    const sep = currentPath.endsWith('/') ? '' : '/'
    const fullPath = currentPath ? `${currentPath}${sep}${name}` : `/${name}`
    onSelect(fullPath)
    onClose()
  }

  const handlePathSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const p = pathInput.trim() || '/'
    navigate(p)
  }

  const handleSelectByPath = () => {
    const p = pathInput.trim()
    if (p) {
      onSelect(p)
      onClose()
    }
  }

  const breadcrumbParts = currentPath
    .replace(/\/+$/, '')
    .split('/')
    .filter(Boolean)

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={title}
      size="xl"
      footer={
        <div className="flex w-full items-center justify-between gap-2">
          <form onSubmit={handlePathSubmit} className="flex flex-1 gap-2 min-w-0">
            <input
              type="text"
              value={pathInput}
              onChange={(e) => setPathInput(e.target.value)}
              placeholder="/path/to/file"
              className={cn(
                'flex-1 min-w-0 rounded-md border px-3 py-2 text-sm',
                'bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100',
                'border-slate-300 dark:border-slate-600',
                'placeholder:text-slate-400 dark:placeholder:text-slate-500'
              )}
            />
            <button
              type="submit"
              className="shrink-0 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              Go
            </button>
            <button
              type="button"
              onClick={handleSelectByPath}
              className="shrink-0 rounded-md border border-slate-300 bg-slate-50 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-300 dark:hover:bg-slate-600"
            >
              Select Path
            </button>
          </form>
          <button
            type="button"
            onClick={onClose}
            className="shrink-0 rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-300 dark:hover:bg-slate-600"
          >
            Cancel
          </button>
        </div>
      }
    >
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={goUp}
            className="flex items-center gap-1 rounded-md border border-slate-300 bg-slate-50 px-2 py-1.5 text-sm text-slate-700 hover:bg-slate-100 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-300 dark:hover:bg-slate-600"
          >
            <ChevronUp className="h-4 w-4" />
            Go Up
          </button>
          <div className="flex flex-1 min-w-0 flex-wrap items-center gap-1 rounded-md bg-slate-100 px-2 py-1.5 dark:bg-slate-700/50">
            <button
              type="button"
              onClick={() => navigate('/')}
              className="text-sm text-blue-600 hover:underline dark:text-blue-400"
            >
              /
            </button>
            {breadcrumbParts.map((part, i) => {
              const pathSoFar = '/' + breadcrumbParts.slice(0, i + 1).join('/')
              return (
                <span key={pathSoFar} className="flex items-center gap-1">
                  <span className="text-slate-400 dark:text-slate-500">/</span>
                  <button
                    type="button"
                    onClick={() => navigate(pathSoFar)}
                    className="text-sm text-blue-600 hover:underline dark:text-blue-400"
                  >
                    {part}
                  </button>
                </span>
              )
            })}
          </div>
        </div>

        {error && (
          <div className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-900/30 dark:text-red-400">
            {error}
          </div>
        )}

        <div
          className="min-h-[280px] max-h-[360px] overflow-y-auto rounded-md border border-slate-200 bg-slate-50 dark:border-slate-600 dark:bg-slate-800/50"
          style={{ minHeight: 280 }}
        >
          {isLoading ? (
            <div className="flex h-[280px] items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-slate-400" />
            </div>
          ) : (
            <div className="flex flex-col p-2">
              {dirs.map((d) => (
                <button
                  key={d}
                  type="button"
                  onClick={() => handleDirClick(d)}
                  className={cn(
                    'flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm',
                    'hover:bg-slate-200 dark:hover:bg-slate-700',
                    'text-slate-800 dark:text-slate-200'
                  )}
                >
                  <Folder className="h-5 w-5 shrink-0 text-amber-500 dark:text-amber-400" />
                  <span className="truncate">{d}</span>
                </button>
              ))}
              {files.map((f) => (
                <button
                  key={f}
                  type="button"
                  onClick={() => handleFileClick(f)}
                  className={cn(
                    'flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm',
                    'hover:bg-slate-200 dark:hover:bg-slate-700',
                    'text-slate-800 dark:text-slate-200'
                  )}
                >
                  <FileIcon className="h-5 w-5 shrink-0 text-slate-400 dark:text-slate-500" />
                  <span className="truncate">{f}</span>
                </button>
              ))}
              {!isLoading && dirs.length === 0 && files.length === 0 && !error && (
                <div className="py-8 text-center text-sm text-slate-500 dark:text-slate-400">
                  This directory is empty
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </Modal>
  )
}
