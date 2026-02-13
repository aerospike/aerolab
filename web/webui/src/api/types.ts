export interface CommandInfo {
  name: string
  displayName?: string
  path: string
  description: string
  icon?: string
  children?: CommandInfo[]
  parameters?: ParameterInfo[]
  hasChildren: boolean
  hidden: boolean
  webHidden?: boolean
  simpleMode: boolean
}

export interface ParameterInfo {
  name: string
  displayName?: string
  fieldName: string
  short?: string
  long?: string
  description?: string
  type: string
  default?: string
  required?: boolean
  webType?: string // "upload", "download", "textarea", "file"
  choices?: string[]
  choiceLabels?: string[]
  choicesMethod?: string
  hidden?: boolean
  webHidden?: boolean
  simpleMode: boolean
  group?: string
  namespace?: string
  noDefault?: boolean
  isSlice?: boolean
  isPositional?: boolean
  isFile?: boolean
  optional?: boolean
}

export interface Job {
  id: string
  user: string
  commandPath: string
  parameters: Record<string, unknown>
  cliCommand: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'error'
  createdAt: string
  startedAt?: string
  completedAt?: string
  error?: string
  refreshInventory?: boolean
  pid?: number
  exitCode?: number
  cancelled?: boolean
  timedOut?: boolean
  reloadRequired?: boolean
}

export interface JobListResponse {
  jobs: Job[]
  count: number
}

export interface HealthResponse {
  status: string
  version: string
  allowServerBrowse?: boolean
}

export interface GenerateCLIRequest {
  commandPath: string
  parameters?: Record<string, unknown>
  preferShort?: boolean
  includeDefaults?: boolean
}

export interface GenerateCLIResponse {
  cli: string
}

export interface JobSubmitResponse {
  jobId: string
  user: string
  commandPath: string
  cliCommand: string
  status: string
  createdAt: string
  statusUrl?: string
  logsUrl?: string
  logsStreamUrl?: string
}

export interface AerolabConfig {
  rootPath: string
  version: string
}
