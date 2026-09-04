import { useEffect, useState } from 'react'
import {
  KeyRound,
  Activity,
  Terminal,
  Settings2,
  BookOpen,
  GitBranch,
  Lock,
  Boxes,
} from 'lucide-react'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
} from '@/components/ui/sidebar'
import { NavMain } from '@/components/nav-main'
import { NavSecondary } from '@/components/nav-secondary'
import { NavUser } from '@/components/nav-user'
import { api, Meta, EDGE_KEY } from '@/api'

const navMainItems = [
  {
    title: 'Profiles',
    url: '/',
    icon: KeyRound,
  },
  {
    title: 'Usage',
    url: '/usage',
    icon: Activity,
  },
  {
    title: 'Logs',
    url: '/logs',
    icon: Terminal,
    badge: 'LIVE',
  },
  {
    title: 'Settings',
    url: '/settings',
    icon: Settings2,
  },
]

export function AppSidebar({
  onTriggerGate,
  ...props
}: React.ComponentProps<typeof Sidebar> & {
  onTriggerGate?: () => void
}) {
  const [meta, setMeta] = useState<Meta | null>(null)

  useEffect(() => {
    api<Meta>('/v1/dashboard/meta')
      .then(setMeta)
      .catch(() => {})
  }, [])

  const handleLock = () => {
    sessionStorage.removeItem(EDGE_KEY)
    if (onTriggerGate) {
      onTriggerGate()
    } else {
      window.location.reload()
    }
  }

  const secondaryItems = [
    {
      title: 'Documentation',
      url: 'https://github.com/inja-online/llm-gateway#readme',
      icon: BookOpen,
    },
    {
      title: 'GitHub Repository',
      url: 'https://github.com/inja-online/llm-gateway',
      icon: GitBranch,
    },
    {
      title: 'Lock Session',
      icon: Lock,
      onClick: handleLock,
    },
  ]

  return (
    <Sidebar variant="inset" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild className="hover:bg-sidebar-accent/50">
              <div className="flex items-center gap-3">
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                  <Boxes className="size-4" />
                </div>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-semibold tracking-tight">inja</span>
                  <span className="truncate text-xs text-muted-foreground font-mono">
                    llm-gateway
                  </span>
                </div>
              </div>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <NavMain items={navMainItems} />
        <NavSecondary items={secondaryItems} className="mt-auto" />
      </SidebarContent>

      <SidebarFooter>
        <NavUser meta={meta} onLock={handleLock} />
      </SidebarFooter>
    </Sidebar>
  )
}
