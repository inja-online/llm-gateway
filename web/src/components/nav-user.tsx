import { Shield, ShieldAlert, Cpu, CheckCircle2 } from 'lucide-react'
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { EDGE_KEY } from '@/api'

export function NavUser({
  meta,
  onLock,
}: {
  meta: { version?: string; sqlite?: boolean; nodb?: boolean } | null
  onLock: () => void
}) {
  const hasEdgeKey = Boolean(sessionStorage.getItem(EDGE_KEY))

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton
          size="lg"
          className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground border border-sidebar-border/60 bg-sidebar-accent/30 rounded-lg p-2"
          onClick={onLock}
          tooltip="Edge Auth Session"
        >
          <div className="flex aspect-square size-8 items-center justify-center rounded-md bg-primary/10 text-primary">
            {hasEdgeKey ? (
              <Shield className="size-4 text-emerald-400" />
            ) : (
              <ShieldAlert className="size-4 text-amber-400" />
            )}
          </div>
          <div className="grid flex-1 text-left text-xs leading-tight">
            <span className="truncate font-semibold flex items-center gap-1.5">
              <span>{hasEdgeKey ? 'Edge Protected' : 'Open / Local'}</span>
              <CheckCircle2 className="size-3 text-emerald-400" />
            </span>
            <span className="truncate text-[11px] text-muted-foreground font-mono">
              v{meta?.version || 'dev'} {meta?.sqlite ? '· sqlite' : '· nodb'}
            </span>
          </div>
          <Cpu className="ml-auto size-4 text-muted-foreground/60" />
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
