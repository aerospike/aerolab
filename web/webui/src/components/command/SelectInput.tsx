import { useState, useMemo, useRef, useEffect } from 'react'
import { Check } from 'lucide-react'
import { Eye, EyeOff } from 'lucide-react'
import type { ParameterInfo } from '@/api/types'
import { cn } from '@/utils/cn'
import InfoTooltip from '@/components/common/InfoTooltip'

// Searchable combobox for single-select with many choices
interface ComboboxSelectProps {
  param: ParameterInfo
  value: string
  onChange: (value: string) => void
  choices: string[]
  labels: string[]
  hasLabels: boolean
  getLabel: (val: string, idx: number) => string
  filteredIndices: number[]
  filter: string
  setFilter: (filter: string) => void
  placeholder: string
}

function ComboboxSelect({
  param,
  value,
  onChange,
  choices,
  labels,
  hasLabels,
  getLabel,
  filteredIndices,
  filter,
  setFilter,
  placeholder,
}: ComboboxSelectProps) {
  const [isOpen, setIsOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  const selectedIdx = value ? choices.indexOf(value) : -1
  const selectedLabel = selectedIdx >= 0 ? getLabel(choices[selectedIdx], selectedIdx) : value || ''
  const inputDisplayValue = isOpen ? filter : selectedLabel

  // Click outside to close
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false)
        setFilter('')
      }
    }
    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside)
      return () => document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [isOpen, setFilter])

  const handleSelect = (opt: string) => {
    onChange(opt)
    setIsOpen(false)
    setFilter('')
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!isOpen) {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        setIsOpen(true)
      }
      return
    }
    if (e.key === 'Escape') {
      setIsOpen(false)
      setFilter('')
      return
    }
    if (e.key === 'Enter' && filteredIndices.length > 0) {
      e.preventDefault()
      const idx = filteredIndices[0]
      handleSelect(choices[idx])
    }
  }

  return (
    <div ref={containerRef} className="relative">
      <input
        id={param.fieldName}
        type="text"
        role="combobox"
        aria-expanded={isOpen}
        aria-haspopup="listbox"
        aria-autocomplete="list"
        autoComplete="off"
        placeholder={placeholder}
        value={inputDisplayValue}
        onChange={(e) => {
          setFilter(e.target.value)
          if (!isOpen) setIsOpen(true)
        }}
        onFocus={() => setIsOpen(true)}
        onKeyDown={handleKeyDown}
        className={cn(
          'block w-full rounded-md border bg-white px-3 py-2 text-slate-900 shadow-sm',
          'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-gray-900',
          'border-slate-300 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100'
        )}
      />
      {isOpen && (
        <div
          role="listbox"
          className="absolute z-50 mt-1 max-h-60 w-full overflow-y-auto rounded-md border border-slate-300 bg-white shadow-lg dark:border-slate-600 dark:bg-slate-800"
        >
          {filteredIndices.length === 0 ? (
            <div className="px-3 py-2 text-sm text-slate-500 dark:text-slate-400">No matches</div>
          ) : (
            filteredIndices.map((idx) => {
              const opt = choices[idx]
              const isSelected = opt === value
              return (
                <button
                  key={opt}
                  type="button"
                  role="option"
                  aria-selected={isSelected}
                  onClick={() => handleSelect(opt)}
                  className={cn(
                    'flex w-full flex-col items-start gap-0.5 px-3 py-2 text-left text-sm transition-colors',
                    'hover:bg-slate-100 dark:hover:bg-slate-700',
                    isSelected && 'bg-blue-50 dark:bg-blue-900/30'
                  )}
                >
                  <span className="flex w-full items-center justify-between gap-2">
                    <span className="font-medium text-slate-900 dark:text-slate-100">{opt}</span>
                    {isSelected && <Check size={14} className="shrink-0 text-blue-600 dark:text-blue-400" />}
                  </span>
                  {hasLabels && (() => {
                    const lbl = labels[idx]
                    const detail = lbl.startsWith(opt) ? lbl.slice(opt.length).trim() : lbl
                    return detail ? (
                      <span className="text-xs text-slate-500 dark:text-slate-400">{detail}</span>
                    ) : null
                  })()}
                </button>
              )
            })
          )}
        </div>
      )}
    </div>
  )
}

interface SelectInputProps {
  param: ParameterInfo
  value: string | string[]
  onChange: (value: string | string[]) => void
  visible: boolean
  onVisibilityChange: (visible: boolean) => void
}

export default function SelectInput({
  param,
  value,
  onChange,
  visible,
  onVisibilityChange,
}: SelectInputProps) {
  const [filter, setFilter] = useState('')
  const choices = param.choices ?? []
  const labels = param.choiceLabels ?? []
  const hasLabels = labels.length === choices.length
  const isMulti = param.isSlice && choices.length > 0

  // Get the display label for a choice value
  const getLabel = (val: string, idx: number): string => {
    if (hasLabels && idx >= 0 && idx < labels.length) return labels[idx]
    return val
  }

  // Build filtered indices (filter against both value and label)
  const filteredIndices = useMemo(() => {
    const indices = choices.map((_, i) => i)
    if (!filter.trim()) return indices
    const f = filter.toLowerCase()
    return indices.filter((i) => {
      const label = hasLabels ? labels[i] : choices[i]
      const val = choices[i]
      return val.toLowerCase().includes(f) || label.toLowerCase().includes(f)
    })
  }, [choices, labels, hasLabels, filter])

  if (param.optional && !visible) {
    return (
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm text-slate-500 dark:text-slate-400">
          {param.displayName ?? param.name}
          {param.description && <InfoTooltip text={param.description} />}
        </span>
        <button
          type="button"
          onClick={() => onVisibilityChange(true)}
          className="rounded p-1 text-slate-500 hover:bg-slate-100 hover:text-blue-600 dark:text-slate-400 dark:hover:bg-slate-700 dark:hover:text-blue-400"
          title="Show optional field"
        >
          <Eye size={16} />
        </button>
      </div>
    )
  }

  const displayValue = Array.isArray(value) ? value.join(', ') : value

  if (isMulti) {
    const selected = Array.isArray(value) ? value : value ? [value] : []
    const toggle = (opt: string) => {
      const next = selected.includes(opt)
        ? selected.filter((s) => s !== opt)
        : [...selected, opt]
      onChange(next)
    }
    return (
      <div className="flex items-start gap-4">
        <label className="w-48 shrink-0 pt-2 text-sm font-medium text-slate-700 dark:text-slate-300 flex items-center gap-1">
          {param.optional && (
            <button
              type="button"
              onClick={() => onVisibilityChange(false)}
              className="rounded p-0.5 text-slate-400 hover:text-red-500 dark:text-slate-500 dark:hover:text-red-400"
              title="Hide optional field"
            >
              <EyeOff size={14} />
            </button>
          )}
          {param.required && '* '}
          {param.displayName ?? param.name}
          {param.description && <InfoTooltip text={param.description} />}
        </label>
        <div className="min-w-0 flex-1">
          {choices.length > 8 && (
            <input
              type="text"
              placeholder="Filter..."
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              className="mb-2 block w-full rounded-md border border-slate-300 px-2 py-1 text-sm dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100"
            />
          )}
          <div className="max-h-48 overflow-y-auto rounded-md border border-slate-300 dark:border-slate-600 dark:bg-slate-800">
            {filteredIndices.map((idx) => (
              <label
                key={choices[idx]}
                className="flex cursor-pointer items-center gap-2 border-b border-slate-200 px-3 py-2 last:border-0 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-700/50"
              >
                <input
                  type="checkbox"
                  checked={selected.includes(choices[idx])}
                  onChange={() => toggle(choices[idx])}
                  className="h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500 dark:border-slate-600"
                />
                <span className="text-sm text-slate-800 dark:text-slate-200">{getLabel(choices[idx], idx)}</span>
              </label>
            ))}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="flex items-start gap-4">
      <label
        htmlFor={param.fieldName}
        className="w-48 shrink-0 pt-2 text-sm font-medium text-slate-700 dark:text-slate-300 flex items-center gap-1"
      >
        {param.optional && (
          <button
            type="button"
            onClick={() => onVisibilityChange(false)}
            className="rounded p-0.5 text-slate-400 hover:text-red-500 dark:text-slate-500 dark:hover:text-red-400"
            title="Hide optional field"
          >
            <EyeOff size={14} />
          </button>
        )}
        {param.required && '* '}
        {param.displayName ?? param.name}
        {param.description && <InfoTooltip text={param.description} />}
      </label>
      <div className="min-w-0 flex-1">
        {choices.length > 12 ? (
          <ComboboxSelect
            param={param}
            value={displayValue}
            onChange={onChange}
            choices={choices}
            labels={labels}
            hasLabels={hasLabels}
            getLabel={getLabel}
            filteredIndices={filteredIndices}
            filter={filter}
            setFilter={setFilter}
            placeholder={param.default || 'Select...'}
          />
        ) : (
          <select
            id={param.fieldName}
            value={displayValue}
            onChange={(e) => onChange(e.target.value)}
            className={cn(
              'block w-full rounded-md border bg-white px-3 py-2 text-slate-900 shadow-sm',
              'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-gray-900',
              'border-slate-300 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100'
            )}
          >
            <option value="">{param.default || 'Select...'}</option>
            {choices.map((opt, idx) => (
              <option key={opt} value={opt}>
                {getLabel(opt, idx)}
              </option>
            ))}
          </select>
        )}
      </div>
    </div>
  )
}
