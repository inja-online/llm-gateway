import { useCallback, useEffect, useRef, useState } from 'react'
import { api, qs, startStream, UsageEvent } from '../api'

export default function Logs() {
  const [rows, setRows] = useState<UsageEvent[]>([])
  const [provider, setProvider] = useState('')
  const [model, setModel] = useState('')
  const [status, setStatus] = useState('')
  const [q, setQ] = useState('')
  const [err, setErr] = useState('')
  const [live, setLive] = useState('off')

  const load = useCallback(async () => {
    try {
      const r = await api<{ events: UsageEvent[] }>(
        `/v1/dashboard/logs${qs({ provider, model, status, q })}`,
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
      setRows((cur) => (cur.some((x) => x.request_id === ev.request_id) ? cur : [ev, ...cur].slice(0, 500)))
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

  return (
    <>
      <h1>logs · {live}</h1>
      {err && <p className="err">{err}</p>}
      <div className="filters">
        <input placeholder="provider" value={provider} onChange={(e) => setProvider(e.target.value)} />
        <input placeholder="model" value={model} onChange={(e) => setModel(e.target.value)} />
        <input placeholder="status" value={status} onChange={(e) => setStatus(e.target.value)} />
        <input placeholder="request_id" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <table>
        <thead>
          <tr>
            <th>time</th>
            <th>id</th>
            <th>provider</th>
            <th>model</th>
            <th>status</th>
            <th>http</th>
            <th>ms</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((e) => (
            <tr key={e.request_id}>
              <td>{e.time?.slice(11, 19)}</td>
              <td>
                <code>{e.request_id}</code>
              </td>
              <td>{e.provider}</td>
              <td>{e.model}</td>
              <td className={`badge ${e.status}`}>{e.status}</td>
              <td>{e.http_status}</td>
              <td>{e.latency_ms}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}
