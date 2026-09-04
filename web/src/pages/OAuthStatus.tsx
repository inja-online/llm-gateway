import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, OAuthStatus as Status } from '../api'

export default function OAuthStatus() {
  const [sp] = useSearchParams()
  const provider = sp.get('provider') || 'chatgpt'
  const [st, setSt] = useState<Status | null>(null)
  const [err, setErr] = useState('')

  useEffect(() => {
    let n = 0
    const tick = async () => {
      try {
        const s = await api<Status>(`/v1/dashboard/oauth/status?provider=${encodeURIComponent(provider)}`)
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

  return (
    <>
      <h1>oauth · {provider}</h1>
      {err && <p className="err">{err}</p>}
      {!st && <p className="muted">polling…</p>}
      {st && (
        <p>
          state <code>{st.state}</code>
          {st.kind && <> · kind <code>{st.kind}</code></>}
        </p>
      )}
      {st?.kind === 'device' && (
        <p>
          code <code>{st.user_code}</code>
          {st.verification_uri && (
            <>
              {' '}
              <a href={st.verification_uri} target="_blank" rel="noreferrer">
                {st.verification_uri}
              </a>
            </>
          )}
        </p>
      )}
      {st?.authorize_url && st.state === 'pending' && (
        <p>
          <a href={st.authorize_url}>open authorize url</a>
        </p>
      )}
      {st?.error && <p className="err">{st.error}</p>}
      {st?.state === 'complete' && <p className="badge ok">complete</p>}
      <p>
        <Link to="/">back to profiles</Link>
      </p>
    </>
  )
}
