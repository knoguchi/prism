import { useState, useMemo, useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { listCaptures, getCapture, getEnabledHosts, addEnabledHost, removeEnabledHost } from '../api/client'
import type { CaptureListItem } from '../api/types'
import TrafficList from '../components/TrafficList'
import RequestDetail from '../components/RequestDetail'
import Sidebar from '../components/Sidebar'
import TrafficHeader from '../components/TrafficHeader'

type ViewMode = 'list' | 'tree'

export default function TrafficPage() {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [viewMode, setViewMode] = useState<ViewMode>('list')
  const [filters, setFilters] = useState({
    host: '',
    method: '',
    status: '',
    search: '',
  })
  const queryClient = useQueryClient()

  // Fetch enabled hosts from API
  const { data: enabledHostsData } = useQuery({
    queryKey: ['enabledHosts'],
    queryFn: getEnabledHosts,
    staleTime: 30000,
    refetchOnMount: true,
    refetchOnWindowFocus: true,
    retry: 3,
  })
  const enabledHosts = useMemo(() => new Set(enabledHostsData || []), [enabledHostsData])
  const enabledHostsArray = useMemo(() => Array.from(enabledHosts), [enabledHosts])

  // Single query that combines:
  // 1. Enabled hosts: ACCUMULATE all traffic (max 20 per path, no overall limit)
  // 2. Non-enabled hosts: recent ephemeral traffic (limited to 200, just to show hosts in sidebar)
  const { data: capturesData, isLoading: listLoading } = useQuery({
    queryKey: ['captures', enabledHostsArray, filters],
    queryFn: () => listCaptures({
      hosts: enabledHostsArray.length > 0 ? enabledHostsArray : undefined,
      method: filters.method || undefined,
      status: filters.status || undefined,
      limit_per_path: 20,      // For enabled hosts: max 20 per path (accumulates)
      ephemeral_limit: 200,    // For non-enabled hosts: recent traffic limit
    }),
    refetchInterval: 5000,
  })

  // Fetch selected capture detail
  const { data: selectedCapture, isLoading: detailLoading } = useQuery({
    queryKey: ['capture', selectedId],
    queryFn: () => getCapture(selectedId!),
    enabled: !!selectedId,
  })

  // All captures from single call - used by both sidebar and traffic list
  const allCaptures = capturesData?.data || []

  // Filter to enabled hosts for the traffic list (middle pane)
  const filteredCaptures = useMemo(() => {
    if (enabledHosts.size === 0) return allCaptures
    return allCaptures.filter(c => enabledHosts.has(c.host))
  }, [allCaptures, enabledHosts])

  // Toggle a host's enabled state via API
  const handleToggleHost = useCallback(async (host: string) => {
    try {
      if (enabledHosts.has(host)) {
        await removeEnabledHost(host)
      } else {
        await addEnabledHost(host)
      }
      queryClient.invalidateQueries({ queryKey: ['enabledHosts'] })
      queryClient.invalidateQueries({ queryKey: ['captures'] })
    } catch (e) {
      console.error('Failed to toggle host:', e)
    }
  }, [enabledHosts, queryClient])

  const handleSelect = (capture: CaptureListItem) => {
    setSelectedId(capture.id)
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Traffic-specific header with filters */}
      <TrafficHeader
        viewMode={viewMode}
        onViewModeChange={setViewMode}
        filters={filters}
        onFiltersChange={setFilters}
        totalCaptures={filteredCaptures.length}
        allCaptures={capturesData?.pagination?.total || 0}
      />

      {/* Main content */}
      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar - shows ALL hosts from captures so users can toggle them */}
        <Sidebar
          captures={allCaptures}
          selectedId={selectedId}
          onSelect={handleSelect}
          enabledHosts={enabledHosts}
          onToggleHost={handleToggleHost}
        />

        {/* Traffic list */}
        <div className="flex-1 flex flex-col border-r border-proxy-border">
          <TrafficList
            captures={filteredCaptures}
            selectedId={selectedId}
            onSelect={handleSelect}
            loading={listLoading}
          />
        </div>

        {/* Detail panel */}
        <div className="w-[500px] flex flex-col overflow-hidden">
          <RequestDetail
            capture={selectedCapture || null}
            loading={detailLoading && !!selectedId}
          />
        </div>
      </div>
    </div>
  )
}
