import { FormEvent, useState } from 'react'
import { KeyRound, ShieldAlert } from 'lucide-react'
import { EDGE_KEY } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

export default function Gate({ onUnlock }: { onUnlock: () => void }) {
  const [key, setKey] = useState('')

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!key.trim()) return
    sessionStorage.setItem(EDGE_KEY, key.trim())
    onUnlock()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <Card className="w-full max-w-md border-border/80 shadow-2xl bg-card">
        <CardHeader className="text-center space-y-2">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
            <KeyRound className="h-6 w-6" />
          </div>
          <CardTitle className="text-xl tracking-tight">Operator Authentication</CardTitle>
          <CardDescription className="text-xs text-muted-foreground">
            Enter your Gateway Edge Auth bearer token to access the control plane.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="edge" className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Bearer Token / API Key
              </label>
              <Input
                id="edge"
                type="password"
                placeholder="inja_..."
                autoComplete="off"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                autoFocus
                className="font-mono text-sm bg-background/50"
              />
            </div>
            <div className="flex items-center gap-2 rounded-lg border border-amber-500/20 bg-amber-500/10 p-3 text-xs text-amber-400">
              <ShieldAlert className="h-4 w-4 shrink-0" />
              <span>Token is stored in temporary session storage only.</span>
            </div>
            <Button className="w-full font-medium" type="submit" disabled={!key.trim()}>
              Unlock Dashboard
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
