import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Terminal,
  Search,
  FilterX,
  Radio,
  Clock,
  AlertCircle,
  Hash,
} from 'lucide-react'
import { api, qs, startStream, UsageEvent } from '../api'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

export default function Logs() {
  const [rows, setRows] = useState<UsageEvent[]>([])
  const [provider, setProvider] = useState('')
  const [model, setModel] = useState('')
  const [status, setStatus] = useState('')
  const [q, setQ] = useState('')
  const [err, setErr] = useState('')
  const [live, setLive] = useState<'sse' | 'poll' | 'off'>('off')

  const load = useCallback(async () => {
    try {
      const r = await api<{ events: UsageEvent[] }>(
        `/v1/dashboard/logs${qs({ provider, model, status, q })}`
      )
      setRows(r.events || [])
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }, [provider, model, status, q])

  const loadRef = useRef(load)
  loadRef.current = load
  const filters = useRef({ provider, model, status, q })
  filters.current = { provider, model, status, q }

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    const ac = new AbortController()
    let poll = 0
    const merge = (ev: UsageEvent) => {
      const f = filters.current
      if (f.provider && ev.provider !== f.provider) return
      if (f.model && ev.model !== f.model) return
      if (f.status && ev.status !== f.status) return
      if (f.q && !ev.request_id.includes(f.q)) return
      setRows((cur) =>
        cur.some((x) => x.request_id === ev.request_id)
          ? cur
          : [ev, ...cur].slice(0, 500)
      )
    }
    const fallback = () => {
      setLive('poll')
      const tick = () => {
        loadRef.current()
        poll = window.setTimeout(tick, 2000)
      }
      tick()
    }
    setLive('sse')
    startStream(merge, ac.signal)
      .then(() => {
        if (!ac.signal.aborted) fallback()
      })
      .catch(() => {
        if (!ac.signal.aborted) fallback()
      })
    return () => {
      ac.abort()
      clearTimeout(poll)
    }
  }, [])

  const clearFilters = () => {
    setProvider('')
    setModel('')
    setStatus('')
    setQ('')
  }

  const hasFilters = Boolean(provider || model || status || q)

  const renderHttpStatus = (status: number) => {
    if (status >= 200 && status < 300) {
      return (
        <span className="font-mono text-xs font-semibold text-emerald-400">
          {status}
        </span>
      )
    }
    if (status >= 400 && status < 500) {
      return (
        <span className="font-mono text-xs font-semibold text-amber-400">
          {status}
        </span>
      )
    }
    return (
      <span className="font-mono text-xs font-semibold text-rose-400">
        {status}
      </span>
    )
  }

  const renderStatusBadge = (s: string) => {
    if (s === 'ok' || s === 'success') {
      return <Badge variant="success">OK</Badge>
    }
    if (s.includes('err') || s.includes('fail')) {
      return <Badge variant="destructive">{s}</Badge>
    }
    return <Badge variant="secondary">{s}</Badge>
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <div className="flex items-center gap-2.5">
            <h1 className="text-2xl font-bold tracking-tight">Request Logs</h1>
            <div className="flex items-center gap-1.5 rounded-full border border-border bg-card px-2.5 py-0.5 text-xs font-medium">
              <Radio
                className={`h-3 w-3 ${
                  live === 'sse'
                    ? 'text-emerald-400 animate-pulse'
                    : live === 'poll'
                    ? 'text-amber-400'
                    : 'text-muted-foreground'
                }`}
              />
              <span className="uppercase text-[10px] tracking-wider text-muted-foreground font-mono">
                {live === 'sse' ? 'LIVE SSE' : live === 'poll' ? 'POLLING' : 'OFFLINE'}
              </span>
            </div>
          </div>
          <p className="text-sm text-muted-foreground mt-0.5">
            Inspect real-time request traffic, latency, and upstream responses.
          </p>
        </div>

        {hasFilters && (
          <Button
            variant="ghost"
            size="sm"
            onClick={clearFilters}
            className="gap-1.5 text-xs text-muted-foreground hover:text-foreground self-start sm:self-auto"
          >
            <FilterX className="h-3.5 w-3.5" />
            <span>Reset Filters</span>
          </Button>
        )}
      </div>

      {err && (
        <div className="flex items-center gap-2.5 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" />
          <span>{err}</span>
        </div>
      )}

      {/* Filter Bar */}
      <Card className="bg-card/70 border-border/60">
        <CardContent className="p-3 sm:p-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-3">
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground/60" />
              <Input
                placeholder="Filter by provider..."
                value={provider}
                onChange={(e) => setProvider(e.target.value)}
                className="pl-8 text-xs bg-background/50 h-9"
              />
            </div>
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground/60" />
              <Input
                placeholder="Filter by model..."
                value={model}
                onChange={(e) => setModel(e.target.value)}
                className="pl-8 text-xs bg-background/50 h-9"
              />
            </div>
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground/60" />
              <Input
                placeholder="Filter by status..."
                value={status}
                onChange={(e) => setStatus(e.target.value)}
                className="pl-8 text-xs bg-background/50 h-9"
              />
            </div>
            <div className="relative">
              <Hash className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground/60" />
              <Input
                placeholder="Search request_id..."
                value={q}
                onChange={(e) => setQ(e.target.value)}
                className="pl-8 text-xs font-mono bg-background/50 h-9"
              />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Logs Table */}
      <Card>
        <CardHeader className="p-4 sm:p-6 pb-3">
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-base font-semibold">Stream Events</CardTitle>
              <CardDescription className="text-xs">
                Displaying up to 500 buffered transactions.
              </CardDescription>
            </div>
            <Badge variant="outline" className="font-mono text-xs">
              {rows.length} buffered
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="w-[100px]">Time</TableHead>
                <TableHead className="w-[180px]">Request ID</TableHead>
                <TableHead className="w-[120px]">Provider</TableHead>
                <TableHead>Model</TableHead>
                <TableHead className="w-[100px]">Status</TableHead>
                <TableHead className="w-[80px] text-right">HTTP</TableHead>
                <TableHead className="w-[90px] text-right">Latency</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className="h-32 text-center text-muted-foreground">
                    <Terminal className="mx-auto h-8 w-8 text-muted-foreground/40 mb-2" />
                    No transactions matching current criteria.
                  </TableCell>
                </TableRow>
              ) : (
                rows.map((e) => (
                  <TableRow key={e.request_id} className="font-mono text-xs">
                    <TableCell className="text-muted-foreground">
                      <span className="flex items-center gap-1.5">
                        <Clock className="h-3 w-3 text-muted-foreground/50" />
                        {e.time?.slice(11, 19) || '—'}
                      </span>
                    </TableCell>
                    <TableCell className="text-foreground truncate max-w-[180px]" title={e.request_id}>
                      {e.request_id}
                    </TableCell>
                    <TableCell className="font-sans font-medium text-foreground capitalize">
                      {e.provider}
                    </TableCell>
                    <TableCell className="text-muted-foreground truncate max-w-[200px]" title={e.model}>
                      {e.model}
                    </TableCell>
                    <TableCell>{renderStatusBadge(e.status)}</TableCell>
                    <TableCell className="text-right">{renderHttpStatus(e.http_status)}</TableCell>
                    <TableCell className="text-right text-muted-foreground">
                      <span
                        className={
                          e.latency_ms > 2000
                            ? 'text-amber-400'
                            : e.latency_ms > 5000
                            ? 'text-rose-400'
                            : 'text-foreground'
                        }
                      >
                        {e.latency_ms}ms
                      </span>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}
