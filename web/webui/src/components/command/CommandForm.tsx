import { useState, useEffect, useCallback, useImperativeHandle, forwardRef, useRef } from 'react'
import type { CommandInfo, ParameterInfo } from '@/api/types'
import { usePreferences } from '@/hooks/usePreferences'
import { useServerBrowse } from '@/contexts/ServerBrowseContext'
import ParameterInput from './ParameterInput'
import ToggleInput from './ToggleInput'
import SelectInput from './SelectInput'
import TagInput from './TagInput'
import FileInput from './FileInput'
import ParameterGroup from './ParameterGroup'
import SeparatorRow from './SeparatorRow'
import InfoTooltip from '@/components/common/InfoTooltip'
import { toast } from 'sonner'

interface CommandFormProps {
  commandInfo: CommandInfo
  onSubmit: (params: Record<string, unknown>) => void
  onFormValuesChange?: (values: Record<string, unknown>) => void
}

export interface CommandFormRef {
  submit: () => void
}

function getInputType(
  param: ParameterInfo
): 'input' | 'toggle' | 'select' | 'tag' | 'file' | 'textarea' | 'separator' {
  if (param.type === 'separator') return 'separator'
  if (param.type === 'bool') return 'toggle'
  if (param.webType === 'textarea') return 'textarea'
  if (
    param.webType === 'upload' ||
    param.webType === 'file' ||
    param.webType === 'download' ||
    param.isFile
  )
    return 'file'
  if (param.choices && param.choices.length > 0) return 'select'
  if (param.isSlice && !param.choices?.length) return 'tag'
  return 'input'
}

function buildInitialValues(params: ParameterInfo[] | undefined): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  if (!params) return out
  for (const p of params) {
    if (p.hidden || p.webHidden) continue
    if (p.type === 'bool') {
      out[p.fieldName] = p.default === 'true' ? 'true' : 'false'
    } else if (p.isSlice && !p.choices?.length) {
      out[p.fieldName] = []
    } else if (p.choices && p.isSlice) {
      out[p.fieldName] = p.default ? p.default.split(',') : []
    } else {
      out[p.fieldName] = p.default ?? ''
    }
  }
  return out
}

function applyUrlParams(
  params: ParameterInfo[] | undefined,
  initial: Record<string, unknown>,
  setValues: (updater: (prev: Record<string, unknown>) => Record<string, unknown>) => void,
  urlSetFields?: Set<string>
): Record<string, unknown> {
  if (!params || typeof window === 'undefined') return initial
  const search = new URLSearchParams(window.location.search)
  const out = { ...initial }

  for (const p of params) {
    const val =
      search.get(p.fieldName) ??
      search.get(p.long ?? '') ??
      (p.short ? search.get(p.short) : undefined)
    if (val === undefined || val === null) continue
    if (p.optional && urlSetFields) urlSetFields.add(p.fieldName)

    if (val === 'discover-caller-ip') {
      fetch('https://api.ipify.org?format=json')
        .then((r) => r.json())
        .then((d: { ip?: string }) => {
          if (d.ip) {
            setValues((prev) => ({ ...prev, [p.fieldName]: d.ip }))
          }
        })
        .catch(() => {})
      continue
    }

    if (p.type === 'bool') {
      out[p.fieldName] = val === 'true' || val === '1' ? 'true' : 'false'
    } else if (p.isSlice && !p.choices?.length) {
      out[p.fieldName] = val.split(',').map((s) => s.trim()).filter(Boolean)
    } else if (p.choices && p.isSlice) {
      out[p.fieldName] = val.split(',').map((s) => s.trim()).filter(Boolean)
    } else {
      out[p.fieldName] = val
    }
  }
  return out
}

/** Build the full CLI-compatible parameter key for a parameter (using long tag + namespace) */
function getParamKey(p: ParameterInfo): string {
  const base = p.name || p.fieldName
  if (p.namespace) return `${p.namespace}-${base}`
  return base
}

/** Translate form values from internal fieldName keys to CLI-compatible long tag keys */
function translateFormValues(
  values: Record<string, unknown>,
  params: ParameterInfo[]
): Record<string, unknown> {
  const mapping: Record<string, string> = {}
  for (const p of params) {
    mapping[p.fieldName] = getParamKey(p)
  }
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(values)) {
    out[mapping[k] ?? k] = v
  }
  return out
}

/** Check whether a form value still matches the parameter's declared default */
function isDefaultValue(value: unknown, param: ParameterInfo): boolean {
  const def = param.default ?? ''
  if (param.type === 'bool') {
    const valBool = value === 'true' || value === true
    const defBool = def === 'true'
    return valBool === defBool
  }
  if (param.isSlice) {
    if (Array.isArray(value)) {
      if (value.length === 0 && !def) return true
      return value.join(',') === def
    }
    return String(value) === def
  }
  // Numeric and string comparison: stringify both sides
  return String(value) === def
}

function groupParams(params: ParameterInfo[]): Map<string | null, ParameterInfo[]> {
  const map = new Map<string | null, ParameterInfo[]>()
  for (const p of params) {
    const g = p.group || null
    if (!map.has(g)) map.set(g, [])
    map.get(g)!.push(p)
  }
  return map
}

const CommandForm = forwardRef<CommandFormRef, CommandFormProps>(function CommandForm(
  { commandInfo, onSubmit, onFormValuesChange },
  ref
) {
  const { simpleMode } = usePreferences()
  const allowServerBrowse = useServerBrowse()
  const params = commandInfo.parameters ?? []

  const [formValues, setFormValues] = useState<Record<string, unknown>>(() =>
    buildInitialValues(params)
  )
  const [visibility, setVisibility] = useState<Record<string, boolean>>({})
  const [requiredMissing, setRequiredMissing] = useState<Set<string>>(new Set())

  const filteredParams = params.filter((p) => {
    if (p.hidden || p.webHidden) return false
    if (simpleMode && !p.simpleMode) return false
    return true
  })

  useEffect(() => {
    // Initialize visibility: optional non-toggle fields start hidden
    const initVis: Record<string, boolean> = {}
    for (const p of params) {
      if (p.optional && p.type !== 'bool') {
        initVis[p.fieldName] = false
      }
    }

    const urlSetFields = new Set<string>()
    const next = applyUrlParams(params, buildInitialValues(params), setFormValues, urlSetFields)
    setFormValues(next)

    // Reveal optional fields that were populated from URL params
    for (const f of urlSetFields) {
      initVis[f] = true
    }
    setVisibility(initVis)
  }, [commandInfo.path])

  useEffect(() => {
    onFormValuesChange?.(translateFormValues(formValues, params))
  }, [formValues, params, onFormValuesChange])

  const handleChange = useCallback((field: string, value: unknown) => {
    setFormValues((prev) => ({ ...prev, [field]: value }))
  }, [])

  const handleVisibilityChange = useCallback((field: string, visible: boolean) => {
    setVisibility((prev) => ({ ...prev, [field]: visible }))
  }, [])

  const validate = useCallback((): boolean => {
    const missing = new Set<string>()
    for (const p of filteredParams) {
      if (getInputType(p) === 'separator') continue
      if (!p.required) continue
      const v = formValues[p.fieldName]
      if (v === undefined || v === null) {
        missing.add(p.fieldName)
      } else if (typeof v === 'string' && !v.trim()) {
        missing.add(p.fieldName)
      } else if (Array.isArray(v) && v.length === 0) {
        missing.add(p.fieldName)
      }
    }
    setRequiredMissing(missing)
    if (missing.size > 0) {
      const first = Array.from(missing)[0]
      const param = params.find((q) => q.fieldName === first)
      toast.error(`Missing required field: ${param?.displayName ?? param?.name ?? first}`)
      return false
    }
    return true
  }, [filteredParams, formValues, params])

  const submitRef = useRef<() => void>(() => {})

  const handleSubmit = useCallback(() => {
    if (!validate()) return
    const toSend: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(formValues)) {
      if (visibility[k] === false) continue
      const param = params.find((p) => p.fieldName === k)
      // When user explicitly revealed an optional field (clicked eye), always include
      // the value even if empty - this allows clearing a previous setting.
      const explicitlyRevealed = param?.optional && visibility[k] === true
      if (v === '' && !param?.required && !explicitlyRevealed) continue
      if (Array.isArray(v) && v.length === 0 && !explicitlyRevealed) continue
      // Skip values that still match their default (but not explicitly revealed optional fields)
      if (param && !param.required && !explicitlyRevealed && isDefaultValue(v, param)) continue
      toSend[k] = v
    }
    onSubmit(translateFormValues(toSend, params))
  }, [formValues, visibility, params, validate, onSubmit])

  submitRef.current = handleSubmit

  useImperativeHandle(ref, () => ({
    submit: () => submitRef.current?.(),
  }))

  const groups = groupParams(filteredParams)
  const ungrouped = groups.get(null) ?? []

  const renderParam = (p: ParameterInfo) => {
    const inputType = getInputType(p)
    const visible = visibility[p.fieldName] ?? true
    const val = formValues[p.fieldName]

    if (inputType === 'separator') {
      return <SeparatorRow key={p.fieldName} label={p.displayName ?? p.name ?? p.fieldName} />
    }
    if (inputType === 'input') {
      return (
        <ParameterInput
          key={p.fieldName}
          param={p}
          value={String(val ?? p.default ?? '')}
          onChange={(v) => handleChange(p.fieldName, v)}
          visible={visible}
          onVisibilityChange={(v) => handleVisibilityChange(p.fieldName, v)}
          showValidationError={requiredMissing.has(p.fieldName)}
        />
      )
    }
    if (inputType === 'toggle') {
      return (
        <ToggleInput
          key={p.fieldName}
          param={p}
          value={String(val ?? p.default ?? '')}
          onChange={(v) => handleChange(p.fieldName, v)}
        />
      )
    }
    if (inputType === 'select') {
      return (
        <SelectInput
          key={p.fieldName}
          param={p}
          value={(val ?? p.default ?? (p.isSlice ? [] : '')) as string | string[]}
          onChange={(v) => handleChange(p.fieldName, v)}
          visible={visible}
          onVisibilityChange={(v) => handleVisibilityChange(p.fieldName, v)}
        />
      )
    }
    if (inputType === 'tag') {
      const arr = Array.isArray(val) ? val : val ? [val] : []
      return (
        <TagInput
          key={p.fieldName}
          param={p}
          values={arr.map(String)}
          onChange={(v) => handleChange(p.fieldName, v)}
        />
      )
    }
    if (inputType === 'file') {
      return (
        <FileInput
          key={p.fieldName}
          param={p}
          value={(val ?? '') as string | File}
          onChange={(v) => handleChange(p.fieldName, v)}
          allowServerBrowse={allowServerBrowse}
        />
      )
    }
    return (
      <div key={p.fieldName} className="flex items-start gap-4">
        <label className="w-48 shrink-0 pt-2 text-sm font-medium text-slate-700 dark:text-slate-300">
          {p.required && '* '}
          {p.displayName ?? p.name}
          {p.description && <InfoTooltip text={p.description} />}
        </label>
        <textarea
          value={String(val ?? '')}
          onChange={(e) => handleChange(p.fieldName, e.target.value)}
          placeholder={p.default}
          rows={4}
          className="min-w-0 flex-1 rounded-md border border-slate-300 bg-white px-3 py-2 text-slate-900 shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100 dark:focus:ring-offset-gray-900"
        />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <button
        type="button"
        onClick={handleSubmit}
        className="rounded-md bg-blue-600 px-4 py-2 font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-gray-900"
      >
        Run
      </button>

      <div className="space-y-3">
        {ungrouped.map((p) => renderParam(p))}
        {Array.from(groups.entries())
          .filter(([g]) => g != null)
          .map(([group, groupParams]) => (
            <ParameterGroup
              key={group!}
              group={group!}
              parameters={groupParams}
              formValues={formValues}
              onChange={handleChange}
              visibility={visibility}
              onVisibilityChange={handleVisibilityChange}
              requiredMissing={requiredMissing}
            />
          ))}
      </div>

      <button
        type="button"
        onClick={handleSubmit}
        className="rounded-md bg-blue-600 px-4 py-2 font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-gray-900"
      >
        Run
      </button>
    </div>
  )
})

export default CommandForm
