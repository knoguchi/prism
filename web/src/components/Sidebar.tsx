import { useState, useMemo, useRef, useEffect } from 'react'
import { ChevronRight, ChevronDown, Globe, Folder, FileJson, Check } from 'lucide-react'
import type { CaptureListItem } from '../api/types'

interface SidebarProps {
  captures: CaptureListItem[]
  selectedId: string | null
  onSelect: (capture: CaptureListItem) => void
  enabledHosts: Set<string>
  onToggleHost: (host: string) => void
}

interface HostNode {
  host: string
  count: number
  paths: Map<string, PathNode>
}

interface PathNode {
  path: string
  methods: Map<string, CaptureListItem[]>
}

// Reverse a hostname for sorting (api.example.com -> moc.elpmaxe.ipa)
const reverseHost = (host: string) => host.split('').reverse().join('')

export default function Sidebar({ captures, selectedId, onSelect, enabledHosts, onToggleHost }: SidebarProps) {
  // PERSISTENT storage for enabled hosts' paths - never loses data
  const persistentPathsRef = useRef<Map<string, Set<string>>>(new Map())

  // Track manually collapsed hosts (enabled hosts are expanded by default)
  const manuallyCollapsedRef = useRef<Set<string>>(new Set())
  const expandedPathsRef = useRef<Set<string>>(new Set())

  // State to trigger re-renders
  const [, forceUpdate] = useState({})

  // Accumulate paths for enabled hosts - NEVER remove, only add
  useEffect(() => {
    for (const capture of captures) {
      if (enabledHosts.has(capture.host)) {
        const pathKey = capture.path.split('?')[0] || '/'
        if (!persistentPathsRef.current.has(capture.host)) {
          persistentPathsRef.current.set(capture.host, new Set())
        }
        persistentPathsRef.current.get(capture.host)!.add(pathKey)
      }
    }
  }, [captures, enabledHosts])

  // Build tree: enabled hosts use persistent paths, non-enabled use current captures
  const hostTree = useMemo(() => {
    const tree = new Map<string, HostNode>()

    // First, add all enabled hosts with their persistent paths
    for (const host of enabledHosts) {
      const persistentPaths = persistentPathsRef.current.get(host) || new Set()
      if (persistentPaths.size > 0 || captures.some(c => c.host === host)) {
        const hostNode: HostNode = { host, count: 0, paths: new Map() }
        tree.set(host, hostNode)

        // Add all persistent paths
        for (const path of persistentPaths) {
          hostNode.paths.set(path, { path, methods: new Map() })
        }
      }
    }

    // Then add captures data
    for (const capture of captures) {
      let hostNode = tree.get(capture.host)

      // For non-enabled hosts, create node only from current captures
      if (!hostNode && !enabledHosts.has(capture.host)) {
        hostNode = { host: capture.host, count: 0, paths: new Map() }
        tree.set(capture.host, hostNode)
      }

      if (!hostNode) continue

      hostNode.count++
      const pathKey = capture.path.split('?')[0] || '/'

      let pathNode = hostNode.paths.get(pathKey)
      if (!pathNode) {
        pathNode = { path: pathKey, methods: new Map() }
        hostNode.paths.set(pathKey, pathNode)
      }

      const methodCaptures = pathNode.methods.get(capture.method) || []
      methodCaptures.push(capture)
      pathNode.methods.set(capture.method, methodCaptures)
    }

    // Sort: enabled hosts first, then by reversed hostname
    return Array.from(tree.values()).sort((a, b) => {
      const aEnabled = enabledHosts.has(a.host) ? 0 : 1
      const bEnabled = enabledHosts.has(b.host) ? 0 : 1
      if (aEnabled !== bEnabled) return aEnabled - bEnabled
      return reverseHost(a.host).localeCompare(reverseHost(b.host))
    })
  }, [captures, enabledHosts])

  const isHostExpanded = (host: string) => {
    if (enabledHosts.has(host)) {
      return !manuallyCollapsedRef.current.has(host)
    }
    return false
  }

  const toggleHost = (host: string) => {
    if (enabledHosts.has(host)) {
      if (manuallyCollapsedRef.current.has(host)) {
        manuallyCollapsedRef.current.delete(host)
      } else {
        manuallyCollapsedRef.current.add(host)
      }
      forceUpdate({})
    }
  }

  const togglePath = (pathKey: string) => {
    if (expandedPathsRef.current.has(pathKey)) {
      expandedPathsRef.current.delete(pathKey)
    } else {
      expandedPathsRef.current.add(pathKey)
    }
    forceUpdate({})
  }

  const getMethodColor = (method: string) => {
    const colors: Record<string, string> = {
      GET: 'text-emerald-400',
      POST: 'text-blue-400',
      PUT: 'text-yellow-400',
      PATCH: 'text-orange-400',
      DELETE: 'text-red-400',
    }
    return colors[method] || 'text-gray-400'
  }

  return (
    <aside className="h-full w-full bg-proxy-sidebar overflow-y-auto">
      <div className="p-2 text-xs font-semibold text-proxy-text-dim uppercase tracking-wide">
        Hosts
      </div>

      {hostTree.length === 0 ? (
        <div className="p-4 text-sm text-proxy-text-dim text-center">
          No captures yet
        </div>
      ) : (
        <div className="pb-4">
          {hostTree.map((hostNode) => {
            const expanded = isHostExpanded(hostNode.host)
            const pathList = Array.from(hostNode.paths.values()).sort((a, b) =>
              a.path.localeCompare(b.path)
            )

            return (
              <div key={hostNode.host}>
                {/* Host row */}
                <div className="flex items-center hover:bg-proxy-border">
                  {/* Checkbox */}
                  <button
                    className="p-1 ml-1"
                    onClick={(e) => {
                      e.stopPropagation()
                      onToggleHost(hostNode.host)
                    }}
                    title={enabledHosts.has(hostNode.host) ? 'Hide traffic from this host' : 'Show traffic from this host'}
                  >
                    <div className={`w-4 h-4 rounded border flex items-center justify-center ${
                      enabledHosts.has(hostNode.host)
                        ? 'bg-proxy-accent border-proxy-accent'
                        : 'border-proxy-text-dim'
                    }`}>
                      {enabledHosts.has(hostNode.host) && <Check size={12} className="text-white" />}
                    </div>
                  </button>
                  {/* Expand/collapse and host name */}
                  <button
                    className="flex-1 flex items-center gap-1 px-1 py-1 text-left"
                    onClick={() => toggleHost(hostNode.host)}
                  >
                    {expanded ? (
                      <ChevronDown size={14} className="text-proxy-text-dim" />
                    ) : (
                      <ChevronRight size={14} className="text-proxy-text-dim" />
                    )}
                    <Globe size={14} className={enabledHosts.has(hostNode.host) ? 'text-proxy-accent' : 'text-proxy-text-dim'} />
                    <span className={`flex-1 truncate text-sm ${!enabledHosts.has(hostNode.host) ? 'text-proxy-text-dim' : ''}`}>
                      {hostNode.host}
                    </span>
                    <span className="text-xs text-proxy-text-dim mr-2">{hostNode.count}</span>
                  </button>
                </div>

                {/* Paths - shown for expanded enabled hosts */}
                {expanded && (
                  <div className="ml-4">
                    {pathList.map((pathNode) => {
                      const pathKey = `${hostNode.host}:${pathNode.path}`
                      const isPathExpanded = expandedPathsRef.current.has(pathKey)
                      const methodList = Array.from(pathNode.methods.entries())

                      return (
                        <div key={pathKey}>
                          {/* Path row */}
                          <button
                            className="w-full flex items-center gap-1 px-2 py-0.5 hover:bg-proxy-border text-left"
                            onClick={() => togglePath(pathKey)}
                          >
                            {methodList.length > 0 ? (
                              isPathExpanded ? (
                                <ChevronDown size={12} className="text-proxy-text-dim" />
                              ) : (
                                <ChevronRight size={12} className="text-proxy-text-dim" />
                              )
                            ) : (
                              <span className="w-3" />
                            )}
                            <Folder size={12} className="text-proxy-text-dim" />
                            <span className="flex-1 truncate text-xs">{pathNode.path}</span>
                            {methodList.length > 0 && (
                              <span className="text-xs text-proxy-text-dim">
                                {methodList.reduce((sum, [, caps]) => sum + caps.length, 0)}
                              </span>
                            )}
                          </button>

                          {/* Methods */}
                          {isPathExpanded && methodList.length > 0 && (
                            <div className="ml-4">
                              {methodList.map(([method, methodCaptures]) => (
                                <div key={method}>
                                  {methodCaptures.slice(0, 5).map((capture) => (
                                    <button
                                      key={capture.id}
                                      className={`w-full flex items-center gap-1 px-2 py-0.5 hover:bg-proxy-border text-left ${
                                        selectedId === capture.id ? 'bg-proxy-accent' : ''
                                      }`}
                                      onClick={() => onSelect(capture)}
                                    >
                                      <FileJson size={10} className="text-proxy-text-dim" />
                                      <span className={`text-xs font-mono ${getMethodColor(method)}`}>
                                        {method}
                                      </span>
                                      <span className="text-xs text-proxy-text-dim">
                                        {capture.status_code}
                                      </span>
                                    </button>
                                  ))}
                                  {methodCaptures.length > 5 && (
                                    <div className="px-2 py-0.5 text-xs text-proxy-text-dim">
                                      +{methodCaptures.length - 5} more
                                    </div>
                                  )}
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </aside>
  )
}