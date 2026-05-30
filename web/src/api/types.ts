// Captured request/response types

export interface CaptureListItem {
  id: string
  method: string
  url: string
  host: string
  path: string
  status_code: number
  content_type: string
  request_size: number
  response_size: number
  latency_ms: number
  captured_at: string
}

export interface CaptureDetail {
  id: string
  request: RequestDetail
  response: ResponseDetail | null
}

export interface RequestDetail {
  method: string
  url: string
  host: string
  path: string
  query_string: string
  headers: Record<string, string[]>
  body: string | null
  content_type: string
  body_size: number
  protocol: string
  is_https: boolean
  remote_addr: string
  captured_at: string
}

export interface ResponseDetail {
  status_code: number
  status_text: string
  headers: Record<string, string[]>
  body: string | null
  content_type: string
  body_size: number
  latency_ms: number
  captured_at: string
}

export interface TimingInfo {
  started_at: string
  ttfb_ms: number
  total_ms: number
}

export interface PaginatedResponse<T> {
  data: T[]
  pagination: {
    page: number
    limit: number
    total: number
    total_pages: number
  }
}

export interface SearchResult {
  query: string
  results: CaptureListItem[]
  total: number
}

export interface SchemaInfo {
  host: string
  method: string
  path_pattern: string
  format: string
  sample_count: number
  updated_at: string
}

export interface HostSchemas {
  host: string
  endpoints: EndpointInfo[]
}

export interface EndpointInfo {
  method: string
  path_pattern: string
  sample_count: number
  formats: string[]
}

export interface Stats {
  total_requests: number
  total_responses: number
  total_websocket_messages: number
  storage_size_bytes: number
  by_method: Record<string, number>
  by_status: Record<string, number>
  top_hosts: { host: string; count: number }[]
}