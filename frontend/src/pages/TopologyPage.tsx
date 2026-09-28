import { useEffect, useRef, useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import * as d3 from 'd3'
import { topology } from '@/lib/api'
import { PageHeader, StatusDot, Spinner } from '@/components/shared/Layout'
import type { Service, TopoEdge } from '@/types'
import { X, ZoomIn, ZoomOut, Maximize2, RefreshCw } from 'lucide-react'

const STATE_COLORS: Record<string, string> = {
  GOOD:     'hsl(142 71% 45%)',
  DEGRADED: 'hsl(38 92% 50%)',
  CRITICAL: 'hsl(0 84% 60%)',
  UNKNOWN:  'hsl(220 8% 55%)',
}

interface GraphNode extends Service { x?: number; y?: number; fx?: number | null; fy?: number | null }
interface GraphEdge extends TopoEdge { source: GraphNode; target: GraphNode }

export default function TopologyPage() {
  const svgRef = useRef<SVGSVGElement>(null)
  const simRef = useRef<d3.Simulation<GraphNode, GraphEdge> | null>(null)
  const [selected, setSelected] = useState<GraphNode | null>(null)
  const [focusId, setFocusId] = useState<string | null>(null)

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['topology-graph', focusId],
    queryFn: () => focusId ? topology.subgraph(focusId) : topology.graph(),
    refetchInterval: 30_000,
  })

  const buildGraph = useCallback(() => {
    if (!svgRef.current || !data) return
    const svg = d3.select(svgRef.current)
    svg.selectAll('*').remove()

    const { width, height } = svgRef.current.getBoundingClientRect()
    const nodes: GraphNode[] = (data.nodes ?? []).map(n => ({ ...n }))
    const edges: GraphEdge[] = (data.edges ?? []).map(e => ({
      ...e,
      source: nodes.find(n => n.id === e.source_id) ?? nodes[0],
      target: nodes.find(n => n.id === e.target_id) ?? nodes[0],
    })).filter(e => e.source && e.target)

    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.2, 4])
      .on('zoom', e => g.attr('transform', e.transform))

    svg.call(zoom as any)
    const g = svg.append('g')

    // Arrow marker
    svg.append('defs').append('marker')
      .attr('id', 'arrow').attr('markerWidth', 8).attr('markerHeight', 8)
      .attr('refX', 18).attr('refY', 3).attr('orient', 'auto')
      .append('path').attr('d', 'M0,0 L0,6 L8,3 z')
      .attr('fill', 'hsl(220 8% 38%)')

    // Color edges by latency: green(<100ms) → amber(100-500ms) → red(>500ms)
    const edgeColor = (e: GraphEdge) => {
      const ms = e.avg_latency_ms ?? 0
      if (ms > 500) return '#ef4444'
      if (ms > 100) return '#f97316'
      if (ms > 50)  return '#f59e0b'
      return 'hsl(220 8% 38%)'
    }
    const link = g.append('g').selectAll('line').data(edges).join('line')
      .attr('stroke', edgeColor)
      .attr('stroke-width', d => {
        const cpm = d.calls_per_min ?? 1
        return Math.max(0.8, Math.min(3.5, Math.log(cpm + 1) * 0.6))
      })
      .attr('stroke-opacity', 0.75)
      .attr('marker-end', 'url(#arrow)')

    // Edge latency labels (shown mid-edge)
    const edgeLabel = g.append('g').selectAll('text').data(edges.filter(e => e.avg_latency_ms && e.avg_latency_ms > 0)).join('text')
      .attr('text-anchor', 'middle')
      .attr('font-size', '8')
      .attr('font-family', 'JetBrains Mono, monospace')
      .attr('fill', edgeColor)
      .attr('pointer-events', 'none')
      .text(d => `${Math.round(d.avg_latency_ms ?? 0)}ms`)

    const node = g.append('g').selectAll('g').data(nodes).join('g')
      .attr('cursor', 'pointer')
      .call((d3.drag<SVGGElement, GraphNode>()
        .on('start', (e: any, d: any) => { if (!e.active) sim.alphaTarget(0.3).restart(); d.fx = d.x; d.fy = d.y })
        .on('drag',  (e: any, d: any) => { d.fx = e.x; d.fy = e.y })
        .on('end',   (e: any, d: any) => { if (!e.active) sim.alphaTarget(0); d.fx = null; d.fy = null })) as any
      )
      .on('click', (_, d) => { setSelected(d); setFocusId(null) })
      .on('dblclick', (_, d) => { setFocusId(d.id); setSelected(null) })

    node.append('circle')
      .attr('r', 18)
      .attr('fill', 'hsl(220 11% 14%)')
      .attr('stroke', d => STATE_COLORS[d.health.state] ?? STATE_COLORS.UNKNOWN)
      .attr('stroke-width', 2)

    node.append('text')
      .text(d => d.name.length > 10 ? d.name.slice(0, 9) + '…' : d.name)
      .attr('text-anchor', 'middle').attr('dy', '0.35em')
      .attr('font-size', '9').attr('font-family', 'JetBrains Mono, monospace')
      .attr('fill', 'hsl(215 20% 70%)')

    node.append('circle')
      .attr('r', 4).attr('cx', 12).attr('cy', -14)
      .attr('fill', d => STATE_COLORS[d.health.state] ?? STATE_COLORS.UNKNOWN)

    const sim = d3.forceSimulation<GraphNode>(nodes)
      .force('link',    d3.forceLink<GraphNode, GraphEdge>(edges).id(d => d.id).distance(100))
      .force('charge',  d3.forceManyBody().strength(-300))
      .force('center',  d3.forceCenter(width / 2, height / 2))
      .force('collide', d3.forceCollide(30))
      .on('tick', () => {
        link.attr('x1', d => (d.source as GraphNode).x ?? 0)
            .attr('y1', d => (d.source as GraphNode).y ?? 0)
            .attr('x2', d => (d.target as GraphNode).x ?? 0)
            .attr('y2', d => (d.target as GraphNode).y ?? 0)
        edgeLabel
          .attr('x', d => (((d.source as GraphNode).x ?? 0) + ((d.target as GraphNode).x ?? 0)) / 2)
          .attr('y', d => (((d.source as GraphNode).y ?? 0) + ((d.target as GraphNode).y ?? 0)) / 2 - 4)
        node.attr('transform', d => `translate(${d.x ?? 0},${d.y ?? 0})`)
      })

    simRef.current = sim
    // Center view
    svg.transition().duration(300).call(zoom.translateTo as any, width / 2, height / 2)
  }, [data])

  useEffect(() => { buildGraph() }, [buildGraph])

  const zoomBy = (factor: number) => {
    if (!svgRef.current) return
    d3.select(svgRef.current).transition().duration(200).call(
      d3.zoom<SVGSVGElement, unknown>().scaleBy as any, factor
    )
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title={focusId ? 'Service subgraph' : 'Topology map'}
        subtitle={focusId ? 'Double-click a service to expand its connections' : `${data?.nodes?.length ?? 0} services · ${data?.edges?.length ?? 0} connections`}
        actions={
          <div className="flex gap-1">
            {focusId && (
              <button onClick={() => setFocusId(null)}
                className="flex items-center gap-1.5 px-2.5 py-1.5 text-xs bg-surface-2 hover:bg-surface-3 border border-surface-3 text-slate-400 rounded-lg">
                <X size={12} /> Show all
              </button>
            )}
            <button onClick={() => zoomBy(1.3)} className="p-1.5 bg-surface-2 hover:bg-surface-3 border border-surface-3 text-slate-400 rounded-lg"><ZoomIn size={14} /></button>
            <button onClick={() => zoomBy(0.7)} className="p-1.5 bg-surface-2 hover:bg-surface-3 border border-surface-3 text-slate-400 rounded-lg"><ZoomOut size={14} /></button>
            <button onClick={() => refetch()} className="p-1.5 bg-surface-2 hover:bg-surface-3 border border-surface-3 text-slate-400 rounded-lg"><RefreshCw size={14} /></button>
          </div>
        }
      />

      <div className="flex-1 relative overflow-hidden">
        {isLoading && (
          <div className="absolute inset-0 flex items-center justify-center bg-surface-0/60 z-10">
            <Spinner size={24} />
          </div>
        )}

        <svg ref={svgRef} className="w-full h-full" />

        {/* Legend */}
        <div className="absolute bottom-4 left-4 flex items-center gap-4 bg-surface-1/80 backdrop-blur border border-surface-3 rounded-xl px-3 py-2">
          {Object.entries(STATE_COLORS).map(([state, color]) => (
            <div key={state} className="flex items-center gap-1.5">
              <span className="w-2 h-2 rounded-full" style={{ background: color }} />
              <span className="text-xs text-slate-500">{state}</span>
            </div>
          ))}
        </div>

        {/* Detail panel */}
        {selected && (
          <div className="absolute top-4 right-4 w-72 bg-surface-1 border border-surface-3 rounded-xl p-4 animate-slide-up">
            <div className="flex items-start justify-between mb-3">
              <div>
                <h3 className="text-sm font-semibold text-slate-200 font-mono">{selected.name}</h3>
                <div className="flex items-center gap-1.5 mt-1">
                  <StatusDot status={selected.health.state} />
                  <span className="text-xs text-slate-500">{selected.health.state} · {selected.health.score}%</span>
                </div>
              </div>
              <button onClick={() => setSelected(null)} className="text-slate-600 hover:text-slate-300">
                <X size={14} />
              </button>
            </div>
            <dl className="space-y-2 text-xs">
              {[
                ['Kind', selected.kind],
                ['Namespace', selected.namespace],
                ['Cluster', selected.cluster_name],
                ['Version', selected.version ?? '—'],
                ['Tech', selected.tech_stack?.join(', ') ?? '—'],
              ].map(([k, v]) => (
                <div key={k} className="flex justify-between">
                  <dt className="text-slate-600">{k}</dt>
                  <dd className="text-slate-300 font-mono text-right max-w-[160px] truncate">{v}</dd>
                </div>
              ))}
            </dl>
            <button onClick={() => { setFocusId(selected.id); setSelected(null) }}
              className="mt-3 w-full py-1.5 text-xs bg-brand/15 hover:bg-brand/25 text-brand rounded-lg transition-colors">
              Expand subgraph
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
