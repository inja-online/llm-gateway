import { useEffect, useState } from 'react'
import {
  Activity,
  ArrowDownLeft,
  ArrowUpRight,
  BarChart3,
  Layers,
  Clock,
  RefreshCw,
  AlertCircle,
} from 'lucide-react'
import { api, UsageEvent, UsageTotals } from '../api'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

function ProviderDistribution({ data }: { data: Record<string, number> }) {
  const entries = Object.entries(data).sort((a, b) => b[1] - a[1])
  const total = entries.reduce((acc, [, val]) => acc + val, 0) || 1

  if (entries.length === 0) {
    return (
      <div className="py-8 text-center text-xs text-muted-foreground">
        No provider traffic recorded yet.
      </div>
    )
  }

  return (
    <div className="space-y-3.5">
      {entries.map(([name, count]) => {
        const pct = Math.round((count / total) * 100)
        return (
          <div key={name} className="space-y-1.5">
            <div className="flex items-center justify-between text-xs">
              <span className="font-medium text-foreground capitalize">
                {name || 'unknown'}
              </span>
              <span className="font-mono text-muted-foreground">
                {count.toLocaleString()} reqs ({pct}%)
              </span>
            </div>
            <div className="h-2 w-full overflow-hidden rounded-full bg-secondary/60">
              <div
                className="h-full rounded-full bg-emerald-500 transition-all duration-500"
                style={{ width: `${Math.max(pct, 3)}%` }}
              />
            </div>
          </div>
        )
      })}
    </div>
  )
}

export default function Usage() {
  const [totals, setTotals] = useState<UsageTotals | null>(null)
  const [recent, setRecent] = useState<UsageEvent[]>([])
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)

  const load = () => {
    setLoading(true)
    api<{ totals: UsageTotals; recent: UsageEvent[] }>('/v1/dashboard/usage')
      .then((r) => {
        setTotals(r.totals)
        setRecent(r.recent || [])
        setErr('')
      })
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
  }, [])

  const renderStatus = (status: string) => {
    const s = status.toLowerCase()
    if (s === 'ok' || s === 'success' || s === '200') {
      return <Badge variant="success">OK</Badge>
    }
    if (s.includes('err') || s.includes('fail')) {
      return <Badge variant="destructive">{status}</Badge>
    }
    return <Badge variant="secondary">{status}</Badge>
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Usage & Analytics</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            Real-time telemetry, token throughput, and request breakdown.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={load}
          disabled={loading}
          className="gap-1.5 self-start sm:self-auto"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
          <span>Refresh</span>
        </Button>
      </div>

      {err && (
        <div className="flex items-center gap-2.5 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" />
          <span>{err}</span>
        </div>
      )}

      {/* KPI Cards */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Card className="relative overflow-hidden">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Total Requests
            </CardTitle>
            <Activity className="h-4 w-4 text-emerald-400" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold tracking-tight font-mono">
              {totals ? totals.requests.toLocaleString() : '—'}
            </div>
            <p className="text-xs text-muted-foreground mt-1">Processed proxy requests</p>
          </CardContent>
        </Card>

        <Card className="relative overflow-hidden">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Prompt Tokens (In)
            </CardTitle>
            <ArrowDownLeft className="h-4 w-4 text-sky-400" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold tracking-tight font-mono">
              {totals ? totals.tokens_in.toLocaleString() : '—'}
            </div>
            <p className="text-xs text-muted-foreground mt-1">Inbound request tokens</p>
          </CardContent>
        </Card>

        <Card className="relative overflow-hidden">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Completion Tokens (Out)
            </CardTitle>
            <ArrowUpRight className="h-4 w-4 text-indigo-400" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold tracking-tight font-mono">
              {totals ? totals.tokens_out.toLocaleString() : '—'}
            </div>
            <p className="text-xs text-muted-foreground mt-1">Generated output tokens</p>
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        {/* Provider Distribution Card */}
        <Card className="lg:col-span-1">
          <CardHeader className="pb-3">
            <CardTitle className="text-base font-semibold flex items-center gap-2">
              <BarChart3 className="h-4 w-4 text-emerald-400" />
              <span>Traffic by Provider</span>
            </CardTitle>
            <CardDescription className="text-xs">
              Distribution of incoming requests across upstreams.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ProviderDistribution data={totals?.by_provider || {}} />
          </CardContent>
        </Card>

        {/* Recent Traffic Table Card */}
        <Card className="lg:col-span-2">
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="text-base font-semibold flex items-center gap-2">
                  <Layers className="h-4 w-4 text-primary" />
                  <span>Recent Requests</span>
                </CardTitle>
                <CardDescription className="text-xs">
                  Latest proxied transactions in memory.
                </CardDescription>
              </div>
              <Badge variant="outline" className="text-xs font-mono">
                {recent.length} recent
              </Badge>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="w-[100px]">Time</TableHead>
                  <TableHead className="w-[110px]">Provider</TableHead>
                  <TableHead>Model</TableHead>
                  <TableHead className="text-right w-[80px]">In</TableHead>
                  <TableHead className="text-right w-[80px]">Out</TableHead>
                  <TableHead className="w-[90px] text-right">Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {recent.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="h-32 text-center text-muted-foreground">
                      <Clock className="mx-auto h-8 w-8 text-muted-foreground/40 mb-2" />
                      No request history recorded yet.
                    </TableCell>
                  </TableRow>
                ) : (
                  recent.slice(0, 50).map((e) => (
                    <TableRow key={e.request_id}>
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        {e.time?.slice(11, 19) || '—'}
                      </TableCell>
                      <TableCell className="text-xs font-medium capitalize">
                        {e.provider}
                      </TableCell>
                      <TableCell className="font-mono text-xs text-foreground truncate max-w-[160px]">
                        {e.model}
                      </TableCell>
                      <TableCell className="text-right font-mono text-xs text-muted-foreground">
                        {e.tokens_in}
                      </TableCell>
                      <TableCell className="text-right font-mono text-xs text-muted-foreground">
                        {e.tokens_out}
                      </TableCell>
                      <TableCell className="text-right">{renderStatus(e.status)}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
