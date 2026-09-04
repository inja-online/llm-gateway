import { useEffect, useState } from 'react'
import { NavLink, Route, Routes } from 'react-router-dom'
import { setOnAuth } from './api'
import Gate from './pages/Gate'
import Logs from './pages/Logs'
import OAuthStatus from './pages/OAuthStatus'
import Profiles from './pages/Profiles'
import Settings from './pages/Settings'
import Usage from './pages/Usage'

export default function App() {
  const [gate, setGate] = useState(false)
  const [gen, setGen] = useState(0)
  useEffect(() => {
    setOnAuth(() => setGate(true))
  }, [])
  return (
    <>
      <header className="app">
        <strong>inja</strong>
        <nav>
          <NavLink to="/" end>
            profiles
          </NavLink>
          <NavLink to="/usage">usage</NavLink>
          <NavLink to="/logs">logs</NavLink>
          <NavLink to="/settings">settings</NavLink>
        </nav>
      </header>
      <main key={gen}>
        <Routes>
          <Route path="/" element={<Profiles />} />
          <Route path="/usage" element={<Usage />} />
          <Route path="/logs" element={<Logs />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="/oauth" element={<OAuthStatus />} />
        </Routes>
      </main>
      {gate && (
        <Gate
          onUnlock={() => {
            setGate(false)
            setGen((n) => n + 1)
          }}
        />
      )}
    </>
  )
}
