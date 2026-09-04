import { useEffect, useState } from 'react'
import { api, UsageEvent, UsageTotals } from '../api'

function Bars({ data }: { data: Record<string, number> }) {
  const entries = Object.entries(data).sort((a, b) => b[1] - a[1])
  const max = Math.max(1, ...entries.map(([, n]) => n))
  const h = Math.max(80, entries.length * 22 + 8)
  return (
    <svg className="bars" viewBox={`0 0 400 ${h}`} width="400" height={h} role="img">
      {entries.map(([k, n], i) => {
        const y = 4 + i * 22
        const w = (n / max) * 260
        return (
          <g key={k}>
            <text x="0" y={y + 12} fill="#8a9178" fontSize="11" fontFamily="IBM Plex Sans, ui-sans-serif">
              {k || '(none)'}
            </text>
            <rect x="120" y={y} width={w} height="14" fill="#c8f542" />
            <text x={128 + w} y={y + 12} fill="#e6ead9" fontSize="11">
              {n}
            </text>
          </g>
        )
      })}
    </svg>
  )
}

export default function Usage() {
  const [totals, setTotals] = useState<UsageTotals | null>(null)
  const [recent, setRecent] = useState<UsageEvent[]>([])
  const [err, setErr] = useState('')

  useEffect(() => {
    api<{ totals: UsageTotals; recent: UsageEvent[] }>('/v1/dashboard/usage')
      .then((r) => {
        setTotals(r.totals)
        setRecent(r.recent || [])
        setErr('')
      })
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)))
  }, [])

  return (
    <>
      <h1>usage</h1>
      {err && <p className="err">{err}</p>}
      <div className="kpis">
        <div className="kpi">
          <b>{totals?.requests ?? '—'}</b>
          <span>requests</span>
        </div>
        <div className="kpi">
          <b>{totals?.tokens_in ?? '—'}</b>
          <span>tokens in</span>
        </div>
        <div className="kpi">
          <b>{totals?.tokens_out ?? '—'}</b>
          <span>tokens out</span>
        </div>
      </div>
      <h1>by provider</h1>
      <Bars data={totals?.by_provider || {}} />
      <h1>recent</h1>
      <table>
        <thead>
          <tr>
            <th>time</th>
            <th>provider</th>
            <th>model</th>
            <th>in</th>
            <th>out</th>
            <th>status</th>
          </tr>
        </thead>
        <tbody>
          {recent.slice(0, 50).map((e) => (
            <tr key={e.request_id}>
              <td>{e.time?.slice(11, 19)}</td>
              <td>{e.provider}</td>
              <td>{e.model}</td>
              <td>{e.tokens_in}</td>
              <td>{e.tokens_out}</td>
              <td className={`badge ${e.status}`}>{e.status}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}
