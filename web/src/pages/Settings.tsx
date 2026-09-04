import { FormEvent, useEffect, useState } from 'react'
import { api, Meta, Settings as S } from '../api'

export default function Settings() {
  const [meta, setMeta] = useState<Meta | null>(null)
  const [s, setS] = useState<S | null>(null)
  const [path, setPath] = useState('')
  const [ring, setRing] = useState('2000')
  const [err, setErr] = useState('')
  const [ok, setOk] = useState('')

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
    try {
      const n = Number(ring) || 0
      const st = await api<S>('/v1/dashboard/settings', {
        method: 'PUT',
        body: JSON.stringify({ sqlite_path: path, ring_size: n }),
      })
      setS(st)
      setPath(st.sqlite_path)
      setRing(String(st.ring_size))
      setOk('applied process-local')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <>
      <h1>settings</h1>
      {err && <p className="err">{err}</p>}
      {ok && <p className="badge ok">{ok}</p>}
      <p className="muted">
        version {meta?.version || '—'} · sqlite binary {meta?.sqlite ? 'yes' : 'no'} · nodb {meta?.nodb ? 'yes' : 'no'}
      </p>
      <p className="warn">PUT applies process-local only. It does not rewrite gateway.yaml.</p>
      <form onSubmit={save}>
        <p>
          <label>
            sqlite path
            <br />
            <input value={path} onChange={(e) => setPath(e.target.value)} style={{ width: '28rem' }} />
          </label>
        </p>
        <p>
          <label>
            ring size
            <br />
            <input value={ring} onChange={(e) => setRing(e.target.value)} />
          </label>
        </p>
        <p className="muted">jsonl {s?.jsonl_output || '(none)'} · sqlite {s?.sqlite_enabled ? 'on' : 'off'}</p>
        <button className="accent" type="submit">save</button>
      </form>
    </>
  )
}
