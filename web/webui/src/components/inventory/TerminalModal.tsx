import { useRef, useEffect, useCallback, useState } from 'react'
import { X, Terminal as TerminalIcon, Loader2 } from 'lucide-react'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import { getConfig } from '@/utils/config'
import 'xterm/css/xterm.css'

interface TerminalModalProps {
  isOpen: boolean
  onClose: () => void
  type: 'cluster' | 'client' | 'agi'
  name: string
  nodeNo: number
}

type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'error'

export function TerminalModal({ isOpen, onClose, type, name, nodeNo }: TerminalModalProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const terminalRef = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const [connState, setConnState] = useState<ConnectionState>('connecting')
  const [errorMsg, setErrorMsg] = useState('')

  const cleanup = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }
    if (fitAddonRef.current) {
      fitAddonRef.current.dispose()
      fitAddonRef.current = null
    }
    if (terminalRef.current) {
      terminalRef.current.dispose()
      terminalRef.current = null
    }
  }, [])

  const handleClose = useCallback(() => {
    cleanup()
    setConnState('connecting')
    setErrorMsg('')
    onClose()
  }, [cleanup, onClose])

  // Handle Escape key
  useEffect(() => {
    if (!isOpen) return
    const handleKeyDown = (e: KeyboardEvent) => {
      // Only close on Escape if the terminal is not focused
      // (so users can still use Escape in the shell)
      if (e.key === 'Escape' && document.activeElement !== containerRef.current?.querySelector('.xterm-helper-textarea')) {
        handleClose()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      document.body.style.overflow = ''
    }
  }, [isOpen, handleClose])

  // Initialize terminal and WebSocket when modal opens
  useEffect(() => {
    if (!isOpen || !containerRef.current) return

    setConnState('connecting')
    setErrorMsg('')

    // Create xterm.js terminal
    const terminal = new Terminal({
      theme: {
        background: '#0f172a',
        foreground: '#e2e8f0',
        cursor: '#94a3b8',
        cursorAccent: '#0f172a',
        selectionBackground: '#475569',
        black: '#1e293b',
        red: '#f87171',
        green: '#4ade80',
        yellow: '#facc15',
        blue: '#60a5fa',
        magenta: '#c084fc',
        cyan: '#22d3ee',
        white: '#f8fafc',
        brightBlack: '#64748b',
        brightRed: '#f87171',
        brightGreen: '#4ade80',
        brightYellow: '#facc15',
        brightBlue: '#60a5fa',
        brightMagenta: '#c084fc',
        brightCyan: '#22d3ee',
        brightWhite: '#f8fafc',
      },
      cursorBlink: true,
      scrollback: 10000,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    })

    const fitAddon = new FitAddon()
    terminal.loadAddon(fitAddon)
    terminal.open(containerRef.current)

    // Small delay to let the container render before fitting
    requestAnimationFrame(() => {
      fitAddon.fit()
    })

    terminalRef.current = terminal
    fitAddonRef.current = fitAddon

    terminal.write('\x1b[33mConnecting to ' + name + ':' + nodeNo + '...\x1b[0m\r\n')

    // Build WebSocket URL
    const config = getConfig()
    const basePath = config.rootPath?.replace(/\/$/, '') || ''
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}${basePath}/api/terminal/ws?type=${encodeURIComponent(type)}&name=${encodeURIComponent(name)}&node=${nodeNo}`

    const ws = new WebSocket(wsUrl)
    ws.binaryType = 'arraybuffer'
    wsRef.current = ws

    ws.onopen = () => {
      setConnState('connected')
      terminal.write('\x1b[32mConnected.\x1b[0m\r\n\r\n')
      terminal.focus()

      // Send initial size
      const dimensions = fitAddon.proposeDimensions()
      if (dimensions) {
        ws.send(JSON.stringify({ type: 'resize', cols: dimensions.cols, rows: dimensions.rows }))
      }
    }

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        terminal.write(new Uint8Array(event.data))
      } else {
        terminal.write(event.data)
      }
    }

    ws.onclose = (event) => {
      setConnState('disconnected')
      if (!event.wasClean) {
        terminal.write('\r\n\x1b[31m*** Connection lost ***\x1b[0m\r\n')
      } else {
        terminal.write('\r\n\x1b[33m*** Connection closed ***\x1b[0m\r\n')
      }
    }

    ws.onerror = () => {
      setConnState('error')
      setErrorMsg('WebSocket connection failed')
      terminal.write('\r\n\x1b[31m*** Connection error ***\x1b[0m\r\n')
    }

    // Terminal input → WebSocket (as binary)
    terminal.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    })

    // Handle terminal resize
    const handleResize = () => {
      fitAddon.fit()
      if (ws.readyState === WebSocket.OPEN) {
        const dims = fitAddon.proposeDimensions()
        if (dims) {
          ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }))
        }
      }
    }

    window.addEventListener('resize', handleResize)
    const ro = new ResizeObserver(handleResize)
    if (containerRef.current) ro.observe(containerRef.current)

    return () => {
      ro.disconnect()
      window.removeEventListener('resize', handleResize)
      cleanup()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, type, name, nodeNo])

  if (!isOpen) return null

  const stateLabel = {
    connecting: 'Connecting...',
    connected: 'Connected',
    disconnected: 'Disconnected',
    error: 'Error',
  }[connState]

  const stateColor = {
    connecting: 'text-amber-400',
    connected: 'text-green-400',
    disconnected: 'text-slate-400',
    error: 'text-red-400',
  }[connState]

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="terminal-modal-title"
    >
      <div className="flex h-[90vh] w-[95vw] max-w-7xl flex-col rounded-lg border border-slate-700 bg-slate-900 shadow-2xl">
        {/* Header */}
        <header className="flex items-center justify-between gap-4 border-b border-slate-700 px-4 py-3">
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <TerminalIcon className="h-5 w-5 text-green-400" />
            <h2
              id="terminal-modal-title"
              className="truncate text-lg font-semibold text-slate-100"
            >
              {name}:{nodeNo}
            </h2>
            <span className="text-xs text-slate-500">({type})</span>
            <span className={`flex items-center gap-1.5 text-xs font-medium ${stateColor}`}>
              {connState === 'connecting' && <Loader2 className="h-3 w-3 animate-spin" />}
              {connState === 'connected' && <span className="h-2 w-2 rounded-full bg-green-500" />}
              {connState === 'disconnected' && <span className="h-2 w-2 rounded-full bg-slate-500" />}
              {connState === 'error' && <span className="h-2 w-2 rounded-full bg-red-500" />}
              {stateLabel}
            </span>
          </div>
          <button
            type="button"
            onClick={handleClose}
            className="rounded p-2 text-slate-400 hover:bg-slate-700 hover:text-slate-100"
            aria-label="Close terminal"
          >
            <X className="h-5 w-5" />
          </button>
        </header>

        {/* Terminal */}
        <div className="flex-1 overflow-hidden p-2">
          <div
            ref={containerRef}
            className="h-full w-full rounded overflow-hidden"
            style={{ backgroundColor: '#0f172a' }}
          />
        </div>

        {/* Footer */}
        <footer className="flex items-center justify-between border-t border-slate-700 px-4 py-2">
          <span className="text-xs text-slate-500">
            {errorMsg || 'Press Ctrl+D or type "exit" to end the session'}
          </span>
          <button
            type="button"
            onClick={handleClose}
            className="rounded-md bg-slate-700 px-3 py-1.5 text-sm font-medium text-slate-200 hover:bg-slate-600"
          >
            Disconnect
          </button>
        </footer>
      </div>
    </div>
  )
}
