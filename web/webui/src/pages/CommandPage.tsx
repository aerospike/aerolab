import { useRef, useState, useCallback } from 'react'
import { useParams } from 'react-router-dom'
import { useCommandInfo } from '@/hooks/useCommands'
import { useJobModal } from '@/contexts/JobModalContext'
import {
  executeCommand,
  executeCommandWithFiles,
  executeFileUpload,
  getDownloadUrl,
} from '@/api/client'
import type { CommandFormRef } from '@/components/command/CommandForm'
import CommandForm from '@/components/command/CommandForm'
import CLIPreview from '@/components/command/CLIPreview'
import Spinner from '@/components/common/Spinner'
import { toast } from 'sonner'

export default function CommandPage() {
  const { '*': path } = useParams<{ '*': string }>()
  const commandPath = path ?? ''
  const { openJobModal } = useJobModal()
  const { data: commandInfo, isLoading, error } = useCommandInfo(
    commandPath || undefined
  )

  const [formValues, setFormValues] = useState<Record<string, unknown>>({})
  const formRef = useRef<CommandFormRef>(null)

  const hasDownload =
    commandInfo?.parameters?.some((p) => p.webType === 'download') ?? false
  const hasUpload =
    commandInfo?.parameters?.some((p) => p.webType === 'upload') ?? false

  const hasCLIPreview =
    !!(commandInfo?.parameters && commandInfo.parameters.length > 0)

  const handleSubmit = useCallback(
    async (params: Record<string, unknown>) => {
      if (!commandPath || !commandInfo) return
      const hasFileValues = Object.values(params).some((v) => v instanceof File)

      try {
        if (hasDownload) {
          const url = getDownloadUrl(commandPath, params)
          window.open(url, '_blank', 'noopener,noreferrer')
          toast.success('Download started in new tab')
          return
        }

        if (hasUpload) {
          await executeFileUpload(commandPath, params)
          toast.success('Upload completed')
          return
        }

        if (hasFileValues) {
          const result = await executeCommandWithFiles(commandPath, params)
          toast.success('Job started')
          openJobModal(result.jobId)
          return
        }

        const result = await executeCommand(commandPath, params)
        if ('jobId' in result && result.jobId) {
          toast.success('Job started')
          openJobModal(result.jobId)
        }
      } catch (err) {
        toast.error(String(err instanceof Error ? err.message : err))
      }
    },
    [commandPath, commandInfo, hasDownload, hasUpload, openJobModal]
  )

  const handleRunFromPreview = useCallback(() => {
    formRef.current?.submit()
  }, [])

  if (isLoading) {
    return <Spinner text="Loading command..." />
  }

  if (error) {
    return (
      <div className="rounded-lg border border-red-200 bg-red-50 p-4 dark:border-red-800 dark:bg-red-900/20">
        <p className="mb-3 text-red-800 dark:text-red-200">
          Failed to load command: {String(error)}
        </p>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
        >
          Retry
        </button>
      </div>
    )
  }

  if (!commandPath) {
    return (
      <div className="flex flex-col items-center justify-center py-16">
        <p className="text-center text-slate-500 dark:text-slate-400">
          Select a command from the sidebar to get started
        </p>
      </div>
    )
  }

  if (!commandInfo) {
    return <Spinner text="Loading command..." />
  }

  return (
    <>
      <div className={hasCLIPreview ? 'pb-28' : 'pb-4'}>
        <h1 className="mb-4 text-2xl font-semibold text-slate-900 dark:text-slate-100">
          Command: {commandInfo?.displayName ?? commandInfo?.name ?? commandPath ?? '(root)'}
        </h1>

        {commandInfo.description && (
          <p className="mb-6 text-slate-600 dark:text-slate-400">
            {commandInfo.description}
          </p>
        )}

        <CommandForm
          ref={formRef}
          commandInfo={commandInfo}
          onSubmit={handleSubmit}
          onFormValuesChange={setFormValues}
        />
      </div>

      {hasCLIPreview && (
        <CLIPreview
          commandPath={commandPath}
          formValues={formValues}
          enabled={!!commandPath}
          onRun={handleRunFromPreview}
        />
      )}
    </>
  )
}
