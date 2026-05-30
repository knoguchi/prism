import { useState, useEffect, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getEnabledHosts, listSchemas, subscribeToValidation, requestSchemaFix, previewSchemaFix, activateSchemaVersion, type ValidationEvent, type ValidationError, type SchemaFixResponse, type PreviewResult } from '../api/client'
import { CheckCircle, XCircle, AlertTriangle, Globe, Radio, ChevronRight, ChevronDown, Trash2, Wand2, X, Loader2, Check } from 'lucide-react'

// AI Fix Modal Component
interface AIFixModalProps {
  host: string
  errors: ValidationError[]
  requestIds: string[]
  onClose: () => void
}

function AIFixModal({ host, errors, requestIds, onClose }: AIFixModalProps) {
  const queryClient = useQueryClient()
  const [step, setStep] = useState<'requesting' | 'preview' | 'done'>('requesting')
  const [fixResponse, setFixResponse] = useState<SchemaFixResponse | null>(null)
  const [previewResult, setPreviewResult] = useState<PreviewResult | null>(null)
  const [error, setError] = useState<string | null>(null)

  // Request AI fix
  const fixMutation = useMutation({
    mutationFn: () => {
      const errorMessages = errors.map(e => `${e.location}: ${e.message}`)
      return requestSchemaFix(host, errorMessages)
    },
    onSuccess: (data) => {
      setFixResponse(data)
      // Auto-preview with the failing requests
      previewMutation.mutate({ version: data.version, requestIds: requestIds.slice(0, 10) })
    },
    onError: (err: Error) => {
      setError(err.message)
    }
  })

  // Preview fix
  const previewMutation = useMutation({
    mutationFn: ({ version, requestIds }: { version: number; requestIds: string[] }) =>
      previewSchemaFix(host, version, requestIds),
    onSuccess: (data) => {
      setPreviewResult(data)
      setStep('preview')
    },
    onError: (err: Error) => {
      setError(err.message)
    }
  })

  // Activate fix
  const activateMutation = useMutation({
    mutationFn: (version: number) => activateSchemaVersion(host, version),
    onSuccess: () => {
      setStep('done')
      // Invalidate schemas to refresh
      queryClient.invalidateQueries({ queryKey: ['schemas'] })
    },
    onError: (err: Error) => {
      setError(err.message)
    }
  })

  // Auto-start the fix request
  useEffect(() => {
    fixMutation.mutate()
  }, [])

  const isLoading = fixMutation.isPending || previewMutation.isPending || activateMutation.isPending

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-50">
      <div className="bg-[#1a1a2e] border border-proxy-border rounded-lg shadow-xl max-w-2xl w-full max-h-[80vh] overflow-hidden flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-proxy-border">
          <h3 className="text-lg font-medium flex items-center gap-2">
            <Wand2 size={18} className="text-purple-400" />
            AI Schema Fix
          </h3>
          <button onClick={onClose} className="text-proxy-text-dim hover:text-proxy-text">
            <X size={18} />
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-auto p-4">
          {error && (
            <div className="mb-4 p-3 bg-red-500/10 border border-red-500/30 rounded text-red-400 text-sm">
              {error}
            </div>
          )}

          {step === 'requesting' && (
            <div className="text-center py-8">
              <Loader2 size={32} className="mx-auto mb-4 animate-spin text-purple-400" />
              <p>Analyzing validation errors and generating fix...</p>
              <p className="text-sm text-proxy-text-dim mt-2">
                {errors.length} error{errors.length > 1 ? 's' : ''} to fix
              </p>
            </div>
          )}

          {step === 'preview' && fixResponse && previewResult && (
            <div className="space-y-4">
              {/* Changes */}
              <div>
                <h4 className="font-medium mb-2">Proposed Changes</h4>
                <ul className="space-y-1">
                  {fixResponse.changes.map((change, i) => (
                    <li key={i} className="flex items-start gap-2 text-sm">
                      <span className="text-emerald-400 mt-0.5">+</span>
                      <span>{change}</span>
                    </li>
                  ))}
                </ul>
              </div>

              {/* Reasoning */}
              <div>
                <h4 className="font-medium mb-2">Reasoning</h4>
                <p className="text-sm text-proxy-text-dim">{fixResponse.reasoning}</p>
              </div>

              {/* Preview results */}
              <div>
                <h4 className="font-medium mb-2">Validation Preview</h4>
                <div className="grid grid-cols-2 gap-4 mb-3">
                  <div className="p-3 bg-emerald-500/10 rounded border border-emerald-500/30 text-center">
                    <div className="text-xl font-bold text-emerald-400">{previewResult.valid_count}</div>
                    <div className="text-xs text-proxy-text-dim">Now Valid</div>
                  </div>
                  <div className="p-3 bg-red-500/10 rounded border border-red-500/30 text-center">
                    <div className="text-xl font-bold text-red-400">{previewResult.invalid_count}</div>
                    <div className="text-xs text-proxy-text-dim">Still Invalid</div>
                  </div>
                </div>

                {previewResult.invalid_count > 0 && (
                  <div className="text-sm text-yellow-400 mb-3">
                    <AlertTriangle size={14} className="inline mr-1" />
                    Some requests are still invalid. You may need additional fixes.
                  </div>
                )}

                {/* Individual results */}
                <div className="max-h-48 overflow-auto border border-proxy-border rounded">
                  <table className="w-full text-xs">
                    <thead className="bg-proxy-sidebar sticky top-0">
                      <tr>
                        <th className="px-2 py-1 text-left">Status</th>
                        <th className="px-2 py-1 text-left">Method</th>
                        <th className="px-2 py-1 text-left">Path</th>
                        <th className="px-2 py-1 text-left">Error</th>
                      </tr>
                    </thead>
                    <tbody>
                      {previewResult.results.map((result, i) => (
                        <tr key={i} className={result.valid ? 'bg-emerald-500/5' : 'bg-red-500/5'}>
                          <td className="px-2 py-1">
                            {result.valid ? (
                              <CheckCircle size={12} className="text-emerald-400" />
                            ) : (
                              <XCircle size={12} className="text-red-400" />
                            )}
                          </td>
                          <td className="px-2 py-1 font-mono">{result.method}</td>
                          <td className="px-2 py-1 font-mono truncate max-w-32">{result.path}</td>
                          <td className="px-2 py-1 text-red-400 truncate max-w-64">
                            {result.errors && result.errors.length > 0
                              ? result.errors.map(e => e.message).join('; ')
                              : ''}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          )}

          {step === 'done' && (
            <div className="text-center py-8">
              <Check size={32} className="mx-auto mb-4 text-emerald-400" />
              <p className="text-lg font-medium">Schema Updated Successfully</p>
              <p className="text-sm text-proxy-text-dim mt-2">
                The fix has been applied. New requests will be validated against the updated schema.
              </p>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 px-4 py-3 border-t border-proxy-border">
          {step === 'preview' && fixResponse && (
            <>
              <button
                onClick={onClose}
                className="px-4 py-2 text-sm text-proxy-text-dim hover:text-proxy-text"
                disabled={isLoading}
              >
                Reject
              </button>
              <button
                onClick={() => activateMutation.mutate(fixResponse.version)}
                className="px-4 py-2 text-sm bg-emerald-600 hover:bg-emerald-500 text-white rounded flex items-center gap-2"
                disabled={isLoading}
              >
                {activateMutation.isPending ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  <Check size={14} />
                )}
                Accept & Apply
              </button>
            </>
          )}
          {step === 'done' && (
            <button
              onClick={onClose}
              className="px-4 py-2 text-sm bg-proxy-accent hover:bg-proxy-accent/80 text-white rounded"
            >
              Close
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

export default function ValidationPage() {
  const [selectedHost, setSelectedHost] = useState<string | null>(null)
  const [events, setEvents] = useState<ValidationEvent[]>([])
  const [expandedErrors, setExpandedErrors] = useState<Set<string>>(new Set())
  const [warning, setWarning] = useState<string | null>(null)
  const [isConnected, setIsConnected] = useState(false)
  const [methodFilter, setMethodFilter] = useState<string>('')
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [aiFixModal, setAiFixModal] = useState<{ errors: ValidationError[]; requestIds: string[] } | null>(null)

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

  const openAiFix = useCallback((errors: ValidationError[], requestIds: string[]) => {
    setAiFixModal({ errors, requestIds })
  }, [])

  // Filter events by method and status
  const filteredEvents = events.filter(e => {
    if (methodFilter && e.method !== methodFilter) return false
    if (statusFilter && e.status !== statusFilter) return false
    return true
  })

  // Calculate stats (from filtered events)
  const validCount = filteredEvents.filter(e => e.status === 'valid').length
  const invalidCount = filteredEvents.filter(e => e.status === 'invalid').length
  const unmatchedCount = filteredEvents.filter(e => e.status === 'unmatched').length

  // Get unique methods for filter dropdown
  const availableMethods = [...new Set(events.map(e => e.method))].sort()

  // Group events by method + matched_path + status + single error
  // Each individual error becomes its own row, then identical errors are grouped
  type GroupedEvent = ValidationEvent & { count: number; request_ids: string[]; singleError?: ValidationError }
  const groupedEvents = filteredEvents.reduce<GroupedEvent[]>((acc, event) => {
    // If event has multiple errors, create a separate row for each error
    const errors = event.errors && event.errors.length > 0 ? event.errors : [undefined]

    for (const singleError of errors) {
      // Create a signature for grouping - use single error only
      const errorSig = singleError ? `${singleError.location}:${singleError.message}` : ''
      const signature = `${event.method}|${event.matched_path || event.path}|${event.status}|${errorSig}`

      const existing = acc.find(g => {
        const gErrorSig = g.singleError ? `${g.singleError.location}:${g.singleError.message}` : ''
        const gSig = `${g.method}|${g.matched_path || g.path}|${g.status}|${gErrorSig}`
        return gSig === signature
      })

      if (existing) {
        existing.count++
        if (!existing.request_ids.includes(event.request_id)) {
          existing.request_ids.push(event.request_id)
        }
      } else {
        acc.push({
          ...event,
          count: 1,
          request_ids: [event.request_id],
          singleError,
          // Override errors array to only contain this single error (for display)
          errors: singleError ? [singleError] : undefined
        })
      }
    }
    return acc
  }, [])

  // Sort by path (matched_path or path)
  groupedEvents.sort((a, b) => {
    const pathA = a.matched_path || a.path
    const pathB = b.matched_path || b.path
    return pathA.localeCompare(pathB)
  })

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
                <div className="flex items-center gap-2">
                  <select
                    value={statusFilter}
                    onChange={(e) => setStatusFilter(e.target.value)}
                    className="px-2 py-1 text-xs bg-proxy-bg border border-proxy-border rounded text-proxy-text"
                  >
                    <option value="">All Status</option>
                    <option value="invalid">Errors Only</option>
                    <option value="valid">Valid Only</option>
                    <option value="unmatched">Unmatched Only</option>
                  </select>
                  <select
                    value={methodFilter}
                    onChange={(e) => setMethodFilter(e.target.value)}
                    className="px-2 py-1 text-xs bg-proxy-bg border border-proxy-border rounded text-proxy-text"
                  >
                    <option value="">All Methods</option>
                    {availableMethods.map(method => (
                      <option key={method} value={method}>{method}</option>
                    ))}
                  </select>
                  <button
                    onClick={clearEvents}
                    className="flex items-center gap-1 px-2 py-1 text-xs text-proxy-text-dim hover:text-proxy-text hover:bg-proxy-border rounded"
                  >
                    <Trash2 size={12} />
                    Clear
                  </button>
                </div>
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
                      <th className="px-4 py-2 w-16">Count</th>
                      <th className="px-4 py-2 w-20">Method</th>
                      <th className="px-4 py-2">Matched Route</th>
                      <th className="px-4 py-2 w-16">Code</th>
                      <th className="px-4 py-2">Error</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-proxy-border">
                    {groupedEvents.map((event) => (
                      <>
                        <tr
                          key={event.request_ids[0] + (event.singleError?.message || '')}
                          className={`hover:bg-proxy-border cursor-pointer ${getStatusBg(event.status)}`}
                          onClick={() => event.singleError && toggleError(event.request_ids[0] + (event.singleError?.message || ''))}
                        >
                          <td className="px-4 py-2">
                            {getStatusIcon(event.status)}
                          </td>
                          <td className="px-4 py-2">
                            <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                              event.count > 1 ? 'bg-proxy-accent text-white' : 'text-proxy-text-dim'
                            }`}>
                              {event.count}×
                            </span>
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
                            {event.matched_path || event.path}
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
                          <td className="px-4 py-2">
                            {event.singleError && (
                              <span className={`flex items-center gap-1 ${getStatusColor(event.status)}`}>
                                {expandedErrors.has(event.request_ids[0] + (event.singleError?.message || '')) ? (
                                  <ChevronDown size={14} className="flex-shrink-0" />
                                ) : (
                                  <ChevronRight size={14} className="flex-shrink-0" />
                                )}
                                <span className="truncate" title={`${event.singleError.location}: ${event.singleError.message}`}>
                                  {event.singleError.location}: {event.singleError.message.length > 40
                                    ? event.singleError.message.substring(0, 40) + '...'
                                    : event.singleError.message}
                                </span>
                              </span>
                            )}
                          </td>
                        </tr>
                        {expandedErrors.has(event.request_ids[0] + (event.singleError?.message || '')) && event.singleError && (
                          <tr key={`${event.request_ids[0]}-${event.singleError?.message || ''}-errors`}>
                            <td colSpan={6} className={`px-4 py-3 ${
                              event.status === 'invalid' ? 'bg-red-500/10' : 'bg-yellow-500/10'
                            }`}>
                              <div className="flex items-start justify-between">
                                <div className="space-y-2 flex-1">
                                  <div className="flex items-start gap-3 text-xs">
                                    <span className={`px-2 py-0.5 rounded ${
                                      event.status === 'invalid'
                                        ? 'bg-red-500/20 text-red-400'
                                        : 'bg-yellow-500/20 text-yellow-400'
                                    }`}>
                                      {event.singleError.location}
                                    </span>
                                    <span className="text-proxy-text-dim break-all">{event.singleError.message}</span>
                                  </div>
                                </div>
                                {event.status === 'invalid' && event.singleError && (
                                  <button
                                    onClick={(e) => {
                                      e.stopPropagation()
                                      openAiFix([event.singleError!], event.request_ids)
                                    }}
                                    className="ml-4 flex items-center gap-1 px-3 py-1.5 text-xs bg-purple-600 hover:bg-purple-500 text-white rounded"
                                  >
                                    <Wand2 size={12} />
                                    AI Fix
                                  </button>
                                )}
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

      {/* AI Fix Modal */}
      {aiFixModal && selectedHost && (
        <AIFixModal
          host={selectedHost}
          errors={aiFixModal.errors}
          requestIds={aiFixModal.requestIds}
          onClose={() => setAiFixModal(null)}
        />
      )}
    </div>
  )
}
