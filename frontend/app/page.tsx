'use client'

import { useState, useDeferredValue, startTransition } from 'react'
import DashboardLayout from '@/layouts/dashboard/DashboardLayout'
import FileUpload from '@/components/main/FileUpload'
import CompanyTable from '@/components/table/CompanyTable'
import ScrapeResults from '@/components/main/ScrapeResults'
import { Company, ScrapeResult } from '@/data'
import { scrapeCompanies } from '@/data/scraper'

export default function HomePage() {
  const [companies, setCompanies] = useState<Company[]>([])
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [results, setResults] = useState<ScrapeResult[]>([])
  // useDeferredValue lets React deprioritise the heavy table re-render so
  // the progress bar and toolbar stay responsive while rows are updating.
  const deferredResults = useDeferredValue(results)
  const [isScraping, setIsScraping] = useState(false)
  const [activeTab, setActiveTab] = useState<'companies' | 'results'>('companies')

  const handleCompaniesLoaded = (data: Company[]) => {
    setCompanies(data)
    setSelectedIds(new Set())
    setResults([])
    setActiveTab('companies')
  }

  const handleScrape = async () => {
    const selected = companies.filter((c) => selectedIds.has(c.id))
    if (selected.length === 0) return

    setIsScraping(true)
    setActiveTab('results')
    await scrapeCompanies(selected, (r) => startTransition(() => setResults(r)))
    setIsScraping(false)
  }

  const selectedCount = selectedIds.size

  return (
    <DashboardLayout>
      <div className="flex flex-col gap-6">
        <FileUpload onCompaniesLoaded={handleCompaniesLoaded} />

        {companies.length > 0 && (
          <div className="bg-white rounded-2xl shadow-sm border border-gray-100 overflow-hidden">
            {/* Tab Bar */}
            <div className="flex items-center justify-between border-b border-gray-100 px-4">
              <div className="flex">
                <button
                  onClick={() => setActiveTab('companies')}
                  className={`px-5 py-3.5 text-sm font-medium border-b-2 transition-colors ${
                    activeTab === 'companies'
                      ? 'border-blue-500 text-blue-600'
                      : 'border-transparent text-gray-500 hover:text-gray-700'
                  }`}
                >
                  Şirketler
                  <span className="ml-2 bg-gray-100 text-gray-600 text-xs px-2 py-0.5 rounded-full">
                    {companies.length}
                  </span>
                </button>
                <button
                  onClick={() => setActiveTab('results')}
                  className={`px-5 py-3.5 text-sm font-medium border-b-2 transition-colors ${
                    activeTab === 'results'
                      ? 'border-blue-500 text-blue-600'
                      : 'border-transparent text-gray-500 hover:text-gray-700'
                  }`}
                >
                  Sonuçlar
                  {results.length > 0 && (
                    <span className="ml-2 bg-blue-100 text-blue-600 text-xs px-2 py-0.5 rounded-full">
                      {results.length}
                    </span>
                  )}
                </button>
              </div>

              {activeTab === 'companies' && (
                <button
                  onClick={handleScrape}
                  disabled={selectedCount === 0 || isScraping}
                  className={`flex items-center gap-2 px-5 py-2 rounded-lg text-sm font-medium transition-all ${
                    selectedCount > 0 && !isScraping
                      ? 'bg-blue-600 hover:bg-blue-700 text-white shadow-sm'
                      : 'bg-gray-100 text-gray-400 cursor-not-allowed'
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
              )}
            </div>

            {activeTab === 'companies' ? (
              <CompanyTable
                companies={companies}
                selectedIds={selectedIds}
                onSelectionChange={setSelectedIds}
              />
            ) : (
              <ScrapeResults results={deferredResults} isScraping={isScraping} />
            )}
          </div>
        )}
      </div>
    </DashboardLayout>
  )
}
