import { FormEvent, useState } from 'react'
import { EDGE_KEY } from '../api'

export default function Gate({ onUnlock }: { onUnlock: () => void }) {
  const [key, setKey] = useState('')
  function submit(e: FormEvent) {
    e.preventDefault()
    sessionStorage.setItem(EDGE_KEY, key.trim())
    onUnlock()
  }
  return (
    <div className="gate">
      <form onSubmit={submit}>
        <strong>edge key</strong>
        <label htmlFor="edge">Authorization bearer</label>
        <input
          id="edge"
          type="password"
          autoComplete="off"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          autoFocus
        />
        <button className="accent" type="submit">unlock</button>
      </form>
    </div>
  )
}
