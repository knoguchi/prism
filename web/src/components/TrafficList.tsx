import type { CaptureListItem } from '../api/types'

interface TrafficListProps {
  captures: CaptureListItem[]
  selectedId: string | null
  onSelect: (capture: CaptureListItem) => void
  loading: boolean
}

export default function TrafficList({ captures, selectedId, onSelect, loading }: TrafficListProps) {
  const getMethodClass = (method: string) => {
    const classes: Record<string, string> = {
      GET: 'method-get',
      POST: 'method-post',
      PUT: 'method-put',
      PATCH: 'method-patch',
      DELETE: 'method-delete',
      HEAD: 'method-head',
      OPTIONS: 'method-options',
    }
    return classes[method] || ''
  }

  const getStatusClass = (status: number) => {
    if (status >= 200 && status < 300) return 'status-2xx'
    if (status >= 300 && status < 400) return 'status-3xx'
    if (status >= 400 && status < 500) return 'status-4xx'
    if (status >= 500) return 'status-5xx'
    return ''
  }

  const formatSize = (bytes: number) => {
    if (bytes === 0) return '-'
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  const formatLatency = (ms: number) => {
    if (ms === 0) return '-'
    if (ms < 1000) return `${ms} ms`
    return `${(ms / 1000).toFixed(2)} s`
  }

  const formatTime = (isoString: string) => {
    const date = new Date(isoString)
    return date.toLocaleTimeString('en-US', { hour12: false })
  }

  if (loading && captures.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center text-proxy-text-dim">
        Loading captures...
      </div>
    )
  }

  if (captures.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center text-proxy-text-dim">
        <p className="text-lg mb-2">No traffic captured yet</p>
        <p className="text-sm">Configure your browser to use the proxy at localhost:8080</p>
      </div>
    )
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Table header */}
      <div className="flex items-center px-3 py-2 bg-proxy-sidebar border-b border-proxy-border text-xs font-semibold text-proxy-text-dim">
        <div className="w-16">Method</div>
        <div className="w-14">Status</div>
        <div className="flex-1 min-w-0">URL</div>
        <div className="w-20 text-right">Size</div>
        <div className="w-20 text-right">Time</div>
        <div className="w-20 text-right">Captured</div>
      </div>

      {/* Table body */}
      <div className="flex-1 overflow-y-auto">
        {captures.map((capture) => (
          <div
            key={capture.id}
            role="button"
            tabIndex={0}
            className={`w-full flex items-center px-3 py-1.5 text-left hover:bg-proxy-border border-b border-proxy-border/50 cursor-pointer ${
              selectedId === capture.id ? 'bg-proxy-accent/30' : ''
            }`}
            onClick={() => onSelect(capture)}
            onKeyDown={(e) => e.key === 'Enter' && onSelect(capture)}
          >
            <div className={`w-16 font-mono text-xs ${getMethodClass(capture.method)}`}>
              {capture.method}
            </div>
            <div className={`w-14 font-mono text-xs ${getStatusClass(capture.status_code)}`}>
              {capture.status_code || '-'}
            </div>
            <div className="flex-1 min-w-0 text-xs truncate" title={capture.url}>
              <span className="text-proxy-text-dim">{capture.host}</span>
              <span>{capture.path}</span>
            </div>
            <div className="w-20 text-right text-xs text-proxy-text-dim">
              {formatSize(capture.response_size)}
            </div>
            <div className="w-20 text-right text-xs text-proxy-text-dim">
              {formatLatency(capture.latency_ms)}
            </div>
            <div className="w-20 text-right text-xs text-proxy-text-dim">
              {formatTime(capture.captured_at)}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
