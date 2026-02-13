import type { ParameterInfo } from '@/api/types'
import { cn } from '@/utils/cn'
import InfoTooltip from '@/components/common/InfoTooltip'

interface ToggleInputProps {
  param: ParameterInfo
  value: string
  onChange: (value: string) => void
}

function isOptionalBoolean(param: ParameterInfo): boolean {
  return !!param.optional || !!param.noDefault || param.default === '' || param.default === undefined
}

export default function ToggleInput({ param, value, onChange }: ToggleInputProps) {
  const optional = isOptionalBoolean(param)

  if (optional) {
    return (
      <div className="flex items-start gap-4">
        <label className="w-48 shrink-0 pt-1.5 text-sm font-medium text-slate-700 dark:text-slate-300">
          {param.displayName ?? param.name}
          {param.description && <InfoTooltip text={param.description} />}
        </label>
        <div className="flex gap-2">
          {(['true', '', 'false'] as const).map((v) => {
            const label = v === '' ? 'UNSET' : v === 'true' ? 'ON' : 'OFF'
            const isActive = value === v
            return (
              <button
                key={label}
                type="button"
                onClick={() => onChange(v)}
                className={cn(
                  'rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-blue-600 text-white'
                    : 'bg-slate-100 text-slate-600 hover:bg-slate-200 dark:bg-slate-700 dark:text-slate-300 dark:hover:bg-slate-600'
                )}
              >
                {label}
              </button>
            )
          })}
        </div>
      </div>
    )
  }

  const checked = value === 'true'
  return (
    <div className="flex items-center gap-4">
      <label className="w-48 shrink-0 text-sm font-medium text-slate-700 dark:text-slate-300">
        {param.displayName ?? param.name}
        {param.description && <InfoTooltip text={param.description} />}
      </label>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        onClick={() => onChange(checked ? 'false' : 'true')}
        className={cn(
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full transition-colors',
          'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-gray-900',
          checked ? 'bg-blue-600' : 'bg-slate-300 dark:bg-slate-600'
        )}
      >
        <span
          className={cn(
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition',
            checked ? 'translate-x-5' : 'translate-x-0.5'
          )}
        />
      </button>
    </div>
  )
}
