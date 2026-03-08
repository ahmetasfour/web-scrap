import type { Metadata } from 'next'
import { AntdRegistry } from '@ant-design/nextjs-registry'
import { BreadcrumbProvider } from '@/contexts/BreadCrumbContext'
import { LanguageProvider } from '@/contexts/LanguageContext'
import './styles.css'

export const metadata: Metadata = {
  title: 'Web Scraper - Şirket Veri Toplama',
  description: 'Excel dosyasından şirket verilerini toplayın',
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="tr" suppressHydrationWarning>
      <body>
        <AntdRegistry>
          <LanguageProvider>
            <BreadcrumbProvider>
              {children}
            </BreadcrumbProvider>
          </LanguageProvider>
        </AntdRegistry>
      </body>
    </html>
  )
}
