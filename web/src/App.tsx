import { useEffect, useState } from 'react'
import { Route, Routes, useLocation, Link } from 'react-router-dom'
import { ShieldCheck, ShieldAlert } from 'lucide-react'
import { setOnAuth, EDGE_KEY } from './api'
import Gate from './pages/Gate'
import Logs from './pages/Logs'
import OAuthStatus from './pages/OAuthStatus'
import Profiles from './pages/Profiles'
import Settings from './pages/Settings'
import Usage from './pages/Usage'
import {
  SidebarProvider,
  SidebarInset,
  SidebarTrigger,
} from '@/components/ui/sidebar'
import { AppSidebar } from '@/components/app-sidebar'
import { Separator } from '@/components/ui/separator'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { Badge } from '@/components/ui/badge'

function HeaderBreadcrumb() {
  const location = useLocation()

  let currentPage = 'Profiles'
  if (location.pathname.startsWith('/usage')) currentPage = 'Usage & Analytics'
  else if (location.pathname.startsWith('/logs')) currentPage = 'Request Logs'
  else if (location.pathname.startsWith('/settings')) currentPage = 'Gateway Settings'
  else if (location.pathname.startsWith('/oauth')) currentPage = 'OAuth Verification'

  const hasEdgeKey = Boolean(sessionStorage.getItem(EDGE_KEY))

  return (
    <header className="flex h-14 shrink-0 items-center justify-between border-b px-4 transition-[width,height] ease-linear group-has-[[data-collapsible=icon]]/sidebar-wrapper:h-12 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="flex items-center gap-2">
        <SidebarTrigger className="-ml-1" />
        <Separator orientation="vertical" className="mr-2 h-4" />
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem className="hidden sm:inline-flex">
              <BreadcrumbLink asChild>
                <Link to="/">Inja Gateway</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator className="hidden sm:inline-flex" />
            <BreadcrumbItem>
              <BreadcrumbPage>{currentPage}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </div>

      <div className="flex items-center gap-3">
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground font-mono">
          <span className="relative flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
            <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
          </span>
          <span className="hidden sm:inline">Gateway Online</span>
        </div>

        <Badge
          variant={hasEdgeKey ? 'success' : 'neutral'}
          className="text-[11px] gap-1 px-2 py-0.5 font-normal"
        >
          {hasEdgeKey ? (
            <>
              <ShieldCheck className="h-3 w-3 text-emerald-400" />
              <span className="hidden sm:inline">Edge Auth Active</span>
            </>
          ) : (
            <>
              <ShieldAlert className="h-3 w-3 text-zinc-400" />
              <span className="hidden sm:inline">Open Access</span>
            </>
          )}
        </Badge>
      </div>
    </header>
  )
}

export default function App() {
  const [gate, setGate] = useState(false)
  const [gen, setGen] = useState(0)

  useEffect(() => {
    setOnAuth(() => setGate(true))
  }, [])

  return (
    <SidebarProvider>
      <AppSidebar onTriggerGate={() => setGate(true)} />
      <SidebarInset>
        <HeaderBreadcrumb />
        <div className="flex flex-1 flex-col gap-4 p-4 md:p-6" key={gen}>
          <Routes>
            <Route path="/" element={<Profiles />} />
            <Route path="/usage" element={<Usage />} />
            <Route path="/logs" element={<Logs />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/oauth" element={<OAuthStatus />} />
          </Routes>
        </div>
      </SidebarInset>

      {gate && (
        <Gate
          onUnlock={() => {
            setGate(false)
            setGen((n) => n + 1)
          }}
        />
      )}
    </SidebarProvider>
  )
}
