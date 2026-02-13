// The server injects window.__AEROLAB_CONFIG__ before </head>
declare global {
  interface Window {
    __AEROLAB_CONFIG__?: {
      rootPath?: string
      version?: string
      forceSimpleMode?: boolean
    }
  }
}

export interface AerolabConfig {
  rootPath: string
  version: string
  forceSimpleMode: boolean
}

export function getConfig(): AerolabConfig {
  const config = window.__AEROLAB_CONFIG__ ?? {}
  return {
    rootPath: config.rootPath ?? '',
    version: config.version ?? '0.0.0',
    forceSimpleMode: config.forceSimpleMode ?? false,
  }
}

export function getRootPath(): string {
  return getConfig().rootPath
}
