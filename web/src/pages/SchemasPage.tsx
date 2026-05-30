import { useState, useEffect, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listSchemas, getEnabledHosts, startAnalysis, getHostDiagram, getSchemaByFormat, getInferenceStatus } from '../api/client'
import { ChevronRight, ChevronDown, Globe, FileCode, Copy, Download, GitBranch, Workflow, Sparkles, AlertCircle, Loader2 } from 'lucide-react'
import MermaidDiagram from '../components/MermaidDiagram'

type SchemaFormat = 'openapi' | 'protobuf' | 'avro' | 'sql' | 'typescript' | 'go' | 'json-schema'
type ViewMode = 'schemas' | 'flows'
type HostStatus = 'pending' | 'running' | 'completed'

export default function SchemasPage() {
  const [selectedHost, setSelectedHost] = useState<string | null>(null)
  const [selectedFormat, setSelectedFormat] = useState<SchemaFormat>('openapi')
  const [expandedHosts, setExpandedHosts] = useState<Set<string>>(new Set())
  const [viewMode, setViewMode] = useState<ViewMode>('schemas')
  const [hostStatuses, setHostStatuses] = useState<Map<string, HostStatus>>(new Map())
  const queryClient = useQueryClient()

  // Fetch enabled hosts (from Traffic tab selections)
  const { data: enabledHosts, isLoading: hostsLoading } = useQuery({
    queryKey: ['enabledHosts'],
    queryFn: getEnabledHosts,
  })

  // Fetch inferred schemas
  const { data: schemasData, isLoading: schemasLoading } = useQuery({
    queryKey: ['schemas'],
    queryFn: () => listSchemas(),
  })

  // Poll status for all enabled hosts
  useEffect(() => {
    if (!enabledHosts || enabledHosts.length === 0) return

    const pollStatus = async () => {
      const newStatuses = new Map<string, HostStatus>()
      for (const host of enabledHosts) {
        try {
          const status = await getInferenceStatus(host)
          newStatuses.set(host, status.status as HostStatus)
        } catch {
          newStatuses.set(host, 'pending')
        }
      }
      setHostStatuses(newStatuses)

      // If any host is running, refresh schemas when it completes
      const hasRunning = Array.from(newStatuses.values()).some(s => s === 'running')
      if (hasRunning) {
        // Continue polling
      } else {
        // Refresh schemas list
        queryClient.invalidateQueries({ queryKey: ['schemas'] })
      }
    }

    pollStatus()
    const interval = setInterval(pollStatus, 3000) // Poll every 3 seconds
    return () => clearInterval(interval)
  }, [enabledHosts, queryClient])

  // Fetch diagram for selected host
  const { data: diagramData } = useQuery({
    queryKey: ['diagram', selectedHost],
    queryFn: () => selectedHost ? getHostDiagram(selectedHost) : null,
    enabled: !!selectedHost && viewMode === 'flows',
  })

  // Fetch schema content for selected host and format
  const { data: schemaContent, isLoading: schemaLoading } = useQuery({
    queryKey: ['schemaContent', selectedHost, selectedFormat],
    queryFn: () => selectedHost ? getSchemaByFormat(selectedHost, selectedFormat) : null,
    enabled: !!selectedHost && viewMode === 'schemas',
  })

  // Mutation for starting analysis
  const analyzeMutation = useMutation({
    mutationFn: startAnalysis,
    onMutate: (host) => {
      // Immediately set status to running
      setHostStatuses(prev => new Map(prev).set(host, 'running'))
    },
    onSuccess: (_, host) => {
      // Refresh diagrams when done
      queryClient.invalidateQueries({ queryKey: ['diagram', host] })
      queryClient.invalidateQueries({ queryKey: ['schemaContent', host] })
    },
  })

  const handleAnalyze = (host: string) => {
    const status = hostStatuses.get(host)
    if (status === 'running') return // Don't allow if already running
    analyzeMutation.mutate(host)
  }

  const isHostRunning = (host: string) => hostStatuses.get(host) === 'running'

  const isLoading = hostsLoading || schemasLoading
  const schemas = schemasData?.schemas || []

  // Create a map of host -> schema for quick lookup
  const schemaByHost = new Map(schemas.map(s => [s.host, s]))

  const toggleHost = (host: string) => {
    setExpandedHosts(prev => {
      const next = new Set(prev)
      if (next.has(host)) {
        next.delete(host)
      } else {
        next.add(host)
      }
      return next
    })
  }

  const formatLabels: Record<SchemaFormat, string> = {
    'openapi': 'OpenAPI',
    'protobuf': 'Protobuf',
    'avro': 'Avro',
    'sql': 'SQL',
    'typescript': 'TypeScript',
    'go': 'Go',
    'json-schema': 'JSON Schema',
  }

  const formatExtensions: Record<SchemaFormat, string> = {
    'openapi': 'yaml',
    'protobuf': 'proto',
    'avro': 'avsc',
    'sql': 'sql',
    'typescript': 'ts',
    'go': 'go',
    'json-schema': 'json',
  }

  const handleCopy = useCallback(() => {
    if (schemaContent?.content) {
      navigator.clipboard.writeText(schemaContent.content)
    }
  }, [schemaContent])

  const handleDownload = useCallback(() => {
    if (!schemaContent?.content || !selectedHost) return
    const ext = formatExtensions[selectedFormat]
    const blob = new Blob([schemaContent.content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${selectedHost}.${ext}`
    a.click()
    URL.revokeObjectURL(url)
  }, [schemaContent, selectedHost, selectedFormat])

  // Get diagram from fetched data or show placeholder
  const currentDiagram = diagramData?.diagram || (selectedHost ? `sequenceDiagram
    participant C as Client
    participant A as ${selectedHost.split('.')[0]}

    Note over C,A: Run "Analyze" to generate a sequence diagram
    Note over C,A: from captured traffic patterns` : '')

  return (
    <div className="flex-1 flex overflow-hidden">
      {/* Left panel - Host/Endpoint tree */}
      <aside className="w-80 border-r border-proxy-border bg-proxy-sidebar overflow-y-auto">
        <div className="p-3 border-b border-proxy-border">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-proxy-text-dim uppercase tracking-wide">
              API Documentation
            </h2>
            <button
              className="flex items-center gap-1.5 px-2 py-1 text-xs bg-proxy-accent text-white rounded hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
              disabled={!selectedHost || isHostRunning(selectedHost)}
              onClick={() => selectedHost && handleAnalyze(selectedHost)}
            >
              {selectedHost && isHostRunning(selectedHost) ? (
                <Loader2 size={12} className="animate-spin" />
              ) : (
                <Sparkles size={12} />
              )}
              {selectedHost && isHostRunning(selectedHost) ? 'Running...' : 'Analyze'}
            </button>
          </div>
        </div>

        {isLoading ? (
          <div className="p-4 text-sm text-proxy-text-dim text-center">
            Loading...
          </div>
        ) : !enabledHosts || enabledHosts.length === 0 ? (
          <div className="p-4 text-sm text-proxy-text-dim text-center">
            <p>No hosts enabled.</p>
            <p className="mt-2 text-xs">
              Enable hosts in the Traffic tab to analyze them here.
            </p>
          </div>
        ) : (
          <div className="py-2">
            {enabledHosts.map((host) => {
              const hostSchema = schemaByHost.get(host)
              const isExpanded = expandedHosts.has(host)
              const hasSchema = !!hostSchema

              return (
                <div key={host}>
                  <button
                    className={`w-full flex items-center gap-2 px-3 py-2 hover:bg-proxy-border text-left ${
                      selectedHost === host ? 'bg-proxy-accent' : ''
                    }`}
                    onClick={() => {
                      toggleHost(host)
                      setSelectedHost(host)
                    }}
                  >
                    {hasSchema ? (
                      isExpanded ? (
                        <ChevronDown size={14} className="text-proxy-text-dim" />
                      ) : (
                        <ChevronRight size={14} className="text-proxy-text-dim" />
                      )
                    ) : (
                      <AlertCircle size={14} className="text-yellow-500" />
                    )}
                    <Globe size={14} className={hasSchema ? 'text-proxy-accent' : 'text-proxy-text-dim'} />
                    <span className="flex-1 text-sm truncate">{host}</span>
                    {isHostRunning(host) ? (
                      <Loader2 size={12} className="animate-spin text-proxy-accent" />
                    ) : hasSchema ? (
                      <span className="text-xs text-proxy-text-dim">
                        {hostSchema.endpoints?.length || 0}
                      </span>
                    ) : (
                      <span className="text-xs text-yellow-500">pending</span>
                    )}
                  </button>

                  {isExpanded && hasSchema && hostSchema.endpoints && (
                    <div className="ml-6 border-l border-proxy-border">
                      {hostSchema.endpoints.map((endpoint, idx) => (
                        <div
                          key={idx}
                          className="flex items-center gap-2 px-3 py-1.5 hover:bg-proxy-border text-xs"
                        >
                          <FileCode size={12} className="text-proxy-text-dim" />
                          <span className={`font-mono ${
                            endpoint.method === 'GET' ? 'text-emerald-400' :
                            endpoint.method === 'POST' ? 'text-blue-400' :
                            endpoint.method === 'PUT' ? 'text-yellow-400' :
                            endpoint.method === 'DELETE' ? 'text-red-400' :
                            'text-proxy-text-dim'
                          }`}>
                            {endpoint.method}
                          </span>
                          <span className="flex-1 truncate text-proxy-text-dim">
                            {endpoint.path_pattern}
                          </span>
                          <span className="text-proxy-text-dim">
                            {endpoint.sample_count}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </aside>

      {/* Right panel */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* View mode tabs */}
        <div className="h-10 flex items-center gap-2 px-4 border-b border-proxy-border bg-proxy-bg">
          <div className="flex items-center border border-proxy-border rounded">
            <button
              className={`flex items-center gap-1.5 px-3 py-1.5 text-sm ${
                viewMode === 'schemas' ? 'bg-proxy-accent text-white' : 'hover:bg-proxy-border'
              }`}
              onClick={() => setViewMode('schemas')}
            >
              <FileCode size={14} />
              Schemas
            </button>
            <button
              className={`flex items-center gap-1.5 px-3 py-1.5 text-sm ${
                viewMode === 'flows' ? 'bg-proxy-accent text-white' : 'hover:bg-proxy-border'
              }`}
              onClick={() => setViewMode('flows')}
            >
              <GitBranch size={14} />
              Flows
            </button>
          </div>

          {viewMode === 'schemas' && (
            <>
              <span className="text-sm text-proxy-text-dim ml-2">Format:</span>
              <select
                className="px-2 py-1 bg-proxy-sidebar border border-proxy-border rounded text-sm"
                value={selectedFormat}
                onChange={(e) => setSelectedFormat(e.target.value as SchemaFormat)}
              >
                {Object.entries(formatLabels).map(([value, label]) => (
                  <option key={value} value={value}>{label}</option>
                ))}
              </select>
            </>
          )}

          <div className="flex-1" />

          <button
            className="flex items-center gap-1 px-2 py-1 text-sm hover:bg-proxy-border rounded disabled:opacity-40"
            onClick={handleCopy}
            disabled={!schemaContent?.content}
          >
            <Copy size={14} />
            Copy
          </button>
          <button
            className="flex items-center gap-1 px-2 py-1 text-sm hover:bg-proxy-border rounded disabled:opacity-40"
            onClick={handleDownload}
            disabled={!schemaContent?.content}
          >
            <Download size={14} />
            Download
          </button>
        </div>

        {/* Content area */}
        <div className="flex-1 overflow-auto p-4 bg-proxy-bg">
          {!selectedHost ? (
            <div className="flex items-center justify-center h-full text-proxy-text-dim">
              <div className="text-center">
                <Workflow size={48} className="mx-auto mb-4 opacity-50" />
                <p>Select a host to view documentation</p>
              </div>
            </div>
          ) : !schemaByHost.has(selectedHost) ? (
            <div className="flex items-center justify-center h-full text-proxy-text-dim">
              <div className="text-center">
                <AlertCircle size={48} className="mx-auto mb-4 text-yellow-500 opacity-70" />
                <p className="text-lg mb-2">No schema for {selectedHost} yet</p>
                <p className="text-sm mb-4">Click "Analyze" to run AI inference on captured traffic.</p>
                <button
                  className="flex items-center gap-2 px-4 py-2 bg-proxy-accent text-white rounded hover:opacity-90 mx-auto disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={isHostRunning(selectedHost)}
                  onClick={() => handleAnalyze(selectedHost)}
                >
                  {isHostRunning(selectedHost) ? (
                    <Loader2 size={16} className="animate-spin" />
                  ) : (
                    <Sparkles size={16} />
                  )}
                  {isHostRunning(selectedHost) ? 'Running...' : `Analyze ${selectedHost}`}
                </button>
              </div>
            </div>
          ) : viewMode === 'flows' ? (
            /* Flows view with Mermaid diagram */
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-medium">API Flow: {selectedHost}</h3>
                <span className="text-sm text-proxy-text-dim">
                  Sequence diagram showing request/response correlations
                </span>
              </div>
              <div className="bg-proxy-sidebar rounded-lg border border-proxy-border p-4 min-h-[400px]">
                <MermaidDiagram chart={currentDiagram} className="w-full" />
              </div>
              <div className="text-xs text-proxy-text-dim">
                <p>This diagram shows how data flows between API calls:</p>
                <ul className="list-disc ml-4 mt-1 space-y-1">
                  <li>Solid arrows (→) represent requests</li>
                  <li>Dashed arrows (--→) represent responses</li>
                  <li>Notes show data passed between requests</li>
                </ul>
              </div>
            </div>
          ) : (
            /* Schemas view */
            <div className="font-mono text-sm whitespace-pre-wrap text-proxy-text-dim">
              <p className="text-proxy-text mb-4 font-sans">
                Schema for {selectedHost} ({formatLabels[selectedFormat]})
              </p>
              {schemaLoading ? (
                <div className="flex items-center gap-2 p-4">
                  <Loader2 size={16} className="animate-spin" />
                  <span>Loading schema...</span>
                </div>
              ) : schemaContent?.content ? (
                <pre className="p-4 bg-proxy-sidebar rounded border border-proxy-border overflow-x-auto">
{schemaContent.content}
                </pre>
              ) : (
                <div className="p-4 bg-proxy-sidebar rounded border border-proxy-border text-center">
                  <p className="mb-2">No {formatLabels[selectedFormat]} schema available yet.</p>
                  <p className="text-xs">Click "Analyze" to generate schemas from captured traffic.</p>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
