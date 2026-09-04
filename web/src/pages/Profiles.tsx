import { FormEvent, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, Profile } from '../api'

export default function Profiles() {
  const nav = useNavigate()
  const [rows, setRows] = useState<Profile[]>([])
  const [err, setErr] = useState('')
  const [paste, setPaste] = useState<{ provider: string; token: string } | null>(null)

  async function load() {
    try {
      const r = await api<{ profiles: Profile[] }>('/v1/dashboard/profiles')
      setRows(r.profiles || [])
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }
  useEffect(() => {
    load()
  }, [])

  async function act(path: string, body?: unknown) {
    try {
      await api(path, { method: 'POST', body: body !== undefined ? JSON.stringify(body) : undefined })
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
      }>('/v1/dashboard/oauth/start', { method: 'POST', body: JSON.stringify({ provider }) })
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
    await act('/v1/dashboard/oauth/complete', { provider: paste.provider, token: paste.token })
    setPaste(null)
  }

  return (
    <>
      <h1>profiles</h1>
      {err && <p className="err">{err}</p>}
      {paste && (
        <form onSubmit={submitPaste} className="filters">
          <span>paste {paste.provider} token</span>
          <input
            type="password"
            autoComplete="off"
            value={paste.token}
            onChange={(e) => setPaste({ ...paste, token: e.target.value })}
            autoFocus
          />
          <button className="accent" type="submit">save</button>
          <button type="button" onClick={() => setPaste(null)}>cancel</button>
        </form>
      )}
      <table>
        <thead>
          <tr>
            <th>provider</th>
            <th>account</th>
            <th>state</th>
            <th>source</th>
            <th>expiry</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((p) => (
            <tr key={`${p.provider}:${p.account_id}`}>
              <td>{p.provider}</td>
              <td>{p.account_id || 'primary'}</td>
              <td className={`badge ${p.access_state}`}>
                {p.disabled ? 'disabled' : p.access_state}
              </td>
              <td>{p.source}</td>
              <td>{p.expiry?.slice(0, 19) || '—'}</td>
              <td className="row-actions">
                {(p.provider === 'chatgpt' || p.provider === 'grok') && (
                  <button onClick={() => login(p.provider)}>login</button>
                )}
                {p.provider === 'claude' && (
                  <button onClick={() => setPaste({ provider: 'claude', token: '' })}>paste token</button>
                )}
                <button onClick={() => act('/v1/dashboard/oauth/import', { provider: p.provider })}>import</button>
                <button
                  onClick={() =>
                    act(`/v1/dashboard/profiles/${p.provider}/disable`, {
                      disabled: !p.disabled,
                      account: p.account_id,
                    })
                  }
                >
                  {p.disabled ? 'enable' : 'disable'}
                </button>
                <button
                  className="danger"
                  onClick={() =>
                    act(
                      `/v1/dashboard/profiles/${p.provider}/logout${p.account_id ? `?account=${encodeURIComponent(p.account_id)}` : ''}`,
                    )
                  }
                >
                  logout
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}
