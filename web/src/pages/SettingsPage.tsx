import { useQuery } from '@tanstack/react-query'
import { Download, Shield, Database, Server } from 'lucide-react'

async function getConfig() {
  const res = await fetch('/api/config')
  return res.json()
}

async function getCAInfo() {
  const res = await fetch('/api/config/ca')
  return res.json()
}

export default function SettingsPage() {
  const { data: config } = useQuery({
    queryKey: ['config'],
    queryFn: getConfig,
  })

  const { data: caInfo } = useQuery({
    queryKey: ['caInfo'],
    queryFn: getCAInfo,
  })

  return (
    <div className="flex-1 overflow-auto p-6 bg-proxy-bg">
      <div className="max-w-3xl mx-auto space-y-6">
        <h1 className="text-2xl font-semibold">Settings</h1>

        {/* Proxy Settings */}
        <section className="p-4 bg-proxy-sidebar rounded-lg border border-proxy-border">
          <div className="flex items-center gap-2 mb-4">
            <Server size={20} className="text-proxy-accent" />
            <h2 className="text-lg font-medium">Proxy Server</h2>
          </div>
          <div className="space-y-3 text-sm">
            <div className="flex justify-between">
              <span className="text-proxy-text-dim">Listen Address</span>
              <span className="font-mono">{config?.proxy?.listen || ':8080'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-proxy-text-dim">API Address</span>
              <span className="font-mono">{config?.api?.listen || ':9090'}</span>
            </div>
          </div>
        </section>

        {/* CA Certificate */}
        <section className="p-4 bg-proxy-sidebar rounded-lg border border-proxy-border">
          <div className="flex items-center gap-2 mb-4">
            <Shield size={20} className="text-proxy-accent" />
            <h2 className="text-lg font-medium">CA Certificate</h2>
          </div>
          <div className="space-y-3 text-sm">
            <div className="flex justify-between">
              <span className="text-proxy-text-dim">Subject</span>
              <span>{caInfo?.subject || 'AI Proxy CA'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-proxy-text-dim">Valid From</span>
              <span>{caInfo?.not_before ? new Date(caInfo.not_before).toLocaleDateString() : '-'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-proxy-text-dim">Valid Until</span>
              <span>{caInfo?.not_after ? new Date(caInfo.not_after).toLocaleDateString() : '-'}</span>
            </div>
            <div className="pt-2">
              <a
                href="/api/config/ca/download"
                className="inline-flex items-center gap-2 px-3 py-2 bg-proxy-accent text-white rounded hover:opacity-90"
              >
                <Download size={16} />
                Download CA Certificate
              </a>
              <p className="mt-2 text-xs text-proxy-text-dim">
                Install this certificate in your browser/system to enable HTTPS interception.
              </p>
            </div>
          </div>
        </section>

        {/* Storage */}
        <section className="p-4 bg-proxy-sidebar rounded-lg border border-proxy-border">
          <div className="flex items-center gap-2 mb-4">
            <Database size={20} className="text-proxy-accent" />
            <h2 className="text-lg font-medium">Storage</h2>
          </div>
          <div className="space-y-3 text-sm">
            <div className="flex justify-between">
              <span className="text-proxy-text-dim">Database</span>
              <span className="font-mono">SQLite</span>
            </div>
            <div className="flex justify-between">
              <span className="text-proxy-text-dim">Inference Min Samples</span>
              <span>{config?.inference?.min_samples || 3}</span>
            </div>
          </div>
        </section>

        {/* MCP Integration */}
        <section className="p-4 bg-proxy-sidebar rounded-lg border border-proxy-border">
          <div className="flex items-center gap-2 mb-4">
            <div className="w-5 h-5 flex items-center justify-center text-proxy-accent font-bold text-xs">
              MCP
            </div>
            <h2 className="text-lg font-medium">MCP Integration</h2>
          </div>
          <div className="text-sm text-proxy-text-dim">
            <p className="mb-3">
              Add Prism to Claude Desktop for AI-powered traffic analysis:
            </p>
            <pre className="p-3 bg-proxy-bg rounded border border-proxy-border text-xs overflow-x-auto">
{`{
  "mcpServers": {
    "prism": {
      "command": "/path/to/prism-mcp",
      "args": ["--db", "./data/proxy.db"]
    }
  }
}`}
            </pre>
          </div>
        </section>
      </div>
    </div>
  )
}
