export interface CLIToken {
  token: string
  className: string
}

/**
 * Splits a CLI string by spaces, respecting quoted strings.
 * Returns tokens with syntax highlighting classes:
 * - nv: command path parts (before any flag)
 * - na: flags/switches (tokens starting with -)
 * - s: values (tokens after a flag)
 */
export function highlightCLI(cli: string): CLIToken[] {
  if (!cli.trim()) return []

  const tokens: CLIToken[] = []
  let i = 0
  let sawFlag = false

  const skipWhitespace = () => {
    while (i < cli.length && /\s/.test(cli[i])) i++
  }

  const readToken = (): string | null => {
    skipWhitespace()
    if (i >= cli.length) return null

    if (cli[i] === '"' || cli[i] === "'") {
      const quote = cli[i]
      i++
      let token = ''
      while (i < cli.length && cli[i] !== quote) {
        if (cli[i] === '\\') {
          i++
          if (i < cli.length) token += cli[i++]
        } else {
          token += cli[i++]
        }
      }
      if (i < cli.length) i++ // consume closing quote
      return token
    }

    let token = ''
    while (i < cli.length && !/\s/.test(cli[i])) {
      token += cli[i++]
    }
    return token || null
  }

  let token: string | null
  while ((token = readToken()) !== null) {
    let className: string
    if (token.startsWith('-')) {
      sawFlag = true
      className = 'na'
    } else if (sawFlag) {
      className = 's'
    } else {
      className = 'nv'
    }
    tokens.push({ token, className })
  }

  return tokens
}
