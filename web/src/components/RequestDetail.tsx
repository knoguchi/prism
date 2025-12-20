import { useState } from 'react'
import type { CaptureDetail } from '../api/types'
import { Copy, Check } from 'lucide-react'

interface RequestDetailProps {
  capture: CaptureDetail | null
  loading: boolean
}

type Tab = 'headers' | 'body' | 'response' | 'timing'

// Helper to format header values (handles both string and array formats)
const formatHeaderValue = (values: string | string[] | unknown): string => {
  if (Array.isArray(values)) {
    return values.join(', ')
  }
  if (typeof values === 'string') {
    return values
  }
  return String(values)
}

export default function RequestDetail({ capture, loading }: RequestDetailProps) {
  const [activeTab, setActiveTab] = useState<Tab>('headers')
  const [copied, setCopied] = useState(false)

  if (loading) {
    return (
      <div className="flex-1 flex items-center justify-center text-proxy-text-dim">
        Loading...
      </div>
    )
  }

  if (!capture) {
    return (
      <div className="flex-1 flex items-center justify-center text-proxy-text-dim">
        Select a request to view details
      </div>
    )
  }

  const { request, response } = capture

  const copyToClipboard = async (text: string) => {
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const formatJson = (str: string | null) => {
    if (!str) return null
    try {
      const parsed = JSON.parse(str)
      return JSON.stringify(parsed, null, 2)
    } catch {
      return str
    }
  }

  const tabs: { id: Tab; label: string }[] = [
    { id: 'headers', label: 'Headers' },
    { id: 'body', label: 'Request Body' },
    { id: 'response', label: 'Response' },
    { id: 'timing', label: 'Timing' },
  ]

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Request summary */}
      <div className="p-3 border-b border-proxy-border bg-proxy-sidebar">
        <div className="flex items-center gap-2 mb-1">
          <span className={`font-mono text-sm method-${request.method.toLowerCase()}`}>
            {request.method}
          </span>
          <span className={`font-mono text-sm ${response ? `status-${Math.floor(response.status_code / 100)}xx` : ''}`}>
            {response?.status_code || 'Pending'}
          </span>
        </div>
        <div className="text-xs text-proxy-text-dim truncate" title={request.url}>
          {request.url}
        </div>
      </div>

      {/* Tabs */}
      <div className="flex border-b border-proxy-border">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            className={`px-4 py-2 text-sm ${
              activeTab === tab.id
                ? 'border-b-2 border-proxy-accent text-proxy-text'
                : 'text-proxy-text-dim hover:text-proxy-text'
            }`}
            onClick={() => setActiveTab(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto p-3">
        {activeTab === 'headers' && (
          <div className="space-y-4">
            <div>
              <h3 className="text-xs font-semibold text-proxy-text-dim mb-2 uppercase">
                General
              </h3>
              <table className="w-full text-xs">
                <tbody>
                  <tr>
                    <td className="py-1 pr-4 text-proxy-text-dim align-top">URL</td>
                    <td className="py-1 break-all">{request.url}</td>
                  </tr>
                  <tr>
                    <td className="py-1 pr-4 text-proxy-text-dim">Method</td>
                    <td className="py-1">{request.method}</td>
                  </tr>
                  <tr>
                    <td className="py-1 pr-4 text-proxy-text-dim">Protocol</td>
                    <td className="py-1">{request.protocol}</td>
                  </tr>
                  <tr>
                    <td className="py-1 pr-4 text-proxy-text-dim">Remote</td>
                    <td className="py-1">{request.remote_addr}</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <div>
              <h3 className="text-xs font-semibold text-proxy-text-dim mb-2 uppercase">
                Request Headers
              </h3>
              <table className="w-full text-xs">
                <tbody>
                  {Object.entries(request.headers || {}).map(([key, values]) => (
                    <tr key={key}>
                      <td className="py-1 pr-4 text-proxy-text-dim align-top whitespace-nowrap">
                        {key}
                      </td>
                      <td className="py-1 break-all">{formatHeaderValue(values)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {response && (
              <div>
                <h3 className="text-xs font-semibold text-proxy-text-dim mb-2 uppercase">
                  Response Headers
                </h3>
                <table className="w-full text-xs">
                  <tbody>
                    {Object.entries(response.headers || {}).map(([key, values]) => (
                      <tr key={key}>
                        <td className="py-1 pr-4 text-proxy-text-dim align-top whitespace-nowrap">
                          {key}
                        </td>
                        <td className="py-1 break-all">{formatHeaderValue(values)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}

        {activeTab === 'body' && (
          <div>
            {request.body ? (
              <div className="relative">
                <button
                  className="absolute top-2 right-2 p-1 hover:bg-proxy-border rounded"
                  onClick={() => copyToClipboard(request.body!)}
                  title="Copy to clipboard"
                >
                  {copied ? <Check size={14} /> : <Copy size={14} />}
                </button>
                <pre className="text-xs overflow-x-auto">
                  <code>{formatJson(request.body)}</code>
                </pre>
              </div>
            ) : (
              <div className="text-proxy-text-dim text-sm">No request body</div>
            )}
          </div>
        )}

        {activeTab === 'response' && (
          <div>
            {response ? (
              <div>
                <div className="mb-2 text-xs">
                  <span className={`status-${Math.floor(response.status_code / 100)}xx font-semibold`}>
                    {response.status_code}
                  </span>
                  <span className="text-proxy-text-dim ml-2">{response.status_text}</span>
                  <span className="text-proxy-text-dim ml-4">
                    {response.body_size} bytes in {response.latency_ms}ms
                  </span>
                </div>
                {response.body ? (
                  <div className="relative">
                    <button
                      className="absolute top-2 right-2 p-1 hover:bg-proxy-border rounded"
                      onClick={() => copyToClipboard(response.body!)}
                      title="Copy to clipboard"
                    >
                      {copied ? <Check size={14} /> : <Copy size={14} />}
                    </button>
                    <pre className="text-xs overflow-x-auto">
                      <code>{formatJson(response.body)}</code>
                    </pre>
                  </div>
                ) : (
                  <div className="text-proxy-text-dim text-sm">No response body</div>
                )}
              </div>
            ) : (
              <div className="text-proxy-text-dim text-sm">No response yet</div>
            )}
          </div>
        )}

        {activeTab === 'timing' && (
          <div className="space-y-4">
            <div>
              <h3 className="text-xs font-semibold text-proxy-text-dim mb-2 uppercase">
                Timing
              </h3>
              <table className="w-full text-xs">
                <tbody>
                  <tr>
                    <td className="py-1 pr-4 text-proxy-text-dim">Started</td>
                    <td className="py-1">
                      {request.captured_at
                        ? new Date(request.captured_at).toISOString()
                        : '-'}
                    </td>
                  </tr>
                  <tr>
                    <td className="py-1 pr-4 text-proxy-text-dim">Total Latency</td>
                    <td className="py-1">{response?.latency_ms ?? '-'} ms</td>
                  </tr>
                  {response?.captured_at && (
                    <tr>
                      <td className="py-1 pr-4 text-proxy-text-dim">Completed</td>
                      <td className="py-1">{new Date(response.captured_at).toISOString()}</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            {/* Simple timing bar */}
            {response && response.latency_ms > 0 && (
              <div>
                <div className="h-6 bg-proxy-accent rounded overflow-hidden">
                  <div className="h-full w-full" title={`Total: ${response.latency_ms}ms`} />
                </div>
                <div className="flex justify-center text-xs text-proxy-text-dim mt-1">
                  <span>Total: {response.latency_ms} ms</span>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
