'use client'

import { useState, useMemo } from 'react'
import * as XLSX from 'xlsx'
import { ScrapeResult } from '@/data'

function gsLink(reName: string, reOrt: string) {
  return `https://www.gelbeseiten.de/suche/${encodeURIComponent(reName.replace(/ /g, '-'))}/${encodeURIComponent(reOrt)}`
}

function getInfo(r: ScrapeResult) {
  const hasEmail = (r.emails && r.emails.length > 0) || !!r.email
  const hasPhone = (r.phones && r.phones.length > 0) || !!r.telefonnummer
  const emailVal = r.emails?.[0] ?? r.email ?? null
  const phoneVal = r.phones?.[0] ?? r.telefonnummer ?? null
  return { hasEmail, hasPhone, emailVal, phoneVal }
}

type SubTab = 'all' | 'no_email' | 'no_phone'

function exportExcel(rows: ScrapeResult[], label: string) {
  const data = rows.map((r) => {
    const { emailVal, phoneVal } = getInfo(r)
    return {
      'Firma Adı':    r.reName,
      'Firma Adı 2':  r.reName2,
      'Şehir':        r.reOrt,
      'PLZ':          r.rePlz,
      'Sokak':        r.reStrasse,
      'E-posta':      emailVal ?? '',
      'Telefon':      phoneVal ?? '',
      'Website':      r.website ?? '',
      'Durum':        r.status,
    }
  })
  const ws = XLSX.utils.json_to_sheet(data)
  const wb = XLSX.utils.book_new()
  XLSX.utils.book_append_sheet(wb, ws, 'Bulunamayan')
  XLSX.writeFile(wb, `bulunamayan-${label}-${new Date().toISOString().slice(0, 10)}.xlsx`)
}

interface NotFoundTableProps {
  results: ScrapeResult[]
  onDetailedScan?: (selected: ScrapeResult[]) => void
}

export default function NotFoundTable({ results, onDetailedScan }: NotFoundTableProps) {
  const [subTab, setSubTab] = useState<SubTab>('all')
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set())

  const all = useMemo(() => results.filter((r) => {
    if (r.status === 'not_found') return true
    if (r.status === 'done') {
      const { hasEmail, hasPhone } = getInfo(r)
      return !hasEmail || !hasPhone
    }
    return false
  }), [results])

  const noEmail = useMemo(() => all.filter((r) => { const { hasEmail, hasPhone } = getInfo(r); return hasPhone && !hasEmail }), [all])
  const noPhone = useMemo(() => all.filter((r) => { const { hasEmail, hasPhone } = getInfo(r); return hasEmail && !hasPhone }), [all])

  const rows = subTab === 'no_email' ? noEmail : subTab === 'no_phone' ? noPhone : all

  const subTabLabel = subTab === 'no_email' ? 'mail-eksik' : subTab === 'no_phone' ? 'telefon-eksik' : 'tumu'

  const allRowIds = useMemo(() => rows.map((r) => r.id), [rows])
  const isAllSelected = allRowIds.length > 0 && allRowIds.every((id) => selectedIds.has(id))
  const isIndeterminate = !isAllSelected && allRowIds.some((id) => selectedIds.has(id))

  const toggleAll = () => {
    if (isAllSelected) {
      setSelectedIds((prev) => {
        const next = new Set(prev)
        allRowIds.forEach((id) => next.delete(id))
        return next
      })
    } else {
      setSelectedIds((prev) => {
        const next = new Set(prev)
        allRowIds.forEach((id) => next.add(id))
        return next
      })
    }
  }

  const toggleRow = (id: string | number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const selectedRows = rows.filter((r) => selectedIds.has(r.id))
  const selectedCount = selectedRows.length

  const handleDetailedScan = () => {
    if (onDetailedScan) onDetailedScan(selectedRows)
  }

  const DownloadIcon = () => (
    <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
        d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
    </svg>
  )

  if (all.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-gray-400 dark:text-gray-500">
        <svg className="w-12 h-12 mb-3 text-green-300 dark:text-green-700" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
            d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <p className="text-sm font-medium text-gray-600 dark:text-gray-300">Eksik kayıt yok</p>
        <p className="text-xs mt-1">Tüm şirketlerin e-posta ve telefon bilgileri bulundu</p>
      </div>
    )
  }

  const tabCls = (active: boolean) =>
    `px-3 py-1.5 text-xs font-medium rounded-lg border transition-colors ${
      active
        ? 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-300 border-orange-300 dark:border-orange-700'
        : 'bg-white dark:bg-gray-700 text-gray-700 dark:text-gray-300 border-gray-200 dark:border-gray-600 hover:bg-gray-100 dark:hover:bg-gray-600 hover:text-gray-900 dark:hover:text-gray-100'
    }`

  return (
    <div>
      {/* Sub-tab bar */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-100 dark:border-gray-700">
        <div className="flex items-center gap-1.5">
          <button onClick={() => setSubTab('all')} className={tabCls(subTab === 'all')}>
            Tümü <span className="opacity-70">({all.length})</span>
          </button>
          <button onClick={() => setSubTab('no_email')} className={tabCls(subTab === 'no_email')}>
            Mail Eksik <span className="opacity-70">({noEmail.length})</span>
          </button>
          <button onClick={() => setSubTab('no_phone')} className={tabCls(subTab === 'no_phone')}>
            Telefon Eksik <span className="opacity-70">({noPhone.length})</span>
          </button>
        </div>

        <div className="flex items-center gap-2">
          {selectedCount > 0 && (
            <button
              onClick={handleDetailedScan}
              className="flex items-center gap-1.5 text-xs font-medium bg-purple-600 hover:bg-purple-700 text-white rounded-lg px-3 py-1.5 transition-colors shadow-sm"
            >
              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                  d="M21 21l-4.35-4.35M17 11A6 6 0 115 11a6 6 0 0112 0z" />
              </svg>
              Detaylı Tara ({selectedCount})
            </button>
          )}
          {rows.length > 0 && (
            <button
              onClick={() => exportExcel(rows, subTabLabel)}
              className="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 border border-gray-200 dark:border-gray-600 rounded-lg px-3 py-1.5 hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
            >
              <DownloadIcon />
              Excel İndir
            </button>
          )}
        </div>
      </div>

      {/* Table */}
      <div className="overflow-x-auto max-h-[500px] overflow-y-auto scrollbar-thin border border-gray-200 dark:border-gray-700 rounded-b-xl">
        <table className="w-full text-sm border-collapse [&_td]:border-r [&_td]:border-gray-200 dark:[&_td]:border-gray-700 [&_td:last-child]:border-r-0">
          <thead className="sticky top-0 z-10">
            <tr className="bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 text-xs border-b-2 border-gray-300 dark:border-gray-600">
              <th className="px-3 py-3 text-center border-r border-gray-200 dark:border-gray-600 w-10">
                <input
                  type="checkbox"
                  checked={isAllSelected}
                  ref={(el) => { if (el) el.indeterminate = isIndeterminate }}
                  onChange={toggleAll}
                  className="w-3.5 h-3.5 cursor-pointer accent-purple-600"
                  title="Tümünü seç"
                />
              </th>
              <th className="px-4 py-3 text-left font-semibold border-r border-gray-200 dark:border-gray-600">Durum</th>
              <th className="px-4 py-3 text-left font-semibold border-r border-gray-200 dark:border-gray-600">Firma Adı</th>
              <th className="px-4 py-3 text-left font-semibold whitespace-nowrap border-r border-gray-200 dark:border-gray-600">Şehir</th>
              <th className="px-4 py-3 text-left font-semibold whitespace-nowrap border-r border-gray-200 dark:border-gray-600">PLZ</th>
              <th className="px-4 py-3 text-left font-semibold border-r border-gray-200 dark:border-gray-600">E-posta</th>
              <th className="px-4 py-3 text-left font-semibold whitespace-nowrap border-r border-gray-200 dark:border-gray-600">Telefon</th>
              <th className="px-4 py-3 text-left font-semibold whitespace-nowrap">Link</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
            {rows.map((r) => {
              const { hasEmail, hasPhone, emailVal, phoneVal } = getInfo(r)
              const isNotFound = r.status === 'not_found'
              const missingBoth = !hasEmail && !hasPhone
              const badgeLabel = isNotFound ? 'Bulunamadı' : missingBoth ? 'Eksik Veri' : !hasEmail ? 'Eksik E-posta' : 'Eksik Telefon'
              const badgeCls = isNotFound
                ? 'bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-400'
                : 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-400'
              const isSelected = selectedIds.has(r.id)

              return (
                <tr
                  key={r.id}
                  onClick={() => toggleRow(r.id)}
                  className={`cursor-pointer transition-colors ${
                    isSelected
                      ? 'bg-purple-50 dark:bg-purple-900/20 hover:bg-purple-100 dark:hover:bg-purple-900/30'
                      : 'hover:bg-gray-50 dark:hover:bg-gray-700/50'
                  }`}
                >
                  <td className="px-3 py-2.5 text-center" onClick={(e) => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => toggleRow(r.id)}
                      className="w-3.5 h-3.5 cursor-pointer accent-purple-600"
                    />
                  </td>
                  <td className="px-4 py-2.5 whitespace-nowrap">
                    <span className={`inline-block text-xs px-2 py-0.5 rounded-full font-medium ${badgeCls}`}>
                      {badgeLabel}
                    </span>
                  </td>
                  <td className="px-4 py-2.5">
                    <p className="font-medium text-gray-800 dark:text-gray-100 text-xs truncate max-w-[200px]">{r.reName}</p>
                    {r.reName2 && <p className="text-gray-400 dark:text-gray-500 text-xs truncate max-w-[200px]">{r.reName2}</p>}
                  </td>
                  <td className="px-4 py-2.5 text-gray-600 dark:text-gray-300 text-xs whitespace-nowrap">
                    {r.reOrt || <span className="text-gray-300 dark:text-gray-600">—</span>}
                  </td>
                  <td className="px-4 py-2.5 text-gray-500 dark:text-gray-400 text-xs font-mono whitespace-nowrap">
                    {r.rePlz || <span className="text-gray-300 dark:text-gray-600">—</span>}
                  </td>
                  <td className="px-4 py-2.5 text-xs" onClick={(e) => e.stopPropagation()}>
                    {hasEmail && emailVal ? (
                      <a href={`mailto:${emailVal}`} className="text-blue-600 dark:text-blue-400 hover:underline">
                        {emailVal}
                      </a>
                    ) : (
                      <span className="text-gray-300 dark:text-gray-600">—</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-xs whitespace-nowrap">
                    {hasPhone && phoneVal ? (
                      <span className="text-green-700 dark:text-green-400">{phoneVal}</span>
                    ) : (
                      <span className="text-gray-300 dark:text-gray-600">—</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-xs whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                    <div className="inline-flex items-center gap-2">
                      {r.website && (
                        <a
                          href={r.website}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center gap-1 text-blue-600 dark:text-blue-400 hover:underline"
                          title={r.website}
                        >
                          <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                              d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                          </svg>
                          {new URL(r.website).hostname.replace(/^www\./, '')}
                        </a>
                      )}
                      <a
                        href={gsLink(r.reName, r.reOrt)}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1 text-yellow-600 dark:text-yellow-400 hover:underline"
                      >
                        <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                            d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                        </svg>
                        GS
                      </a>
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
