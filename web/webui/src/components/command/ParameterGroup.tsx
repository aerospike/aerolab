import { useState } from 'react'
import type { ParameterInfo } from '@/api/types'
import { useServerBrowse } from '@/contexts/ServerBrowseContext'
import ParameterInput from './ParameterInput'
import ToggleInput from './ToggleInput'
import SelectInput from './SelectInput'
import TagInput from './TagInput'
import FileInput from './FileInput'
import { cn } from '@/utils/cn'
import InfoTooltip from '@/components/common/InfoTooltip'

interface ParameterGroupProps {
  group: string
  parameters: ParameterInfo[]
  formValues: Record<string, unknown>
  onChange: (field: string, value: unknown) => void
  visibility: Record<string, boolean>
  onVisibilityChange: (field: string, visible: boolean) => void
  requiredMissing: Set<string>
}

function getInputComponent(
  param: ParameterInfo
): 'input' | 'toggle' | 'select' | 'tag' | 'file' | 'textarea' {
  if (param.type === 'bool') return 'toggle'
  if (param.webType === 'textarea') return 'textarea'
  if (param.webType === 'upload' || param.webType === 'file' || param.webType === 'download' || param.isFile)
    return 'file'
  if (param.choices && param.choices.length > 0) return 'select'
  if (param.isSlice && !param.choices?.length) return 'tag'
  return 'input'
}

export default function ParameterGroup({
  group,
  parameters,
  formValues,
  onChange,
  visibility,
  onVisibilityChange,
  requiredMissing,
}: ParameterGroupProps) {
  const allowServerBrowse = useServerBrowse()
  const allHaveDefaults = parameters.every((p) => p.default !== undefined && p.default !== '')
  const [collapsed, setCollapsed] = useState(allHaveDefaults)

  return (
    <div className="rounded-lg border border-slate-200 dark:border-slate-700">
      <button
        type="button"
        onClick={() => setCollapsed(!collapsed)}
        className="flex w-full items-center justify-between px-4 py-3 text-left font-medium text-slate-700 hover:bg-slate-50 dark:text-slate-300 dark:hover:bg-slate-800"
      >
        {group}
        <span className={cn('inline-block transition-transform', collapsed && '-rotate-90')}>
          ▼
        </span>
      </button>
      {!collapsed && (
        <div className="space-y-3 border-t border-slate-200 px-4 py-3 dark:border-slate-700">
          {parameters.map((param) => {
            const visible = visibility[param.fieldName] ?? true
            const inputType = getInputComponent(param)
            const val = formValues[param.fieldName]

            if (inputType === 'input') {
              return (
                <ParameterInput
                  key={param.fieldName}
                  param={param}
                  value={String(val ?? param.default ?? '')}
                  onChange={(v) => onChange(param.fieldName, v)}
                  visible={visible}
                  onVisibilityChange={(v) => onVisibilityChange(param.fieldName, v)}
                  showValidationError={requiredMissing.has(param.fieldName)}
                />
              )
            }
            if (inputType === 'toggle') {
              return (
                <ToggleInput
                  key={param.fieldName}
                  param={param}
                  value={String(val ?? param.default ?? '')}
                  onChange={(v) => onChange(param.fieldName, v)}
                />
              )
            }
            if (inputType === 'select') {
              return (
                <SelectInput
                  key={param.fieldName}
                  param={param}
                  value={(val ?? param.default ?? (param.isSlice ? [] : '')) as string | string[]}
                  onChange={(v) => onChange(param.fieldName, v)}
                  visible={visible}
                  onVisibilityChange={(v) => onVisibilityChange(param.fieldName, v)}
                />
              )
            }
            if (inputType === 'tag') {
              const arr = Array.isArray(val) ? val : val ? [val] : []
              return (
                <TagInput
                  key={param.fieldName}
                  param={param}
                  values={arr.map(String)}
                  onChange={(v) => onChange(param.fieldName, v)}
                />
              )
            }
            if (inputType === 'file') {
              return (
                <FileInput
                  key={param.fieldName}
                  param={param}
                  value={(val ?? '') as string | File}
                  onChange={(v) => onChange(param.fieldName, v)}
                  allowServerBrowse={allowServerBrowse}
                />
              )
            }
            return (
              <div key={param.fieldName} className="flex items-start gap-4">
                <label className="w-48 shrink-0 pt-2 text-sm font-medium text-slate-700 dark:text-slate-300">
                  {param.required && '* '}
                  {param.displayName ?? param.name}
                  {param.description && <InfoTooltip text={param.description} />}
                </label>
                <textarea
                  value={String(val ?? '')}
                  onChange={(e) => onChange(param.fieldName, e.target.value)}
                  placeholder={param.default}
                  rows={4}
                  className={cn(
                    'min-w-0 flex-1 rounded-md border bg-white px-3 py-2 text-slate-900 shadow-sm',
                    'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-gray-900',
                    'border-slate-300 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100'
                  )}
                />
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
