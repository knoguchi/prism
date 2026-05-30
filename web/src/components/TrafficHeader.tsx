import { Search, Trash2, Download } from 'lucide-react'

interface TrafficHeaderProps {
  filters: {
    host: string
    method: string
    status: string
    search: string
  }
  onFiltersChange: (filters: TrafficHeaderProps['filters']) => void
  totalCaptures: number
  allCaptures?: number
  onExport?: () => void
  onClear?: () => void
}

export default function TrafficHeader({
  filters,
  onFiltersChange,
  totalCaptures,
  allCaptures,
  onExport,
  onClear,
}: TrafficHeaderProps) {
  return (
    <header className="h-10 flex items-center gap-4 px-4 border-b border-proxy-border bg-proxy-bg">
      {/* Search */}
      <div className="flex-1 max-w-md relative">
        <Search size={14} className="absolute left-2 top-1/2 -translate-y-1/2 text-proxy-text-dim" />
        <input
          type="text"
          placeholder="Search traffic..."
          className="w-full pl-8 pr-3 py-1.5 bg-proxy-sidebar border border-proxy-border rounded text-sm focus:outline-none focus:border-proxy-accent"
          value={filters.search}
          onChange={(e) => onFiltersChange({ ...filters, search: e.target.value })}
        />
      </div>

      {/* Method filter */}
      <select
        className="px-2 py-1.5 bg-proxy-sidebar border border-proxy-border rounded text-sm focus:outline-none"
        value={filters.method}
        onChange={(e) => onFiltersChange({ ...filters, method: e.target.value })}
      >
        <option value="">All Methods</option>
        <option value="GET">GET</option>
        <option value="POST">POST</option>
        <option value="PUT">PUT</option>
        <option value="PATCH">PATCH</option>
        <option value="DELETE">DELETE</option>
      </select>

      {/* Status filter */}
      <select
        className="px-2 py-1.5 bg-proxy-sidebar border border-proxy-border rounded text-sm focus:outline-none"
        value={filters.status}
        onChange={(e) => onFiltersChange({ ...filters, status: e.target.value })}
      >
        <option value="">All Status</option>
        <option value="2xx">2xx Success</option>
        <option value="3xx">3xx Redirect</option>
        <option value="4xx">4xx Client Error</option>
        <option value="5xx">5xx Server Error</option>
      </select>

      {/* Stats */}
      <div className="text-sm text-proxy-text-dim">
        {totalCaptures.toLocaleString()}
        {allCaptures !== undefined && allCaptures !== totalCaptures && (
          <span className="text-proxy-text-dim"> / {allCaptures.toLocaleString()}</span>
        )}
        {' '}captures
      </div>

      {/* Actions */}
      <div className="flex items-center gap-1">
        <button
          className="p-1.5 hover:bg-proxy-border rounded"
          title="Export captures as JSON"
          onClick={onExport}
        >
          <Download size={16} />
        </button>
        <button
          className="p-1.5 hover:bg-proxy-border rounded text-proxy-error"
          title="Clear all captures"
          onClick={onClear}
        >
          <Trash2 size={16} />
        </button>
      </div>
    </header>
  )
}
