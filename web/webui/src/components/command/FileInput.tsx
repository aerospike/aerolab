import { useState } from 'react'
import type { ParameterInfo } from '@/api/types'
import { cn } from '@/utils/cn'
import { FileBrowserDialog } from '@/components/common/FileBrowserDialog'
import InfoTooltip from '@/components/common/InfoTooltip'

interface FileInputProps {
  param: ParameterInfo
  value: string | File
  onChange: (value: string | File) => void
  allowServerBrowse?: boolean
}

export default function FileInput({ param, value, onChange, allowServerBrowse }: FileInputProps) {
  const webType = param.webType
  const [isBrowserOpen, setIsBrowserOpen] = useState(false)

  // Explicit webtype overrides take priority
  if (webType === 'download') {
    const strVal = typeof value === 'string' ? value : ''
    return (
      <div className="flex items-start gap-4">
        <label className="w-48 shrink-0 pt-2 text-sm font-medium text-slate-700 dark:text-slate-300">
          {param.required && '* '}
          {param.displayName ?? param.name}
          {param.description && <InfoTooltip text={param.description} />}
        </label>
        <div className="min-w-0 flex-1">
          <input
            type="text"
            value={strVal}
            onChange={(e) => onChange(e.target.value)}
            placeholder={param.default || 'Path for download destination'}
            className={cn(
              'block w-full rounded-md border bg-slate-50 px-3 py-2 text-slate-600',
              'border-slate-300 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-400',
              'read-only:cursor-default'
            )}
            readOnly
          />
          <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">
            Read-only: download path (file will stream to browser)
          </p>
        </div>
      </div>
    )
  }

  // Explicit upload webtype OR isFile without server browse → native file upload
  if (webType === 'upload' || (param.isFile && !allowServerBrowse && webType !== 'text')) {
    const fileVal = value instanceof File ? value.name : ''
    return (
      <div className="flex items-start gap-4">
        <label className="w-48 shrink-0 pt-2 text-sm font-medium text-slate-700 dark:text-slate-300">
          {param.required && '* '}
          {param.displayName ?? param.name}
          {param.description && <InfoTooltip text={param.description} />}
        </label>
        <div className="min-w-0 flex-1">
          <input
            type="file"
            onChange={(e) => {
              const file = e.target.files?.[0]
              onChange(file ?? '')
            }}
            className={cn(
              'block w-full rounded-md border border-slate-300 px-3 py-2 text-sm',
              'file:mr-4 file:rounded-md file:border-0 file:bg-blue-50 file:px-4 file:py-2 file:text-blue-700',
              'dark:border-slate-600 dark:file:bg-blue-900/30 dark:file:text-blue-300'
            )}
          />
          {fileVal && (
            <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">Selected: {fileVal}</p>
          )}
        </div>
      </div>
    )
  }

  // Server-side browse (text input + Browse button)
  // This applies when: explicit webtype:"file", OR isFile with server browse allowed
  const strVal = typeof value === 'string' ? value : ''

  return (
    <div className="flex items-start gap-4">
      <label className="w-48 shrink-0 pt-2 text-sm font-medium text-slate-700 dark:text-slate-300">
        {param.required && '* '}
        {param.displayName ?? param.name}
        {param.description && <InfoTooltip text={param.description} />}
      </label>
      <div className="min-w-0 flex-1">
        <div className="flex gap-2">
          <input
            type="text"
            value={strVal}
            onChange={(e) => onChange(e.target.value)}
            placeholder={param.default || 'File path'}
            className={cn(
              'block flex-1 rounded-md border bg-white px-3 py-2 text-slate-900 shadow-sm',
              'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-gray-900',
              'border-slate-300 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100'
            )}
          />
          <button
            type="button"
            className="rounded-md border border-slate-300 bg-slate-50 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 dark:border-slate-600 dark:bg-slate-700 dark:text-slate-300 dark:hover:bg-slate-600"
            onClick={() => setIsBrowserOpen(true)}
          >
            Browse
          </button>
        </div>
        <FileBrowserDialog
          isOpen={isBrowserOpen}
          onClose={() => setIsBrowserOpen(false)}
          onSelect={(path) => {
            onChange(path)
            setIsBrowserOpen(false)
          }}
          initialPath={strVal || undefined}
          title={`Browse: ${param.displayName ?? param.name}`}
        />
      </div>
    </div>
  )
}
