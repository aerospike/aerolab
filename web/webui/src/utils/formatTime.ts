export function formatTimeAgo(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSec = Math.floor(diffMs / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffHour = Math.floor(diffMin / 60)
  const diffDay = Math.floor(diffHour / 24)

  if (diffSec < 60) return 'just now'
  if (diffMin < 60) return `${diffMin}m ago`
  if (diffHour < 24) return `${diffHour}h ago`
  if (diffDay < 7) return `${diffDay}d ago`
  return date.toLocaleDateString()
}

export function formatDuration(startStr: string, endStr?: string): string {
  const start = new Date(startStr)
  const end = endStr ? new Date(endStr) : new Date()
  const diffMs = end.getTime() - start.getTime()
  const diffSec = Math.floor(diffMs / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffHour = Math.floor(diffMin / 60)

  if (diffSec < 60) return `${diffSec}s`
  if (diffMin < 60) return `${diffMin}m ${diffSec % 60}s`
  return `${diffHour}h ${diffMin % 60}m`
}

/**
 * Format a Go-style duration string (e.g. "3h23m22s") from total seconds.
 * Matches the output of Go's time.Duration.String() truncated to seconds.
 */
function goDurationString(totalSeconds: number): string {
  const negative = totalSeconds < 0
  let secs = Math.abs(Math.round(totalSeconds))
  const hours = Math.floor(secs / 3600)
  secs %= 3600
  const mins = Math.floor(secs / 60)
  secs %= 60
  let result = ''
  if (hours > 0) result += `${hours}h`
  if (mins > 0) result += `${mins}m`
  result += `${secs}s`
  return negative ? `-${result}` : result
}

const SIX_HOURS_MS = 6 * 60 * 60 * 1000
const ZERO_TIME = '0001-01-01T00:00:00Z'

/**
 * Format an expiry timestamp into a relative duration label with color class.
 * Matches CLI behavior: red for NEVER/expired, yellow for < 6h, default otherwise.
 */
export function formatExpiresIn(val: unknown): { label: string; color: string } {
  const red = 'text-red-600 dark:text-red-400 font-medium'
  const yellow = 'text-amber-600 dark:text-amber-400 font-medium'

  if (val == null || val === '' || val === ZERO_TIME) {
    return { label: 'NEVER', color: red }
  }

  const str = String(val)
  const date = new Date(str)
  if (isNaN(date.getTime()) || date.getFullYear() <= 1) {
    return { label: 'NEVER', color: red }
  }

  const now = Date.now()
  const diffMs = date.getTime() - now

  if (diffMs <= 0) {
    // Already expired — show negative duration
    return { label: goDurationString(diffMs / 1000), color: red }
  }

  const label = goDurationString(diffMs / 1000)
  if (diffMs < SIX_HOURS_MS) {
    return { label, color: yellow }
  }
  return { label, color: '' }
}
