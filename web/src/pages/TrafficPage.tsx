import { useState, useMemo, useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Group, Panel, Separator } from 'react-resizable-panels'
import { listCaptures, getCapture, getEnabledHosts, addEnabledHost, removeEnabledHost, clearCaptures } from '../api/client'
import type { CaptureListItem } from '../api/types'
import TrafficList from '../components/TrafficList'
import RequestDetail from '../components/RequestDetail'
import Sidebar from '../components/Sidebar'
import TrafficHeader from '../components/TrafficHeader'

export default function TrafficPage() {
  const [selectedId, setSelectedId] = useState<string | null>(null)
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

  // Filter to enabled hosts for the traffic list (middle pane), then apply search
  const filteredCaptures = useMemo(() => {
    let results = enabledHosts.size === 0
      ? allCaptures
      : allCaptures.filter(c => enabledHosts.has(c.host))

    // Client-side search filter
    if (filters.search) {
      const q = filters.search.toLowerCase()
      results = results.filter(c =>
        c.url.toLowerCase().includes(q) ||
        c.host.toLowerCase().includes(q) ||
        c.path.toLowerCase().includes(q) ||
        c.method.toLowerCase().includes(q)
      )
    }

    return results
  }, [allCaptures, enabledHosts, filters.search])

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

  const handleExport = useCallback(() => {
    const blob = new Blob([JSON.stringify(filteredCaptures, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `prism-captures-${new Date().toISOString().slice(0, 10)}.json`
    a.click()
    URL.revokeObjectURL(url)
  }, [filteredCaptures])

  const handleClear = useCallback(async () => {
    if (!window.confirm('Clear all captured traffic? This cannot be undone.')) return
    try {
      await clearCaptures()
      setSelectedId(null)
      queryClient.invalidateQueries({ queryKey: ['captures'] })
    } catch (e) {
      console.error('Failed to clear captures:', e)
    }
  }, [queryClient])

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Traffic-specific header with filters */}
      <TrafficHeader
        filters={filters}
        onFiltersChange={setFilters}
        totalCaptures={filteredCaptures.length}
        allCaptures={capturesData?.pagination?.total || 0}
        onExport={handleExport}
        onClear={handleClear}
      />

      {/* Main content */}
      <Group orientation="horizontal" className="flex-1 h-full w-full">
        {/* Sidebar - shows ALL hosts from captures so users can toggle them */}
        <Panel id="sidebar" defaultSize="20%" minSize="150px" maxSize="50%">
          <Sidebar
            captures={allCaptures}
            selectedId={selectedId}
            onSelect={handleSelect}
            enabledHosts={enabledHosts}
            onToggleHost={handleToggleHost}
          />
        </Panel>
        <Separator id="sidebar-separator" className="resize-handle" />

        {/* Traffic list */}
        <Panel id="traffic-list" defaultSize="50%" minSize="200px">
          <div className="h-full w-full flex flex-col">
            <TrafficList
              captures={filteredCaptures}
              selectedId={selectedId}
              onSelect={handleSelect}
              loading={listLoading}
            />
          </div>
        </Panel>
        <Separator id="detail-separator" className="resize-handle" />

        {/* Detail panel */}
        <Panel id="detail" defaultSize="30%" minSize="200px">
          <div className="h-full w-full flex flex-col overflow-hidden">
            <RequestDetail
              capture={selectedCapture || null}
              loading={detailLoading && !!selectedId}
            />
          </div>
        </Panel>
      </Group>
    </div>
  )
}
