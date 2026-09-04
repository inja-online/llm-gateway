import { FormEvent, useEffect, useState } from 'react'
import {
  Save,
  CheckCircle2,
  AlertTriangle,
  Database,
  Layers,
  FileCode2,
  Server,
  AlertCircle,
} from 'lucide-react'
import { api, Meta, Settings as S } from '../api'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

export default function Settings() {
  const [meta, setMeta] = useState<Meta | null>(null)
  const [s, setS] = useState<S | null>(null)
  const [path, setPath] = useState('')
  const [ring, setRing] = useState('2000')
  const [err, setErr] = useState('')
  const [ok, setOk] = useState('')
  const [saving, setSaving] = useState(false)

  async function load() {
    try {
      const [m, st] = await Promise.all([
        api<Meta>('/v1/dashboard/meta'),
        api<S>('/v1/dashboard/settings'),
      ])
      setMeta(m)
      setS(st)
      setPath(st.sqlite_path)
      setRing(String(st.ring_size))
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function save(e: FormEvent) {
    e.preventDefault()
    setOk('')
    setSaving(true)
    try {
      const n = Number(ring) || 0
      const st = await api<S>('/v1/dashboard/settings', {
        method: 'PUT',
        body: JSON.stringify({ sqlite_path: path, ring_size: n }),
      })
      setS(st)
      setPath(st.sqlite_path)
      setRing(String(st.ring_size))
      setOk('Settings successfully applied to running process')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6 max-w-4xl">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Gateway Settings</h1>
        <p className="text-sm text-muted-foreground mt-0.5">
          View build metadata and configure process-local telemetry sinks.
        </p>
      </div>

      {err && (
        <div className="flex items-center gap-2.5 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" />
          <span>{err}</span>
        </div>
      )}

      {ok && (
        <div className="flex items-center gap-2.5 rounded-lg border border-emerald-500/20 bg-emerald-500/10 p-3 text-sm text-emerald-400">
          <CheckCircle2 className="h-4 w-4 shrink-0" />
          <span>{ok}</span>
        </div>
      )}

      {/* Build & Runtime Info */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base font-semibold flex items-center gap-2">
            <Server className="h-4 w-4 text-primary" />
            <span>Runtime Environment</span>
          </CardTitle>
          <CardDescription className="text-xs">
            Compiled binary attributes and storage capabilities.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 pt-1">
            <div className="space-y-1">
              <span className="text-xs text-muted-foreground uppercase tracking-wider">Version</span>
              <div className="font-mono text-sm font-semibold">{meta?.version || 'dev'}</div>
            </div>
            <div className="space-y-1">
              <span className="text-xs text-muted-foreground uppercase tracking-wider">SQLite Driver</span>
              <div>
                {meta?.sqlite ? (
                  <Badge variant="success">Compiled</Badge>
                ) : (
                  <Badge variant="neutral">nodb stub</Badge>
                )}
              </div>
            </div>
            <div className="space-y-1">
              <span className="text-xs text-muted-foreground uppercase tracking-wider">JSONL Sink</span>
              <div className="font-mono text-xs text-muted-foreground truncate" title={s?.jsonl_output || 'disabled'}>
                {s?.jsonl_output || 'none'}
              </div>
            </div>
            <div className="space-y-1">
              <span className="text-xs text-muted-foreground uppercase tracking-wider">Storage State</span>
              <div>
                {s?.sqlite_enabled ? (
                  <Badge variant="success">Active</Badge>
                ) : (
                  <Badge variant="secondary">Off</Badge>
                )}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Dynamic Telemetry Configuration */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base font-semibold flex items-center gap-2">
            <Database className="h-4 w-4 text-primary" />
            <span>Telemetry & Storage Options</span>
          </CardTitle>
          <CardDescription className="text-xs">
            Update active ring buffer capacity and SQLite persistence location.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2.5 rounded-lg border border-amber-500/20 bg-amber-500/10 p-3 text-xs text-amber-400 mb-5">
            <AlertTriangle className="h-4 w-4 shrink-0" />
            <span>
              Changes applied here take effect immediately in the running process memory without rewriting <code className="font-mono bg-black/20 px-1 py-0.5 rounded">gateway.yaml</code>.
            </span>
          </div>

          <form onSubmit={save} className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="sqlite-path" className="text-xs font-medium uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
                <FileCode2 className="h-3.5 w-3.5" />
                <span>SQLite Database File Path</span>
              </label>
              <Input
                id="sqlite-path"
                placeholder="/path/to/events.db"
                value={path}
                onChange={(e) => setPath(e.target.value)}
                className="font-mono text-xs bg-background/50"
              />
              <p className="text-[11px] text-muted-foreground">
                Path to SQLite storage file. Created with 0600 permissions on first write if permitted.
              </p>
            </div>

            <div className="space-y-2">
              <label htmlFor="ring-size" className="text-xs font-medium uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
                <Layers className="h-3.5 w-3.5" />
                <span>Ring Buffer Retention Capacity</span>
              </label>
              <Input
                id="ring-size"
                type="number"
                min="1"
                max="100000"
                value={ring}
                onChange={(e) => setRing(e.target.value)}
                className="font-mono text-xs bg-background/50 max-w-xs"
              />
              <p className="text-[11px] text-muted-foreground">
                In-memory circular buffer depth for live queries and SSE stream replay (default: 2000).
              </p>
            </div>

            <div className="pt-2">
              <Button type="submit" disabled={saving} className="gap-1.5">
                <Save className="h-3.5 w-3.5" />
                <span>{saving ? 'Saving...' : 'Apply Settings'}</span>
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
