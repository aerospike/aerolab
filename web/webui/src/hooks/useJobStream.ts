import { useState, useEffect, useCallback, useRef } from 'react'
import { getRootPath } from '@/utils/config'

export function useJobStream(
  jobId: string | null,
  onData: (data: string) => void,
  onComplete: (status: string, error: string, reloadRequired?: boolean) => void
) {
  const [status, setStatus] = useState<string>('')
  const [isConnected, setIsConnected] = useState(false)
  const eventSourceRef = useRef<EventSource | null>(null)

  const disconnect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
      eventSourceRef.current = null
      setIsConnected(false)
    }
  }, [])

  useEffect(() => {
    if (!jobId) {
      disconnect()
      return
    }

    const base = getRootPath()
    const prefix = base ? base.replace(/\/$/, '') : ''
    const path = `${prefix}/api/jobs/${jobId}/logs/stream`
    const url = path.startsWith('/')
      ? `${window.location.origin}${path}`
      : `${window.location.origin}/${path}`

    const es = new EventSource(url)
    eventSourceRef.current = es
    setIsConnected(true)

    es.addEventListener('status', (e: MessageEvent) => {
      setStatus(e.data ?? '')
    })

    es.addEventListener('error', (e: MessageEvent) => {
      const err = e.data ?? 'Connection error'
      setStatus('error')
      onComplete('error', err)
      disconnect()
    })

    es.addEventListener('complete', (e: MessageEvent) => {
      try {
        const payload = JSON.parse(e.data ?? '{}')
        const st = payload.status ?? 'completed'
        const err = payload.error ?? ''
        const reloadRequired = !!payload.reloadRequired
        setStatus(st)
        onComplete(st, err, reloadRequired)
      } catch {
        setStatus('completed')
        onComplete('completed', '', false)
      }
      disconnect()
    })

    es.onmessage = (e: MessageEvent) => {
      if (e.data) {
        onData(e.data)
      }
    }

    es.onerror = () => {
      if (es.readyState === EventSource.CLOSED) return
      setIsConnected(false)
      disconnect()
    }

    return () => {
      disconnect()
    }
  }, [jobId, onData, onComplete, disconnect])

  return { status, isConnected, disconnect }
}
