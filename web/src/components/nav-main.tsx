import { NavLink, useLocation } from 'react-router-dom'
import { type LucideIcon } from 'lucide-react'
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  SidebarMenuBadge,
} from '@/components/ui/sidebar'

export function NavMain({
  items,
}: {
  items: {
    title: string
    url: string
    icon: LucideIcon
    badge?: string
  }[]
}) {
  const location = useLocation()

  return (
    <SidebarGroup>
      <SidebarGroupLabel className="text-xs font-semibold uppercase tracking-wider text-muted-foreground/80">
        Platform
      </SidebarGroupLabel>
      <SidebarMenu>
        {items.map((item) => {
          const isActive =
            item.url === '/'
              ? location.pathname === '/'
              : location.pathname.startsWith(item.url)

          return (
            <SidebarMenuItem key={item.title}>
              <SidebarMenuButton
                asChild
                isActive={isActive}
                tooltip={item.title}
                className={
                  isActive
                    ? 'bg-sidebar-accent text-sidebar-accent-foreground font-medium'
                    : 'text-muted-foreground hover:text-foreground'
                }
              >
                <NavLink to={item.url}>
                  <item.icon className="h-4 w-4 shrink-0" />
                  <span className="font-medium tracking-tight">{item.title}</span>
                  {item.badge && (
                    <SidebarMenuBadge className="bg-primary/10 text-primary border border-primary/20 text-[10px] font-mono">
                      {item.badge}
                    </SidebarMenuBadge>
                  )}
                </NavLink>
              </SidebarMenuButton>
            </SidebarMenuItem>
          )
        })}
      </SidebarMenu>
    </SidebarGroup>
  )
}
