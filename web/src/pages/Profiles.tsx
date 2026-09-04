import { FormEvent, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  LogIn,
  ClipboardPaste,
  Download,
  Power,
  PowerOff,
  LogOut,
  RefreshCw,
  AlertCircle,
  KeyRound,
  ShieldCheck,
  Calendar,
} from 'lucide-react'
import { api, Profile } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

export default function Profiles() {
  const nav = useNavigate()
  const [rows, setRows] = useState<Profile[]>([])
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [paste, setPaste] = useState<{ provider: string; token: string } | null>(null)

  async function load() {
    setLoading(true)
    try {
      const r = await api<{ profiles: Profile[] }>('/v1/dashboard/profiles')
      setRows(r.profiles || [])
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function act(path: string, body?: unknown) {
    try {
      await api(path, {
        method: 'POST',
        body: body !== undefined ? JSON.stringify(body) : undefined,
      })
      await load()
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }

  async function login(provider: string) {
    try {
      const r = await api<{
        kind: string
        authorize_url?: string
        user_code?: string
        verification_uri?: string
      }>('/v1/dashboard/oauth/start', {
        method: 'POST',
        body: JSON.stringify({ provider }),
      })
      if (r.kind === 'redirect' && r.authorize_url) {
        window.open(r.authorize_url, '_blank') || (window.location.href = r.authorize_url)
      }
      nav(`/oauth?provider=${encodeURIComponent(provider)}`)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }

  async function submitPaste(e: FormEvent) {
    e.preventDefault()
    if (!paste) return
    await act('/v1/dashboard/oauth/complete', {
      provider: paste.provider,
      token: paste.token,
    })
    setPaste(null)
  }

  const renderBadge = (p: Profile) => {
    if (p.disabled) {
      return <Badge variant="neutral">disabled</Badge>
    }
    switch (p.access_state) {
      case 'present':
      case 'ok':
        return <Badge variant="success">active</Badge>
      case 'expired':
      case 'cooldown':
        return <Badge variant="warning">{p.access_state}</Badge>
      case 'missing':
      case 'empty':
      case 'upstream_error':
        return <Badge variant="destructive">{p.access_state}</Badge>
      default:
        return <Badge variant="secondary">{p.access_state}</Badge>
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Auth Profiles</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            Manage provider credentials, OAuth sessions, and multi-account pools.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => load()}
            disabled={loading}
            className="gap-1.5"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>Refresh</span>
          </Button>
        </div>
      </div>

      {err && (
        <div className="flex items-center gap-2.5 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" />
          <span>{err}</span>
        </div>
      )}

      {paste && (
        <Card className="border-primary/30 bg-card/60 backdrop-blur">
          <CardHeader className="pb-3">
            <CardTitle className="text-base flex items-center gap-2">
              <KeyRound className="h-4 w-4 text-primary" />
              <span>Paste {paste.provider} Session Token</span>
            </CardTitle>
            <CardDescription className="text-xs">
              Provide manual session authorization token for this provider.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={submitPaste} className="flex flex-col sm:flex-row gap-2">
              <Input
                type="password"
                placeholder="Paste bearer or session token..."
                autoComplete="off"
                value={paste.token}
                onChange={(e) => setPaste({ ...paste, token: e.target.value })}
                autoFocus
                className="font-mono text-xs bg-background flex-1"
              />
              <div className="flex items-center gap-2">
                <Button size="sm" type="submit" disabled={!paste.token.trim()}>
                  Save Token
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  type="button"
                  onClick={() => setPaste(null)}
                >
                  Cancel
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader className="p-4 sm:p-6 pb-3">
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-base font-semibold">Configured Providers</CardTitle>
              <CardDescription className="text-xs">
                {rows.length} {rows.length === 1 ? 'profile' : 'profiles'} registered
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Badge variant="outline" className="font-mono text-xs">
                Pool Active
              </Badge>
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="w-[140px]">Provider</TableHead>
                <TableHead className="w-[140px]">Account</TableHead>
                <TableHead className="w-[110px]">State</TableHead>
                <TableHead className="w-[120px]">Source</TableHead>
                <TableHead className="w-[160px]">Expiry</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-32 text-center text-muted-foreground">
                    <ShieldCheck className="mx-auto h-8 w-8 text-muted-foreground/40 mb-2" />
                    No auth profiles detected. Start an OAuth flow or import credentials.
                  </TableCell>
                </TableRow>
              ) : (
                rows.map((p) => (
                  <TableRow key={`${p.provider}:${p.account_id}`}>
                    <TableCell className="font-medium text-foreground capitalize">
                      {p.provider}
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {p.account_id || 'primary'}
                    </TableCell>
                    <TableCell>{renderBadge(p)}</TableCell>
                    <TableCell className="text-xs text-muted-foreground font-mono">
                      {p.source}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground font-mono">
                      {p.expiry ? (
                        <span className="flex items-center gap-1.5">
                          <Calendar className="h-3 w-3 text-muted-foreground/70" />
                          {p.expiry.slice(0, 19).replace('T', ' ')}
                        </span>
                      ) : (
                        '—'
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1.5 flex-wrap">
                        {(p.provider === 'chatgpt' || p.provider === 'grok') && (
                          <Button
                            size="sm"
                            variant="secondary"
                            className="h-7 text-xs gap-1"
                            onClick={() => login(p.provider)}
                          >
                            <LogIn className="h-3 w-3" />
                            <span>Login</span>
                          </Button>
                        )}
                        {p.provider === 'claude' && (
                          <Button
                            size="sm"
                            variant="secondary"
                            className="h-7 text-xs gap-1"
                            onClick={() => setPaste({ provider: 'claude', token: '' })}
                          >
                            <ClipboardPaste className="h-3 w-3" />
                            <span>Paste</span>
                          </Button>
                        )}
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-7 text-xs gap-1 text-muted-foreground hover:text-foreground"
                          onClick={() => act('/v1/dashboard/oauth/import', { provider: p.provider })}
                        >
                          <Download className="h-3 w-3" />
                          <span>Import</span>
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-7 text-xs gap-1 text-muted-foreground hover:text-foreground"
                          onClick={() =>
                            act(`/v1/dashboard/profiles/${p.provider}/disable`, {
                              disabled: !p.disabled,
                              account: p.account_id,
                            })
                          }
                        >
                          {p.disabled ? (
                            <>
                              <Power className="h-3 w-3 text-emerald-400" />
                              <span>Enable</span>
                            </>
                          ) : (
                            <>
                              <PowerOff className="h-3 w-3 text-amber-400" />
                              <span>Disable</span>
                            </>
                          )}
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-7 text-xs gap-1 text-destructive hover:bg-destructive/10 hover:text-destructive"
                          onClick={() =>
                            act(
                              `/v1/dashboard/profiles/${p.provider}/logout${
                                p.account_id ? `?account=${encodeURIComponent(p.account_id)}` : ''
                              }`
                            )
                          }
                        >
                          <LogOut className="h-3 w-3" />
                          <span>Logout</span>
                        </Button>
                      </div>
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
