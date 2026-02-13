import { useState, useCallback, KeyboardEvent } from 'react'
import type { ParameterInfo } from '@/api/types'
import { cn } from '@/utils/cn'
import InfoTooltip from '@/components/common/InfoTooltip'

interface TagInputProps {
  param: ParameterInfo
  values: string[]
  onChange: (values: string[]) => void
}

export default function TagInput({ param, values, onChange }: TagInputProps) {
  const [input, setInput] = useState('')

  const addTag = useCallback(
    (tag: string) => {
      const t = tag.trim()
      if (!t || values.includes(t)) return
      onChange([...values, t])
      setInput('')
    },
    [values, onChange]
  )

  const removeTag = useCallback(
    (index: number) => {
      onChange(values.filter((_, i) => i !== index))
    },
    [values, onChange]
  )

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      const v = e.key === ',' ? input.replace(/,/g, '') : input
      addTag(v)
    } else if (e.key === 'Backspace' && !input && values.length > 0) {
      removeTag(values.length - 1)
    }
  }

  return (
    <div className="flex items-start gap-4">
      <label
        htmlFor={param.fieldName}
        className="w-48 shrink-0 pt-2 text-sm font-medium text-slate-700 dark:text-slate-300"
      >
        {param.required && '* '}
        {param.displayName ?? param.name}
        {param.description && <InfoTooltip text={param.description} />}
      </label>
      <div className="min-w-0 flex-1">
        <div
          className={cn(
            'flex min-h-[42px] flex-wrap gap-2 rounded-md border bg-white px-3 py-2',
            'focus-within:outline-none focus-within:ring-2 focus-within:ring-blue-500 focus-within:ring-offset-2 dark:focus-within:ring-offset-gray-900',
            'border-slate-300 dark:border-slate-600 dark:bg-slate-800'
          )}
        >
          {values.map((v, i) => (
            <span
              key={`${v}-${i}`}
              className="inline-flex items-center gap-1 rounded-md bg-blue-100 px-2 py-0.5 text-sm text-blue-800 dark:bg-blue-900/50 dark:text-blue-200"
            >
              {v}
              <button
                type="button"
                onClick={() => removeTag(i)}
                className="rounded p-0.5 hover:bg-blue-200 dark:hover:bg-blue-800"
                aria-label={`Remove ${v}`}
              >
                ×
              </button>
            </span>
          ))}
          <input
            id={param.fieldName}
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onBlur={() => input.trim() && addTag(input)}
            placeholder="Type and press Enter or comma to add"
            className="min-w-[120px] flex-1 border-0 bg-transparent p-0 text-slate-900 outline-none placeholder:text-slate-400 dark:text-slate-100 dark:placeholder:text-slate-500"
          />
        </div>
        <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">
          Press Enter or comma to add a tag
        </p>
      </div>
    </div>
  )
}
