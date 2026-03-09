'use client'

import { useState, useDeferredValue, startTransition, useRef, useMemo } from 'react'
import DashboardLayout from '@/layouts/dashboard/DashboardLayout'
import FileUpload from '@/components/main/FileUpload'
import CompanyTable from '@/components/table/CompanyTable'
import ScrapeResults from '@/components/main/ScrapeResults'
import NotFoundTable from '@/components/main/NotFoundTable'
import { Company, ScrapeResult } from '@/data'
import { scrapeCompanies } from '@/data/scraper'

export default function HomePage() {
  const [companies, setCompanies] = useState<Company[]>([])
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [results, setResults] = useState<ScrapeResult[]>([])

  const processedIds = useMemo(
    () => new Set(results.filter((r) => r.status === 'done' || r.status === 'not_found' || r.status === 'error').map((r) => r.id)),
    [results]
  )
  const visibleCompanies = useMemo(
    () => companies.filter((c) => !processedIds.has(c.id)),
    [companies, processedIds]
  )
  // useDeferredValue lets React deprioritise the heavy table re-render so
  // the progress bar and toolbar stay responsive while rows are updating.
  const deferredResults = useDeferredValue(results)
  const [isScraping, setIsScraping] = useState(false)
  const [isPaused, setIsPaused] = useState(false)
  const [activeTab, setActiveTab] = useState<'companies' | 'results' | 'notfound'>('companies')
  const abortControllerRef = useRef<AbortController | null>(null)
  const lastSelectedRef = useRef<Company[]>([])
  const sessionIdRef = useRef<string | null>(null)

  const handleCompaniesLoaded = (data: Company[]) => {
    setCompanies(data)
    setSelectedIds(new Set())
    setResults([])
    setActiveTab('companies')
  }

  const stopBackend = async () => {
    const sid = sessionIdRef.current
    if (!sid) return
    sessionIdRef.current = null
    await fetch(`/api/scrape/stop/${sid}`, { method: 'POST' }).catch(() => {})
  }

  const runScrape = async (selected: Company[]) => {
    if (selected.length === 0) return
    const controller = new AbortController()
    abortControllerRef.current = controller
    sessionIdRef.current = null
    setIsScraping(true)
    setResults([])
    setActiveTab('results')
    try {
      await scrapeCompanies(
        selected,
        (r) => startTransition(() => setResults(r)),
        controller.signal,
        (sid) => { sessionIdRef.current = sid },
        'and',
      )
    } catch (e) {
      if (!(e instanceof DOMException && e.name === 'AbortError')) throw e
    }
    abortControllerRef.current = null
    setIsScraping(false)
  }

  const handleScrape = () => {
    const selected = companies.filter((c) => selectedIds.has(c.id))
    lastSelectedRef.current = selected
    runScrape(selected)
  }

  const handleStop = async () => {
    await stopBackend()
    abortControllerRef.current?.abort()
    setIsPaused(true)
  }

  const handleResume = async () => {
    const remaining = lastSelectedRef.current.filter((c) => !processedIds.has(c.id))
    if (remaining.length === 0) return
    const controller = new AbortController()
    abortControllerRef.current = controller
    sessionIdRef.current = null
    setIsPaused(false)
    setIsScraping(true)
    setActiveTab('results')
    try {
      await scrapeCompanies(
        remaining,
        (r) => startTransition(() => setResults((prev) => {
          const map = new Map(prev.map((x) => [x.id, x]))
          r.forEach((x) => map.set(x.id, x))
          return Array.from(map.values())
        })),
        controller.signal,
        (sid) => { sessionIdRef.current = sid },
        'and',
      )
    } catch (e) {
      if (!(e instanceof DOMException && e.name === 'AbortError')) throw e
    }
    abortControllerRef.current = null
    setIsScraping(false)
  }

  const handleRestart = async () => {
    await stopBackend()
    abortControllerRef.current?.abort()
    setIsPaused(false)
    await new Promise((r) => setTimeout(r, 30))
    runScrape(lastSelectedRef.current)
  }

  const handleCancel = async () => {
    await stopBackend()
    abortControllerRef.current?.abort()
    setResults([])
    setIsScraping(false)
    setIsPaused(false)
    setActiveTab('companies')
  }

  const notFoundResults = useMemo(() => results.filter((r) => {
    if (r.status === 'not_found') return true
    if (r.status === 'done') {
      const hasEmail = (r.emails && r.emails.length > 0) || !!r.email
      const hasPhone = (r.phones && r.phones.length > 0) || !!r.telefonnummer
      return !hasEmail || !hasPhone
    }
    return false
  }), [results])

  const foundResults = useMemo(() => results.filter((r) => {
    if (r.status !== 'done') return false
    const hasEmail = (r.emails && r.emails.length > 0) || !!r.email
    const hasPhone = (r.phones && r.phones.length > 0) || !!r.telefonnummer
    return hasEmail && hasPhone
  }), [results])

  const selectedCount = selectedIds.size
  const totalScanning = results.length

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-6">
        <FileUpload onCompaniesLoaded={handleCompaniesLoaded} />

        {companies.length > 0 && (
          <div className="bg-white dark:bg-gray-800 rounded-2xl shadow-sm border border-gray-100 dark:border-gray-700 overflow-hidden transition-colors duration-200">
            {/* Tab Bar */}
            <div className="border-b border-gray-100 dark:border-gray-700">
              <div className="flex items-center justify-between px-4">
              <div className="flex">
                <button
                  onClick={() => setActiveTab('companies')}
                  className={`px-5 py-3.5 text-sm font-medium border-b-2 transition-colors ${
                    activeTab === 'companies'
                      ? 'border-blue-500 text-blue-600'
                      : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'
                  }`}
                >
                  Şirketler
                  <span className="ml-2 bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-xs px-2 py-0.5 rounded-full">
                    {visibleCompanies.length}
                  </span>
                </button>
                <button
                  onClick={() => setActiveTab('results')}
                  className={`px-5 py-3.5 text-sm font-medium border-b-2 transition-colors ${
                    activeTab === 'results'
                      ? 'border-blue-500 text-blue-600'
                      : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'
                  }`}
                >
                  Bulunan Şirketler
                  {results.length > 0 && (
                    <span className="ml-2 bg-blue-100 text-blue-600 text-xs px-2 py-0.5 rounded-full">
                      {foundResults.length}/{totalScanning}
                    </span>
                  )}
                </button>
                {notFoundResults.length > 0 && (
                  <button
                    onClick={() => setActiveTab('notfound')}
                    className={`px-5 py-3.5 text-sm font-medium border-b-2 transition-colors ${
                      activeTab === 'notfound'
                        ? 'border-orange-500 text-orange-600'
                        : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'
                    }`}
                  >
                    Bulunamayan Şirketler
                    <span className="ml-2 bg-orange-100 text-orange-600 text-xs px-2 py-0.5 rounded-full">
                      {notFoundResults.length}/{totalScanning}
                    </span>
                  </button>
                )}
              </div>

              <div className="flex items-center gap-2">
                {(isScraping || isPaused) && (
                  <>
                    {isScraping && (
                      <div className="flex items-center gap-1.5 text-xs text-blue-600">
                        <span className="w-3 h-3 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
                        Tarama devam ediyor...
                      </div>
                    )}
                    {isPaused ? (
                      <button
                        onClick={handleResume}
                        className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-green-50 text-green-700 hover:bg-green-100 border border-green-200 transition-all"
                      >
                        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        Devam
                      </button>
                    ) : (
                      <button
                        onClick={handleStop}
                        className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-yellow-50 text-yellow-700 hover:bg-yellow-100 border border-yellow-200 transition-all"
                      >
                        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 9v6m4-6v6M9 5H7a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                        </svg>
                        Durdur
                      </button>
                    )}
                    <button
                      onClick={handleRestart}
                      className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-blue-50 text-blue-700 hover:bg-blue-100 border border-blue-200 transition-all"
                    >
                      <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                      </svg>
                      Yeniden Başlat
                    </button>
                    <button
                      onClick={handleCancel}
                      className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-red-50 text-red-600 hover:bg-red-100 border border-red-200 transition-all"
                    >
                      <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                      </svg>
                      İptal
                    </button>
                  </>
                )}
                {activeTab === 'companies' && (
                  <div className="flex items-center gap-3">
                  <button
                    onClick={handleScrape}
                    disabled={selectedCount === 0 || isScraping}
                    className={`flex items-center gap-2 px-5 py-2 rounded-lg text-sm font-medium transition-all ${
                      selectedCount > 0 && !isScraping
                        ? 'bg-blue-600 hover:bg-blue-700 text-white shadow-sm'
                        : 'bg-gray-100 dark:bg-gray-700 text-gray-400 dark:text-gray-500 cursor-not-allowed'
                    }`}
                  >
                    {isScraping ? (
                      <>
                        <span className="inline-block w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                        Taranıyor...
                      </>
                    ) : (
                      <>
                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                            d="M13 10V3L4 14h7v7l9-11h-7z" />
                        </svg>
                        Tara {selectedCount > 0 ? `(${selectedCount})` : ''}
                      </>
                    )}
                  </button>
                  </div>
                )}
              </div>
            </div>
            </div>

            {activeTab === 'companies' ? (
              visibleCompanies.length === 0 && results.length > 0 ? (
                <div className="flex flex-col items-center justify-center py-16 text-gray-400 dark:text-gray-500">
                  <svg className="w-12 h-12 mb-3 text-green-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                      d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <p className="text-sm font-medium text-gray-600 dark:text-gray-300">Tüm şirketler tarandı</p>
                  <p className="text-xs mt-1">Sonuçları görmek için &quot;Sonuçlar&quot; sekmesine geçin</p>
                </div>
              ) : (
              <CompanyTable
                companies={visibleCompanies}
                selectedIds={selectedIds}
                onSelectionChange={setSelectedIds}
              />
              )
            ) : activeTab === 'results' ? (
              <ScrapeResults results={deferredResults} isScraping={isScraping} />
            ) : (
              <NotFoundTable
                results={deferredResults}
                onDetailedScan={(selected) => {
                  // TODO: SaaS entegrasyonu — seçili şirketleri detaylı tara
                  console.log('Detaylı tarama için seçilen şirketler:', selected)
                  alert(`${selected.length} şirket detaylı tarama için seçildi.\n(SaaS entegrasyonu yakında)`)
                }}
              />
            )}
          </div>
        )}
      </div>
    </DashboardLayout>
  )
}
