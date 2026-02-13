import { useState } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import type { ParameterInfo } from '@/api/types'
import { cn } from '@/utils/cn'
import InfoTooltip from '@/components/common/InfoTooltip'

interface ParameterInputProps {
  param: ParameterInfo
  value: string
  onChange: (value: string) => void
  visible: boolean
  onVisibilityChange: (visible: boolean) => void
  showValidationError?: boolean
}

export default function ParameterInput({
  param,
  value,
  onChange,
  visible,
  onVisibilityChange,
  showValidationError = false,
}: ParameterInputProps) {
  const [touched, setTouched] = useState(false)
  const invalid = showValidationError && param.required && !value.trim()
  const hasError = touched && invalid

  const inputType =
    param.type === 'int' || param.type === 'uint'
      ? 'number'
      : param.type === 'float'
        ? 'number'
        : 'text'

  const inputProps = {
    min: param.type === 'uint' ? 0 : undefined,
    step: param.type === 'float' ? 'any' : undefined,
  }

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
        <input
          id={param.fieldName}
          type={inputType}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onBlur={() => setTouched(true)}
          placeholder={param.default}
          className={cn(
            'block w-full rounded-md border bg-white px-3 py-2 text-slate-900 shadow-sm transition-colors',
            'placeholder:text-slate-400 dark:placeholder:text-slate-500',
            'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-gray-900',
            hasError
              ? 'border-red-500 dark:border-red-500'
              : 'border-slate-300 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100'
          )}
          {...inputProps}
        />
        {hasError && (
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">This field is required</p>
        )}
      </div>
    </div>
  )
}
