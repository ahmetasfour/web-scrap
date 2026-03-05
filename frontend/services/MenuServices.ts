export interface MenuItemDef {
  key: string
  label: string
  href: string
  icon?: string
  children?: MenuItemDef[]
  roles?: string[]
}

export const MenuServices = {
  getMenuItems(role: string = 'user'): MenuItemDef[] {
    const allItems: MenuItemDef[] = [
      { key: 'home', label: 'Ana Sayfa', href: '/' },
      { key: 'companies', label: 'Şirketler', href: '/companies' },
      { key: 'results', label: 'Sonuçlar', href: '/results' },
      { key: 'settings', label: 'Ayarlar', href: '/settings', roles: ['admin'] },
    ]

    return allItems.filter(
      (item) => !item.roles || item.roles.includes(role)
    )
  },
}
