'use client'

import { createContext, useContext, useState, ReactNode } from 'react'

interface MenuItem {
  key: string
  label: string
  icon?: string
  href: string
  children?: MenuItem[]
}

interface MenuContextValue {
  menuItems: MenuItem[]
  activeKey: string
  setActiveKey: (key: string) => void
  collapsed: boolean
  setCollapsed: (v: boolean) => void
}

const MenuContext = createContext<MenuContextValue | null>(null)

const defaultMenuItems: MenuItem[] = [
  { key: 'home', label: 'Ana Sayfa', href: '/' },
  { key: 'companies', label: 'Şirketler', href: '/companies' },
  { key: 'results', label: 'Sonuçlar', href: '/results' },
]

export function MenuProvider({ children }: { children: ReactNode }) {
  const [activeKey, setActiveKey] = useState('home')
  const [collapsed, setCollapsed] = useState(false)

  return (
    <MenuContext.Provider value={{
      menuItems: defaultMenuItems,
      activeKey,
      setActiveKey,
      collapsed,
      setCollapsed,
    }}>
      {children}
    </MenuContext.Provider>
  )
}

export function useMenu() {
  const ctx = useContext(MenuContext)
  if (!ctx) throw new Error('useMenu must be used within MenuProvider')
  return ctx
}
