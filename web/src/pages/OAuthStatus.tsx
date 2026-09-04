import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import {
  ExternalLink,
  Copy,
  Check,
  CheckCircle2,
  AlertCircle,
  Loader2,
  ArrowLeft,
  KeyRound,
} from 'lucide-react'
import { api, OAuthStatus as Status } from '../api'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

export default function OAuthStatus() {
  const [sp] = useSearchParams()
  const provider = sp.get('provider') || 'chatgpt'
  const [st, setSt] = useState<Status | null>(null)
  const [err, setErr] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let n = 0
    const tick = async () => {
      try {
        const s = await api<Status>(
          `/v1/dashboard/oauth/status?provider=${encodeURIComponent(provider)}`
        )
        setSt(s)
        setErr('')
        if (s.state === 'complete' || s.state === 'error' || s.state === 'idle') return
      } catch (e) {
        setErr(e instanceof Error ? e.message : String(e))
      }
      n = window.setTimeout(tick, 1500)
    }
    tick()
    return () => clearTimeout(n)
  }, [provider])

  const copyCode = (code: string) => {
    navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="max-w-xl mx-auto py-8 space-y-6">
      <Link
        to="/"
        className="inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
      >
        <ArrowLeft className="h-3.5 w-3.5" />
        <span>Back to Auth Profiles</span>
      </Link>

      <Card>
        <CardHeader className="text-center pb-4">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary border border-primary/20 mb-2">
            <KeyRound className="h-6 w-6" />
          </div>
          <CardTitle className="text-xl capitalize">
            {provider} Authentication
          </CardTitle>
          <CardDescription className="text-xs">
            Connecting and verifying authorization flow.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {err && (
            <div className="flex items-center gap-2.5 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{err}</span>
            </div>
          )}

          {st?.error && (
            <div className="flex items-center gap-2.5 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{st.error}</span>
            </div>
          )}

          {st?.state === 'complete' && (
            <div className="flex flex-col items-center justify-center py-6 text-center space-y-3">
              <div className="flex h-12 w-12 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                <CheckCircle2 className="h-6 w-6" />
              </div>
              <div>
                <h3 className="text-base font-semibold text-foreground">
                  Authorization Complete
                </h3>
                <p className="text-xs text-muted-foreground mt-1">
                  Credentials successfully acquired and stored for {provider}.
                </p>
              </div>
              <Button asChild className="mt-2">
                <Link to="/">Return to Profiles</Link>
              </Button>
            </div>
          )}

          {st?.state !== 'complete' && (
            <div className="space-y-4">
              <div className="flex items-center justify-between rounded-lg border bg-muted/40 p-3 text-xs">
                <span className="text-muted-foreground">Session Status</span>
                <div className="flex items-center gap-2">
                  {st?.state === 'pending' && (
                    <Loader2 className="h-3.5 w-3.5 animate-spin text-primary" />
                  )}
                  <Badge variant={st?.state === 'pending' ? 'outline' : 'secondary'}>
                    {st?.state || 'polling...'}
                  </Badge>
                </div>
              </div>

              {st?.kind === 'device' && st.user_code && (
                <div className="space-y-3 rounded-lg border border-primary/20 bg-primary/5 p-4 text-center">
                  <span className="text-xs text-muted-foreground">Enter this device code:</span>
                  <div className="flex items-center justify-center gap-2">
                    <span className="font-mono text-2xl font-bold tracking-widest text-primary">
                      {st.user_code}
                    </span>
                    <Button
                      size="icon"
                      variant="outline"
                      className="h-8 w-8"
                      onClick={() => copyCode(st.user_code!)}
                    >
                      {copied ? <Check className="h-4 w-4 text-emerald-400" /> : <Copy className="h-4 w-4" />}
                    </Button>
                  </div>
                  {st.verification_uri && (
                    <div className="pt-2">
                      <Button asChild variant="secondary" size="sm" className="gap-1.5">
                        <a href={st.verification_uri} target="_blank" rel="noreferrer">
                          <span>Open Verification Page</span>
                          <ExternalLink className="h-3.5 w-3.5" />
                        </a>
                      </Button>
                    </div>
                  )}
                </div>
              )}

              {st?.authorize_url && st.state === 'pending' && (
                <div className="pt-2 text-center">
                  <Button asChild className="gap-1.5 w-full">
                    <a href={st.authorize_url} target="_blank" rel="noreferrer">
                      <span>Open OAuth Authorization Page</span>
                      <ExternalLink className="h-4 w-4" />
                    </a>
                  </Button>
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
