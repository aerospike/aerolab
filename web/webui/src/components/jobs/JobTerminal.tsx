import { useRef, useImperativeHandle, forwardRef } from 'react'

export interface JobTerminalRef {
  write: (data: string) => void
  clear: () => void
  getContent: () => string
}

interface JobTerminalProps {
  ref?: React.RefObject<JobTerminalRef>
}

/* ------------------------------------------------------------------ */
/*  Stateful ANSI-to-HTML converter                                   */
/*  Tracks SGR state across successive write() calls so colours that  */
/*  span chunk boundaries render correctly.  Each toHtml() call       */
/*  returns self-contained HTML (all <span>s are closed).             */
/* ------------------------------------------------------------------ */

const FG: Record<number, string> = {
  30: '#1e293b', 31: '#f87171', 32: '#4ade80', 33: '#facc15',
  34: '#60a5fa', 35: '#c084fc', 36: '#22d3ee', 37: '#f8fafc',
  90: '#64748b', 91: '#f87171', 92: '#4ade80', 93: '#facc15',
  94: '#60a5fa', 95: '#c084fc', 96: '#22d3ee', 97: '#f8fafc',
}

const BG: Record<number, string> = {
  40: '#1e293b', 41: '#f87171', 42: '#4ade80', 43: '#facc15',
  44: '#60a5fa', 45: '#c084fc', 46: '#22d3ee', 47: '#f8fafc',
  100: '#64748b', 101: '#f87171', 102: '#4ade80', 103: '#facc15',
  104: '#60a5fa', 105: '#c084fc', 106: '#22d3ee', 107: '#f8fafc',
}

class AnsiConverter {
  private fg: string | null = null
  private bg: string | null = null
  private bold = false
  private dim = false
  private italic = false
  private underline = false

  /** Convert a chunk of text (possibly containing ANSI SGR codes) to HTML. */
  toHtml(text: string): string {
    let html = ''
    let spanOpen = false

    // Re-open a span for any state carried over from the previous chunk
    const initial = this.styleString()
    if (initial) {
      html += `<span style="${initial}">`
      spanOpen = true
    }

    let i = 0
    while (i < text.length) {
      // Detect ESC [ … m  (SGR sequence)
      if (text[i] === '\x1b' && i + 1 < text.length && text[i + 1] === '[') {
        let j = i + 2
        let seqEnd = -1
        while (j < text.length) {
          const ch = text[j]
          if (ch === 'm') { seqEnd = j; break }
          if ((ch < '0' || ch > '9') && ch !== ';') break
          j++
        }

        if (seqEnd >= 0) {
          if (spanOpen) { html += '</span>'; spanOpen = false }

          const raw = text.slice(i + 2, seqEnd)
          const codes = raw === '' ? [0] : raw.split(';').map(Number)
          for (const c of codes) this.applyCode(c)

          const s = this.styleString()
          if (s) { html += `<span style="${s}">`; spanOpen = true }

          i = seqEnd + 1
          continue
        }
        // Not a valid SGR — skip the ESC char and let '[' be handled next
        i++
        continue
      }

      // Regular characters — HTML-escape and skip \r
      const ch = text[i]
      if (ch === '<') html += '&lt;'
      else if (ch === '>') html += '&gt;'
      else if (ch === '&') html += '&amp;'
      else if (ch === '"') html += '&quot;'
      else if (ch !== '\r') html += ch

      i++
    }

    if (spanOpen) html += '</span>'
    return html
  }

  reset(): void {
    this.fg = null; this.bg = null
    this.bold = false; this.dim = false
    this.italic = false; this.underline = false
  }

  /* ---- private helpers ---- */

  private applyCode(code: number): void {
    if (code === 0) { this.reset(); return }
    if (code === 1) { this.bold = true; return }
    if (code === 2) { this.dim = true; return }
    if (code === 3) { this.italic = true; return }
    if (code === 4) { this.underline = true; return }
    if (code === 22) { this.bold = false; this.dim = false; return }
    if (code === 23) { this.italic = false; return }
    if (code === 24) { this.underline = false; return }
    if (code === 39) { this.fg = null; return }
    if (code === 49) { this.bg = null; return }
    if (FG[code]) { this.fg = FG[code]; return }
    if (BG[code]) { this.bg = BG[code]; return }
  }

  private styleString(): string {
    const p: string[] = []
    if (this.fg) p.push(`color:${this.fg}`)
    if (this.bg) p.push(`background-color:${this.bg}`)
    if (this.bold) p.push('font-weight:bold')
    if (this.dim) p.push('opacity:0.7')
    if (this.italic) p.push('font-style:italic')
    if (this.underline) p.push('text-decoration:underline')
    return p.join(';')
  }
}

/** Strip all ANSI escape sequences and carriage returns from text. */
function stripAnsi(text: string): string {
  return text.replace(/\x1b\[[0-9;]*[A-Za-z]/g, '').replace(/\r/g, '')
}

/* ------------------------------------------------------------------ */
/*  Component                                                         */
/* ------------------------------------------------------------------ */

function JobTerminalComponent(
  _props: JobTerminalProps,
  ref: React.ForwardedRef<JobTerminalRef>,
) {
  const preRef = useRef<HTMLPreElement>(null)
  const converterRef = useRef(new AnsiConverter())
  const rawContentRef = useRef('')

  useImperativeHandle(ref, () => ({
    write(data: string) {
      rawContentRef.current += data
      const el = preRef.current
      if (!el) return

      // Auto-scroll only when already near the bottom
      const nearBottom =
        el.scrollHeight - el.scrollTop - el.clientHeight < 40

      el.insertAdjacentHTML('beforeend', converterRef.current.toHtml(data))

      if (nearBottom) {
        el.scrollTop = el.scrollHeight
      }
    },
    clear() {
      rawContentRef.current = ''
      converterRef.current = new AnsiConverter()
      if (preRef.current) {
        preRef.current.innerHTML = ''
      }
    },
    getContent() {
      return stripAnsi(rawContentRef.current)
    },
  }))

  return (
    <pre
      ref={preRef}
      className="h-full w-full min-h-[200px] rounded font-mono text-sm"
      style={{
        backgroundColor: '#0f172a',
        color: '#e2e8f0',
        whiteSpace: 'pre',
        overflowX: 'auto',
        overflowY: 'auto',
        padding: '12px',
        margin: 0,
        tabSize: 8,
        cursor: 'text',
        lineHeight: 1.5,
      }}
    />
  )
}

export default forwardRef(JobTerminalComponent)
