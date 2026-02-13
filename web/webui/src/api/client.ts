import { getConfig } from '@/utils/config'
import type {
  CommandInfo,
  HealthResponse,
  Job,
  JobListResponse,
  JobSubmitResponse,
  GenerateCLIRequest,
  GenerateCLIResponse,
} from './types'

function getBaseUrl(): string {
  const config = getConfig()
  return config.rootPath || ''
}

async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const base = getBaseUrl()
  const url = base ? `${base}${path}` : path

  const response = await fetch(url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
  })

  if (!response.ok) {
    const errorText = await response.text()
    throw new Error(errorText || `HTTP ${response.status}: ${response.statusText}`)
  }

  return response.json()
}

export async function fetchCommands(): Promise<CommandInfo> {
  return apiFetch<CommandInfo>('/api/commands')
}

export async function fetchCommandInfo(path: string): Promise<CommandInfo> {
  return apiFetch<CommandInfo>(`/api/commands/${path}`)
}

export interface ListJobsParams {
  status?: string
  all?: boolean
}

export async function fetchJobs(params?: ListJobsParams): Promise<JobListResponse> {
  const searchParams = new URLSearchParams()
  if (params?.status) searchParams.set('status', params.status)
  if (params?.all) searchParams.set('all', 'true')
  const query = searchParams.toString()
  const path = query ? `/api/jobs?${query}` : '/api/jobs'
  return apiFetch<JobListResponse>(path)
}

export async function fetchJobDetails(id: string): Promise<Job> {
  return apiFetch<Job>(`/api/jobs/${id}`)
}

export async function cancelJob(id: string, force?: boolean): Promise<unknown> {
  const query = force ? '?force=true' : ''
  return apiFetch(`/api/jobs/${id}${query}`, { method: 'DELETE' })
}

export interface JobLogsResponse {
  jobId: string
  status: string
  logs: string
}

export async function fetchJobLogs(id: string): Promise<JobLogsResponse> {
  return apiFetch<JobLogsResponse>(`/api/jobs/${id}/logs`)
}

export async function executeCommand(
  path: string,
  params: Record<string, unknown>,
  dryRun?: boolean
): Promise<JobSubmitResponse> {
  const base = getBaseUrl()
  const query = dryRun ? '?dryRun=true' : ''
  const url = base ? `${base}/${path}${query}` : `/${path}${query}`

  const response = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  })

  if (!response.ok) {
    const errorText = await response.text()
    throw new Error(errorText || `HTTP ${response.status}: ${response.statusText}`)
  }

  return response.json()
}

/**
 * Executes a command that contains File values via multipart/form-data.
 * Non-file params are sent as a JSON blob in the _params field.
 * File params are sent as multipart file uploads keyed by field name.
 * Returns a job submission response (async execution).
 */
export async function executeCommandWithFiles(
  path: string,
  params: Record<string, unknown>
): Promise<JobSubmitResponse> {
  const base = getBaseUrl()
  const url = base ? `${base}/${path}` : `/${path}`

  const form = new FormData()
  const jsonParams: Record<string, unknown> = {}

  for (const [k, v] of Object.entries(params)) {
    if (v instanceof File) {
      form.append(k, v)
    } else {
      jsonParams[k] = v
    }
  }

  form.append('_params', JSON.stringify(jsonParams))

  const response = await fetch(url, { method: 'POST', body: form })

  if (!response.ok) {
    const errorText = await response.text()
    throw new Error(errorText || `HTTP ${response.status}: ${response.statusText}`)
  }

  return response.json()
}

export async function generateCLI(req: GenerateCLIRequest): Promise<GenerateCLIResponse> {
  return apiFetch<GenerateCLIResponse>('/api/generate-cli', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

/**
 * Executes a file upload command via multipart/form-data.
 * Returns a result object (success, results, errors) - not a job.
 */
export async function executeFileUpload(
  path: string,
  params: Record<string, unknown>
): Promise<unknown> {
  const base = getBaseUrl()
  const url = base ? `${base}/${path}` : `/${path}`

  const form = new FormData()
  const toFormKey: Record<string, string> = {
    name: 'cluster',
    path: 'destination',
    nodes: 'nodes',
  }
  for (const [k, v] of Object.entries(params)) {
    if (v instanceof File) {
      form.append('file', v)
    } else if (v !== undefined && v !== null && v !== '') {
      const formKey = toFormKey[k] ?? k
      form.append(formKey, String(v))
    }
  }

  const response = await fetch(url, { method: 'POST', body: form })

  if (!response.ok) {
    const errorText = await response.text()
    throw new Error(errorText || `HTTP ${response.status}: ${response.statusText}`)
  }

  return response.json()
}

/**
 * Returns the full GET URL for file download commands (webType: download).
 * Open in new window to stream the file to the browser.
 */
export function getDownloadUrl(path: string, params: Record<string, unknown>): string {
  const base = getBaseUrl()
  const prefix = base ? base.replace(/\/$/, '') : ''
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') {
      q.set(k, String(v))
    }
  }
  const query = q.toString()
  const pathPart = path.startsWith('/') ? path : `/${path}`
  const fullPath = prefix + pathPart + (query ? `?${query}` : '')
  return typeof window !== 'undefined' ? `${window.location.origin}${fullPath}` : fullPath
}

export async function fetchHealth(): Promise<HealthResponse> {
  return apiFetch<HealthResponse>('/api/health')
}

export async function fetchInventory(type: string): Promise<unknown[]> {
  return apiFetch<unknown[]>(`/api/inventory/${type}`)
}

export interface InventorySchema {
  backend: 'aws' | 'gcp' | 'docker'
  entities: Record<string, Array<{ name: string; field: string }>>
}

export async function fetchInventorySchema(): Promise<InventorySchema> {
  return apiFetch<InventorySchema>('/api/inventory/schema')
}

export interface FsHomedirResponse {
  path: string
}

export async function fetchFsHomedir(path?: string): Promise<FsHomedirResponse> {
  const q = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiFetch<FsHomedirResponse>(`/api/fs/homedir${q}`)
}

export interface FsLsResponse {
  path: string
  dirs: string[]
  files: string[]
}

export async function fetchFsLs(path: string): Promise<FsLsResponse> {
  const q = `?path=${encodeURIComponent(path)}`
  return apiFetch<FsLsResponse>(`/api/fs/ls${q}`)
}

export interface InventoryActionItem {
  clusterName: string
  nodeNo: number
}

export interface InventoryActionRequest {
  items: InventoryActionItem[]
  action: string
  type: string
  params?: Record<string, unknown>
}

export interface InventoryActionResponse {
  jobId: string
  jobIds?: string[]
}

export async function inventoryAction(body: InventoryActionRequest): Promise<InventoryActionResponse> {
  return apiFetch<InventoryActionResponse>('/api/inventory/action', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export interface InventoryConnectClusterParams {
  name: string
  node: number
}

export async function inventoryConnectCluster(params: InventoryConnectClusterParams): Promise<{
  host: string
  publicIP: string
  privateIP: string
  clusterName: string
  nodeNo: number
  sshUser: string
  sshKeyPath: string | string[]
}> {
  const q = new URLSearchParams({ name: params.name, node: String(params.node) })
  return apiFetch(`/api/inventory/connect/cluster?${q}`)
}

export async function inventoryConnectClient(params: InventoryConnectClusterParams): Promise<{
  host: string
  publicIP: string
  privateIP: string
  clusterName: string
  nodeNo: number
  sshUser: string
  sshKeyPath: string | string[]
}> {
  const q = new URLSearchParams({ name: params.name, node: String(params.node) })
  return apiFetch(`/api/inventory/connect/client?${q}`)
}

export async function inventoryConnectAgi(name: string): Promise<{
  accessURL: string
  name: string
  publicIP: string
  privateIP: string
}> {
  return apiFetch('/api/inventory/connect/agi', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export interface InventoryConnectTrinoParams {
  name: string
  node: number
  namespace: string
}

export async function inventoryConnectTrino(params: InventoryConnectTrinoParams): Promise<{
  host: string
  port: number
  namespace: string
  url: string
}> {
  const q = new URLSearchParams({
    name: params.name,
    node: String(params.node),
    namespace: params.namespace || '',
  })
  return apiFetch(`/api/inventory/connect/trino?${q}`)
}

export interface InventoryConnectGraphParams {
  name: string
  node: number
}

export async function inventoryConnectGraph(params: InventoryConnectGraphParams): Promise<{
  accessURL: string
  host: string
  clusterName: string
  nodeNo: string
}> {
  const q = new URLSearchParams({ name: params.name, node: String(params.node) })
  return apiFetch(`/api/inventory/connect/graph?${q}`)
}
