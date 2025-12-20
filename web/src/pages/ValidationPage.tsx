import { useState, useEffect, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getEnabledHosts, listSchemas, subscribeToValidation, type ValidationEvent, type ValidationError } from '../api/client'
import { CheckCircle, XCircle, AlertTriangle, Globe, Radio, ChevronRight, ChevronDown, Trash2 } from 'lucide-react'

export default function ValidationPage() {
  const [selectedHost, setSelectedHost] = useState<string | null>(null)
  const [events, setEvents] = useState<ValidationEvent[]>([])
  const [expandedErrors, setExpandedErrors] = useState<Set<string>>(new Set())
  const [warning, setWarning] = useState<string | null>(null)
  const [isConnected, setIsConnected] = useState(false)

  // Fetch enabled hosts
  const { data: enabledHosts, isLoading: hostsLoading } = useQuery({
    queryKey: ['enabledHosts'],
    queryFn: getEnabledHosts,
  })

  // Fetch schemas to know which hosts have OpenAPI specs
  const { data: schemasData } = useQuery({
    queryKey: ['schemas'],
    queryFn: () => listSchemas(),
  })

  // Create a set of hosts that have OpenAPI schemas
  const hostsWithSchemas = new Set(
    schemasData?.schemas
      ?.filter(s => s.endpoints && s.endpoints.length > 0)
      .map(s => s.host) || []
  )

  // Subscribe to validation events when host is selected
  useEffect(() => {
    if (!selectedHost) {
      setIsConnected(false)
      return
    }

    setEvents([])
    setWarning(null)
    setIsConnected(true)

    const unsubscribe = subscribeToValidation(
      selectedHost,
      (event) => {
        setEvents(prev => {
          // Keep last 100 events
          const newEvents = [event, ...prev]
          if (newEvents.length > 100) {
            return newEvents.slice(0, 100)
          }
          return newEvents
        })
      },
      (message) => {
        setWarning(message)
      }
    )

    return () => {
      unsubscribe()
      setIsConnected(false)
    }
  }, [selectedHost])

  const toggleError = useCallback((id: string) => {
    setExpandedErrors(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }, [])

  const clearEvents = useCallback(() => {
    setEvents([])
  }, [])

  // Calculate stats
  const validCount = events.filter(e => e.status === 'valid').length
  const invalidCount = events.filter(e => e.status === 'invalid').length
  const unmatchedCount = events.filter(e => e.status === 'unmatched').length

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'valid':
        return 'text-emerald-400'
      case 'invalid':
        return 'text-red-400'
      case 'unmatched':
        return 'text-yellow-400'
      default:
        return 'text-proxy-text-dim'
    }
  }

  const getStatusBg = (status: string) => {
    switch (status) {
      case 'valid':
        return 'bg-emerald-500/10'
      case 'invalid':
        return 'bg-red-500/10'
      case 'unmatched':
        return 'bg-yellow-500/10'
      default:
        return ''
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'valid':
        return <CheckCircle size={16} className="text-emerald-400" />
      case 'invalid':
        return <XCircle size={16} className="text-red-400" />
      case 'unmatched':
        return <AlertTriangle size={16} className="text-yellow-400" />
      default:
        return null
    }
  }

  return (
    <div className="flex-1 flex overflow-hidden">
      {/* Left panel - Host list */}
      <aside className="w-72 border-r border-proxy-border bg-proxy-sidebar overflow-y-auto">
        <div className="p-3 border-b border-proxy-border">
          <h2 className="text-sm font-semibold text-proxy-text-dim uppercase tracking-wide">
            Live Validation
          </h2>
        </div>

        {hostsLoading ? (
          <div className="p-4 text-sm text-proxy-text-dim text-center">
            Loading hosts...
          </div>
        ) : !enabledHosts || enabledHosts.length === 0 ? (
          <div className="p-4 text-sm text-proxy-text-dim text-center">
            <p>No hosts enabled.</p>
            <p className="mt-2 text-xs">
              Enable hosts in the Traffic tab and run Analysis in the Schemas tab first.
            </p>
          </div>
        ) : (
          <div className="py-2">
            {enabledHosts.map((host) => {
              const hasSchema = hostsWithSchemas.has(host)
              return (
                <button
                  key={host}
                  className={`w-full flex items-center gap-2 px-3 py-2 hover:bg-proxy-border text-left ${
                    selectedHost === host ? 'bg-proxy-accent' : ''
                  }`}
                  onClick={() => setSelectedHost(host)}
                >
                  <Globe size={14} className={hasSchema ? 'text-proxy-accent' : 'text-proxy-text-dim'} />
                  <span className="flex-1 text-sm truncate">{host}</span>
                  {!hasSchema && (
                    <span title="No OpenAPI schema">
                      <AlertTriangle size={12} className="text-yellow-500" />
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        )}
      </aside>

      {/* Right panel - Live validation stream */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {!selectedHost ? (
          <div className="flex-1 flex items-center justify-center text-proxy-text-dim">
            <div className="text-center">
              <Radio size={48} className="mx-auto mb-4 opacity-50" />
              <p>Select a host to monitor live traffic</p>
              <p className="text-sm mt-2">Requests will be validated against the OpenAPI spec in real-time</p>
            </div>
          </div>
        ) : (
          <>
            {/* Header with stats */}
            <div className="p-4 border-b border-proxy-border bg-proxy-sidebar">
              <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-3">
                  <h2 className="text-lg font-medium">Live Monitoring: {selectedHost}</h2>
                  {isConnected && (
                    <span className="flex items-center gap-1 text-xs text-emerald-400">
                      <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
                      Connected
                    </span>
                  )}
                </div>
                <button
                  onClick={clearEvents}
                  className="flex items-center gap-1 px-2 py-1 text-xs text-proxy-text-dim hover:text-proxy-text hover:bg-proxy-border rounded"
                >
                  <Trash2 size={12} />
                  Clear
                </button>
              </div>

              {warning && (
                <div className="mb-3 p-2 bg-yellow-500/10 border border-yellow-500/30 rounded text-sm text-yellow-400">
                  <AlertTriangle size={14} className="inline mr-2" />
                  {warning}
                </div>
              )}

              <div className="grid grid-cols-4 gap-4">
                <div className="p-3 bg-proxy-bg rounded-lg border border-proxy-border">
                  <div className="text-2xl font-bold">{events.length}</div>
                  <div className="text-xs text-proxy-text-dim">Captured</div>
                </div>
                <div className="p-3 bg-proxy-bg rounded-lg border border-emerald-500/30">
                  <div className="text-2xl font-bold text-emerald-400">{validCount}</div>
                  <div className="text-xs text-proxy-text-dim">Valid</div>
                </div>
                <div className="p-3 bg-proxy-bg rounded-lg border border-red-500/30">
                  <div className="text-2xl font-bold text-red-400">{invalidCount}</div>
                  <div className="text-xs text-proxy-text-dim">Invalid</div>
                </div>
                <div className="p-3 bg-proxy-bg rounded-lg border border-yellow-500/30">
                  <div className="text-2xl font-bold text-yellow-400">{unmatchedCount}</div>
                  <div className="text-xs text-proxy-text-dim">Unmatched</div>
                </div>
              </div>
            </div>

            {/* Event stream */}
            <div className="flex-1 overflow-auto">
              {events.length === 0 ? (
                <div className="p-8 text-center text-proxy-text-dim">
                  <Radio size={24} className="mx-auto mb-2 animate-pulse" />
                  <p>Waiting for traffic...</p>
                  <p className="text-xs mt-1">Requests to {selectedHost} will appear here</p>
                </div>
              ) : (
                <table className="w-full text-sm">
                  <thead className="bg-proxy-sidebar sticky top-0">
                    <tr className="text-left text-proxy-text-dim">
                      <th className="px-4 py-2 w-10">Status</th>
                      <th className="px-4 py-2 w-20">Method</th>
                      <th className="px-4 py-2">Path</th>
                      <th className="px-4 py-2 w-16">Code</th>
                      <th className="px-4 py-2 w-48">Matched Route</th>
                      <th className="px-4 py-2 w-24">Errors</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-proxy-border">
                    {events.map((event) => (
                      <>
                        <tr
                          key={event.request_id}
                          className={`hover:bg-proxy-border cursor-pointer ${getStatusBg(event.status)}`}
                          onClick={() => event.errors && event.errors.length > 0 && toggleError(event.request_id)}
                        >
                          <td className="px-4 py-2">
                            {getStatusIcon(event.status)}
                          </td>
                          <td className="px-4 py-2">
                            <span className={`font-mono ${
                              event.method === 'GET' ? 'text-emerald-400' :
                              event.method === 'POST' ? 'text-blue-400' :
                              event.method === 'PUT' ? 'text-yellow-400' :
                              event.method === 'DELETE' ? 'text-red-400' :
                              'text-proxy-text-dim'
                            }`}>
                              {event.method}
                            </span>
                          </td>
                          <td className="px-4 py-2 font-mono text-proxy-text-dim truncate max-w-md">
                            {event.path}
                          </td>
                          <td className="px-4 py-2">
                            <span className={`font-mono ${
                              event.status_code >= 200 && event.status_code < 300 ? 'text-emerald-400' :
                              event.status_code >= 400 && event.status_code < 500 ? 'text-yellow-400' :
                              event.status_code >= 500 ? 'text-red-400' :
                              'text-proxy-text-dim'
                            }`}>
                              {event.status_code}
                            </span>
                          </td>
                          <td className="px-4 py-2 font-mono text-xs text-proxy-text-dim">
                            {event.matched_path || '-'}
                          </td>
                          <td className="px-4 py-2">
                            {event.errors && event.errors.length > 0 && (
                              <span className={`flex items-center gap-1 ${getStatusColor(event.status)}`}>
                                {expandedErrors.has(event.request_id) ? (
                                  <ChevronDown size={14} />
                                ) : (
                                  <ChevronRight size={14} />
                                )}
                                {event.errors.length} error{event.errors.length > 1 ? 's' : ''}
                              </span>
                            )}
                          </td>
                        </tr>
                        {expandedErrors.has(event.request_id) && event.errors && (
                          <tr key={`${event.request_id}-errors`}>
                            <td colSpan={6} className={`px-4 py-3 ${
                              event.status === 'invalid' ? 'bg-red-500/10' : 'bg-yellow-500/10'
                            }`}>
                              <div className="space-y-2">
                                {event.errors.map((err: ValidationError, idx: number) => (
                                  <div key={idx} className="flex items-start gap-3 text-xs">
                                    <span className={`px-2 py-0.5 rounded ${
                                      event.status === 'invalid'
                                        ? 'bg-red-500/20 text-red-400'
                                        : 'bg-yellow-500/20 text-yellow-400'
                                    }`}>
                                      {err.location}
                                    </span>
                                    <span className="text-proxy-text-dim">{err.message}</span>
                                  </div>
                                ))}
                              </div>
                            </td>
                          </tr>
                        )}
                      </>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}