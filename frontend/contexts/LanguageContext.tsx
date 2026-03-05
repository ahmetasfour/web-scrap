'use client'

import { createContext, useContext, useState, ReactNode } from 'react'

type Language = 'tr' | 'en'

interface LanguageContextValue {
  language: Language
  setLanguage: (lang: Language) => void
  t: (key: string) => string
}

const translations: Record<Language, Record<string, string>> = {
  tr: {
    'nav.home': 'Ana Sayfa',
    'nav.companies': 'Şirketler',
    'nav.results': 'Sonuçlar',
    'nav.settings': 'Ayarlar',
    'action.scrape': 'Tara',
    'action.export': 'Dışa Aktar',
    'action.upload': 'Dosya Yükle',
    'status.pending': 'Bekliyor',
    'status.done': 'Tamamlandı',
    'status.error': 'Hata',
  },
  en: {
    'nav.home': 'Home',
    'nav.companies': 'Companies',
    'nav.results': 'Results',
    'nav.settings': 'Settings',
    'action.scrape': 'Scrape',
    'action.export': 'Export',
    'action.upload': 'Upload File',
    'status.pending': 'Pending',
    'status.done': 'Done',
    'status.error': 'Error',
  },
}

export const LanguageContext = createContext<LanguageContextValue>({
  language: 'tr',
  setLanguage: () => {},
  t: (key) => key,
})

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguage] = useState<Language>('tr')
  const t = (key: string): string => translations[language][key] ?? key

  return (
    <LanguageContext.Provider value={{ language, setLanguage, t }}>
      {children}
    </LanguageContext.Provider>
  )
}

export function useLanguage() {
  return useContext(LanguageContext)
}
