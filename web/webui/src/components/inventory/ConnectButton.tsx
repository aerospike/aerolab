import { useState } from 'react'
import { toast } from 'sonner'
import {
  inventoryConnectAgi,
  inventoryConnectTrino,
  inventoryConnectGraph,
} from '@/api/client'
import { PromptDialog } from '@/components/common/PromptDialog'
import { TerminalModal } from './TerminalModal'

interface ConnectButtonProps {
  type: 'cluster' | 'client' | 'agi' | 'trino' | 'graph'
  name: string
  nodeNo: number
  isRunning: boolean
  accessUrl?: string
}

export function ConnectButton({
  type,
  name,
  nodeNo,
  isRunning: running,
}: ConnectButtonProps) {
  const [loading, setLoading] = useState(false)
  const [trinoNamespaceOpen, setTrinoNamespaceOpen] = useState(false)
  const [terminalOpen, setTerminalOpen] = useState(false)

  const handleAttach = () => {
    if (!running) return
    setTerminalOpen(true)
  }

  const handleAgiConnect = async () => {
    if (!running) return
    setLoading(true)
    try {
      const res = await inventoryConnectAgi(name)
      window.open(res.accessURL, '_blank')
    } catch (err) {
      toast.error(String(err instanceof Error ? err.message : err))
    } finally {
      setLoading(false)
    }
  }

  const handleTrinoConnect = () => {
    if (!running) return
    setTrinoNamespaceOpen(true)
  }

  const handleTrinoSubmit = async (namespace: string) => {
    setLoading(true)
    try {
      const res = await inventoryConnectTrino({ name, node: nodeNo, namespace })
      window.open(res.url, '_blank')
    } catch (err) {
      toast.error(String(err instanceof Error ? err.message : err))
    } finally {
      setLoading(false)
    }
  }

  const handleGraphConnect = async () => {
    if (!running) return
    setLoading(true)
    try {
      const res = await inventoryConnectGraph({ name, node: nodeNo })
      window.open(res.accessURL, '_blank')
    } catch (err) {
      toast.error(String(err instanceof Error ? err.message : err))
    } finally {
      setLoading(false)
    }
  }

  const handleClick = () => {
    switch (type) {
      case 'cluster':
      case 'client':
        handleAttach()
        break
      case 'agi':
        handleAgiConnect()
        break
      case 'trino':
        handleTrinoConnect()
        break
      case 'graph':
        handleGraphConnect()
        break
    }
  }

  const getLabel = () => {
    switch (type) {
      case 'cluster':
      case 'client':
        return 'Attach'
      case 'agi':
        return 'Connect'
      case 'trino':
        return 'TrinoCLI'
      case 'graph':
        return 'GremlinConsole'
      default:
        return 'Connect'
    }
  }

  // Determine the terminal connect type mapping
  const getTerminalType = (): 'cluster' | 'client' | 'agi' => {
    if (type === 'cluster') return 'cluster'
    if (type === 'client') return 'client'
    return 'agi'
  }

  return (
    <>
      <button
        type="button"
        onClick={handleClick}
        disabled={!running || loading}
        className="rounded bg-green-600 px-2 py-1 text-xs font-medium text-white hover:bg-green-700 disabled:cursor-not-allowed disabled:bg-slate-300 disabled:text-slate-500 dark:disabled:bg-slate-600"
      >
        {loading ? '...' : getLabel()}
      </button>
      <PromptDialog
        isOpen={trinoNamespaceOpen}
        onClose={() => setTrinoNamespaceOpen(false)}
        onSubmit={handleTrinoSubmit}
        title="Trino CLI"
        message="Enter namespace to connect to"
        defaultValue="test"
        placeholder="test"
      />
      <TerminalModal
        isOpen={terminalOpen}
        onClose={() => setTerminalOpen(false)}
        type={getTerminalType()}
        name={name}
        nodeNo={nodeNo}
      />
    </>
  )
}

/**
 * AttachButton renders only the terminal attach button (xterm.js over WebSocket).
 * Used alongside ConnectButton when both attach and connect are needed (e.g. AGI).
 */
interface AttachButtonProps {
  type: 'cluster' | 'client' | 'agi'
  name: string
  nodeNo: number
  isRunning: boolean
}

export function AttachButton({ type, name, nodeNo, isRunning: running }: AttachButtonProps) {
  const [terminalOpen, setTerminalOpen] = useState(false)

  return (
    <>
      <button
        type="button"
        onClick={() => running && setTerminalOpen(true)}
        disabled={!running}
        className="rounded bg-blue-600 px-2 py-1 text-xs font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-slate-300 disabled:text-slate-500 dark:disabled:bg-slate-600"
      >
        Attach
      </button>
      <TerminalModal
        isOpen={terminalOpen}
        onClose={() => setTerminalOpen(false)}
        type={type}
        name={name}
        nodeNo={nodeNo}
      />
    </>
  )
}
