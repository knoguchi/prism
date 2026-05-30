import type {
  CaptureListItem,
  CaptureDetail,
  PaginatedResponse,
  SearchResult,
  HostSchemas,
  Stats,
} from './types'

const API_BASE = '/api'

async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
    },
    ...options,
  })

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: response.statusText }))
    throw new Error(error.message || `HTTP ${response.status}`)
  }

  return response.json()
}

export interface ListCapturesParams {
  host?: string
  hosts?: string[]  // Multiple hosts filter
  method?: string
  status?: string
  path?: string
  content_type?: string
  page?: number
  limit?: number
  limit_per_path?: number  // For selected hosts: max N per host+path (accumulates)
  ephemeral_limit?: number  // For non-selected hosts: recent traffic limit
}

export async function listCaptures(params: ListCapturesParams = {}): Promise<PaginatedResponse<CaptureListItem>> {
  const searchParams = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== '') {
      if (key === 'hosts' && Array.isArray(value)) {
        // Add each host as hosts[]
        value.forEach(h => searchParams.append('hosts[]', h))
      } else {
        searchParams.set(key, String(value))
      }
    }
  })

  const query = searchParams.toString()
  return fetchJSON(`${API_BASE}/captures${query ? `?${query}` : ''}`)
}

export async function getCapture(id: string): Promise<CaptureDetail> {
  return fetchJSON(`${API_BASE}/captures/${id}`)
}

export async function deleteCapture(id: string): Promise<void> {
  await fetch(`${API_BASE}/captures/${id}`, { method: 'DELETE' })
}

export async function clearCaptures(host?: string): Promise<{ deleted: number }> {
  const params = host ? `?host=${encodeURIComponent(host)}` : ''
  return fetchJSON(`${API_BASE}/captures${params}`, { method: 'DELETE' })
}

export async function searchTraffic(query: string, options?: { host?: string; limit?: number }): Promise<SearchResult> {
  const params = new URLSearchParams({ q: query })
  if (options?.host) params.set('host', options.host)
  if (options?.limit) params.set('limit', String(options.limit))

  return fetchJSON(`${API_BASE}/search?${params}`)
}

export async function listSchemas(host?: string): Promise<{ schemas: HostSchemas[] }> {
  const params = host ? `?host=${encodeURIComponent(host)}` : ''
  return fetchJSON(`${API_BASE}/schemas${params}`)
}

export async function getSchema(host: string, format: string, method?: string, path?: string): Promise<string> {
  const params = new URLSearchParams({ format })
  if (method) params.set('method', method)
  if (path) params.set('path', path)

  const response = await fetch(`${API_BASE}/schemas/${encodeURIComponent(host)}?${params}`)
  return response.text()
}

export async function getSchemaByFormat(host: string, format: string): Promise<{ content: string } | null> {
  try {
    const result = await fetchJSON<{ content: string }>(`${API_BASE}/schemas/${encodeURIComponent(host)}/format/${format}`)
    return result
  } catch {
    return null
  }
}

export async function getStats(period?: string): Promise<Stats> {
  const params = period ? `?period=${period}` : ''
  return fetchJSON(`${API_BASE}/stats${params}`)
}

// Host listing (lightweight - just host names with counts)
export interface HostInfo {
  host: string
  capture_count: number
  last_seen_at: string
}

export async function getHosts(): Promise<HostInfo[]> {
  const result = await fetchJSON<{ hosts: HostInfo[] }>(`${API_BASE}/hosts`)
  return result.hosts || []
}

// Enabled hosts management
export async function getEnabledHosts(): Promise<string[]> {
  const result = await fetchJSON<{ hosts: string[] }>(`${API_BASE}/hosts/enabled`)
  return result.hosts || []
}

export async function setEnabledHosts(hosts: string[]): Promise<string[]> {
  const result = await fetchJSON<{ hosts: string[] }>(`${API_BASE}/hosts/enabled`, {
    method: 'PUT',
    body: JSON.stringify({ hosts }),
  })
  return result.hosts || []
}

export async function addEnabledHost(host: string): Promise<string[]> {
  const result = await fetchJSON<{ hosts: string[] }>(`${API_BASE}/hosts/enabled/${encodeURIComponent(host)}`, {
    method: 'POST',
  })
  return result.hosts || []
}

export async function removeEnabledHost(host: string): Promise<string[]> {
  const result = await fetchJSON<{ hosts: string[] }>(`${API_BASE}/hosts/enabled/${encodeURIComponent(host)}`, {
    method: 'DELETE',
  })
  return result.hosts || []
}

// AI Inference
export interface InferenceStatus {
  host: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  message?: string
}

export async function startAnalysis(host: string): Promise<InferenceStatus> {
  return fetchJSON(`${API_BASE}/inference/${encodeURIComponent(host)}`, {
    method: 'POST',
  })
}

export async function getInferenceStatus(host: string): Promise<InferenceStatus> {
  return fetchJSON(`${API_BASE}/inference/${encodeURIComponent(host)}/status`)
}

export async function getHostDiagram(host: string): Promise<{ host: string; diagram: string }> {
  return fetchJSON(`${API_BASE}/inference/${encodeURIComponent(host)}/diagram`)
}

// Validation
export interface ValidationError {
  location: string
  field?: string
  message: string
  expected?: string
  actual?: string
}

export interface ValidationResult {
  request_id: string
  host: string
  method: string
  path: string
  valid: boolean
  errors?: ValidationError[]
  matched_path?: string
}

export interface ValidationSummary {
  host: string
  total_requests: number
  valid_count: number
  invalid_count: number
  errors_by_path: Record<string, number>
  errors_by_type: Record<string, number>
}

export async function validateHost(host: string, limit?: number): Promise<{ host: string; results: ValidationResult[]; total: number }> {
  const params = limit ? `?limit=${limit}` : ''
  return fetchJSON(`${API_BASE}/validation/${encodeURIComponent(host)}${params}`)
}

export async function getValidationSummary(host: string): Promise<ValidationSummary> {
  return fetchJSON(`${API_BASE}/validation/${encodeURIComponent(host)}/summary`)
}

export async function validateRequest(id: string): Promise<ValidationResult> {
  return fetchJSON(`${API_BASE}/validation/request/${encodeURIComponent(id)}`)
}

// Validation event from SSE
export interface ValidationEvent {
  request_id: string
  method: string
  path: string
  status_code: number
  status: 'valid' | 'invalid' | 'unmatched'
  matched_path?: string
  errors?: ValidationError[]
  timestamp: string
}

// Schema versioning and AI fix
export interface SchemaVersion {
  id: number
  host: string
  format: string
  version: number
  content: string
  change_reason: string
  validation_errors_fixed: string
  created_at: string
  is_active: boolean
}

export interface SchemaFixResponse {
  version: number
  fixed_schema: string
  changes: string[]
  reasoning: string
}

export interface PreviewResult {
  version: number
  results: {
    request_id: string
    method: string
    path: string
    valid: boolean
    matched_path?: string
    errors?: ValidationError[]
  }[]
  valid_count: number
  invalid_count: number
}

export async function listSchemaVersions(host: string, format: string = 'openapi'): Promise<{ versions: SchemaVersion[] }> {
  return fetchJSON(`${API_BASE}/schemas/${encodeURIComponent(host)}/versions?format=${format}`)
}

export async function requestSchemaFix(host: string, validationErrors: string[], sampleData?: string): Promise<SchemaFixResponse> {
  return fetchJSON(`${API_BASE}/schemas/${encodeURIComponent(host)}/fix`, {
    method: 'POST',
    body: JSON.stringify({
      validation_errors: validationErrors,
      sample_data: sampleData,
    }),
  })
}

export async function previewSchemaFix(host: string, version: number, requestIds: string[]): Promise<PreviewResult> {
  return fetchJSON(`${API_BASE}/schemas/${encodeURIComponent(host)}/preview-fix`, {
    method: 'POST',
    body: JSON.stringify({
      version,
      request_ids: requestIds,
    }),
  })
}

export async function activateSchemaVersion(host: string, version: number): Promise<{ message: string }> {
  return fetchJSON(`${API_BASE}/schemas/${encodeURIComponent(host)}/versions/${version}/activate`, {
    method: 'POST',
  })
}

// SSE for real-time validation monitoring
export function subscribeToValidation(
  host: string,
  onValidation: (event: ValidationEvent) => void,
  onWarning?: (message: string) => void
): () => void {
  const eventSource = new EventSource(`${API_BASE}/events/validation/${encodeURIComponent(host)}`)

  // Listen for named 'validation' events
  eventSource.addEventListener('validation', (event: MessageEvent) => {
    try {
      const data = JSON.parse(event.data)
      onValidation(data)
    } catch (e) {
      console.error('Failed to parse validation event:', e)
    }
  })

  eventSource.addEventListener('warning', (event: MessageEvent) => {
    try {
      const data = JSON.parse(event.data)
      if (onWarning) {
        onWarning(data.message)
      }
    } catch (e) {
      console.error('Failed to parse warning event:', e)
    }
  })

  eventSource.onerror = () => {
    console.error('Validation SSE connection error')
  }

  return () => eventSource.close()
}
