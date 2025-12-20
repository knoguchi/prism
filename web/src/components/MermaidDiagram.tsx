import { useEffect, useRef } from 'react'
import mermaid from 'mermaid'

interface MermaidDiagramProps {
  chart: string
  className?: string
}

// Initialize mermaid with dark theme
mermaid.initialize({
  startOnLoad: false,
  theme: 'dark',
  themeVariables: {
    primaryColor: '#6366f1',
    primaryTextColor: '#f1f5f9',
    primaryBorderColor: '#818cf8',
    lineColor: '#94a3b8',
    secondaryColor: '#1e293b',
    tertiaryColor: '#0f172a',
    background: '#0f172a',
    mainBkg: '#1e293b',
    secondBkg: '#334155',
    actorBkg: '#334155',
    actorBorder: '#818cf8',
    actorTextColor: '#f1f5f9',
    signalColor: '#f1f5f9',
    signalTextColor: '#f1f5f9',
    labelBoxBkgColor: '#334155',
    labelBoxBorderColor: '#818cf8',
    labelTextColor: '#f1f5f9',
    loopTextColor: '#f1f5f9',
    noteBorderColor: '#818cf8',
    noteBkgColor: '#475569',
    noteTextColor: '#f1f5f9',
    activationBorderColor: '#818cf8',
    activationBkgColor: '#475569',
    sequenceNumberColor: '#f1f5f9',
    textColor: '#f1f5f9',
    messageTextColor: '#f1f5f9',
  },
  sequence: {
    diagramMarginX: 20,
    diagramMarginY: 20,
    actorMargin: 80,
    width: 180,
    height: 50,
    boxMargin: 10,
    boxTextMargin: 5,
    noteMargin: 10,
    messageMargin: 40,
    mirrorActors: false,
    useMaxWidth: true,
  },
})

export default function MermaidDiagram({ chart, className = '' }: MermaidDiagramProps) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current || !chart) return

    const renderDiagram = async () => {
      try {
        // Generate unique ID for this diagram
        const id = `mermaid-${Math.random().toString(36).substr(2, 9)}`

        // Clear previous content
        containerRef.current!.innerHTML = ''

        // Render the diagram
        const { svg } = await mermaid.render(id, chart)
        containerRef.current!.innerHTML = svg
      } catch (error) {
        console.error('Mermaid rendering error:', error)
        containerRef.current!.innerHTML = `
          <div class="text-red-400 p-4">
            <p class="font-semibold">Failed to render diagram</p>
            <pre class="mt-2 text-xs overflow-auto">${chart}</pre>
          </div>
        `
      }
    }

    renderDiagram()
  }, [chart])

  if (!chart) {
    return (
      <div className={`flex items-center justify-center text-proxy-text-dim ${className}`}>
        No diagram data
      </div>
    )
  }

  return (
    <div
      ref={containerRef}
      className={`mermaid-container overflow-auto ${className}`}
    />
  )
}
