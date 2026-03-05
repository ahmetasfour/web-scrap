'use client'

import { useState, useMemo } from 'react'
import { Company } from '@/data'

interface CompanyTableProps {
  companies: Company[]
  selectedIds: Set<number>
  onSelectionChange: (ids: Set<number>) => void
}

type SortField = 'reName' | 'reOrt' | 'rePlz' | 'reStrasse'

export default function CompanyTable({ companies, selectedIds, onSelectionChange }: CompanyTableProps) {
  const [search, setSearch] = useState('')
  const [sortField, setSortField] = useState<SortField>('reName')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')

  const filtered = useMemo(() => {
    const q = search.toLowerCase()
    return companies
      .filter((c) =>
        c.reName.toLowerCase().includes(q) ||
        c.reOrt.toLowerCase().includes(q) ||
        c.rePlz.toLowerCase().includes(q) ||
        c.reStrasse.toLowerCase().includes(q) ||
        c.reName2.toLowerCase().includes(q) ||
        c.email.toLowerCase().includes(q)
      )
      .sort((a, b) => {
        const va = (a[sortField] ?? '').toString().toLowerCase()
        const vb = (b[sortField] ?? '').toString().toLowerCase()
        return sortDir === 'asc' ? va.localeCompare(vb) : vb.localeCompare(va)
      })
  }, [companies, search, sortField, sortDir])

  const allFilteredSelected =
    filtered.length > 0 && filtered.every((c) => selectedIds.has(c.id))

  const toggleAll = () => {
    const newSet = new Set(selectedIds)
    if (allFilteredSelected) {
      filtered.forEach((c) => newSet.delete(c.id))
    } else {
      filtered.forEach((c) => newSet.add(c.id))
    }
    onSelectionChange(newSet)
  }

  const toggleOne = (id: number) => {
    const newSet = new Set(selectedIds)
    if (newSet.has(id)) newSet.delete(id)
    else newSet.add(id)
    onSelectionChange(newSet)
  }

  const handleSort = (field: SortField) => {
    if (sortField === field) setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    else { setSortField(field); setSortDir('asc') }
  }

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) return <span className="text-gray-300 ml-1">↕</span>
    return <span className="text-blue-500 ml-1">{sortDir === 'asc' ? '↑' : '↓'}</span>
  }

  const columns: { field: SortField; label: string }[] = [
    { field: 'reName', label: 'Firma Adı' },
    { field: 'reOrt', label: 'Şehir' },
    { field: 'rePlz', label: 'PLZ' },
    { field: 'reStrasse', label: 'Sokak' },
  ]

  return (
    <div>
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-100">
        <div className="relative">
          <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400"
            fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            type="text"
            placeholder="Firma adı, şehir veya sokak ara..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 pr-4 py-2 text-sm border border-gray-200 rounded-lg
              focus:outline-none focus:ring-2 focus:ring-blue-100 focus:border-blue-400 w-72"
          />
        </div>

        <div className="flex items-center gap-3 text-xs text-gray-500">
          {selectedIds.size > 0 && (
            <span className="bg-blue-50 text-blue-600 px-2 py-1 rounded-md font-medium">
              {selectedIds.size} seçili
            </span>
          )}
          <span>{filtered.length} şirket</span>
          {search && (
            <button onClick={() => setSearch('')} className="text-gray-400 hover:text-gray-600">
              Temizle ×
            </button>
          )}
        </div>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="bg-gray-50 text-gray-500 text-xs">
              <th className="w-12 px-4 py-3">
                <input
                  type="checkbox"
                  checked={allFilteredSelected}
                  onChange={toggleAll}
                  className="w-4 h-4 rounded accent-blue-600 cursor-pointer"
                />
              </th>
              <th className="px-4 py-3 text-left font-medium text-gray-400">En Objekt</th>
              {columns.map(({ field, label }) => (
                <th key={field}
                  className="px-4 py-3 text-left font-medium cursor-pointer select-none hover:text-gray-700"
                  onClick={() => handleSort(field)}>
                  {label} <SortIcon field={field} />
                </th>
              ))}
              <th className="px-4 py-3 text-left font-medium">E-posta</th>
              <th className="px-4 py-3 text-left font-medium">Telefon</th>
              <th className="px-4 py-3 text-left font-medium">Link</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-50">
            {filtered.map((company) => {
              const gsLink = `https://www.gelbeseiten.de/suche/${encodeURIComponent(company.reName.replace(/ /g, '-'))}/${encodeURIComponent(company.reOrt)}`
              return (
              <tr
                key={company.id}
                onClick={() => toggleOne(company.id)}
                className={`cursor-pointer transition-colors hover:bg-gray-50 ${
                  selectedIds.has(company.id) ? 'bg-blue-50/50' : ''
                }`}
              >
                <td className="px-4 py-3">
                  <input
                    type="checkbox"
                    checked={selectedIds.has(company.id)}
                    onChange={() => toggleOne(company.id)}
                    onClick={(e) => e.stopPropagation()}
                    className="w-4 h-4 rounded accent-blue-600 cursor-pointer"
                  />
                </td>
                <td className="px-4 py-3 text-gray-400 text-xs font-mono">
                  {company.enObjekt || company.id}
                </td>
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <div className="w-7 h-7 rounded-lg bg-gradient-to-br from-blue-500 to-purple-600
                      flex items-center justify-center text-white text-xs font-bold flex-shrink-0">
                      {company.reName.charAt(0).toUpperCase()}
                    </div>
                    <div>
                      <p className="font-medium text-gray-800 text-xs">{company.reName}</p>
                      {company.reName2 && (
                        <p className="text-gray-400 text-xs">{company.reName2}</p>
                      )}
                    </div>
                  </div>
                </td>
                <td className="px-4 py-3 text-gray-600 text-xs">
                  {company.reOrt || <span className="text-gray-300">—</span>}
                </td>
                <td className="px-4 py-3 text-gray-500 text-xs font-mono">
                  {company.rePlz || <span className="text-gray-300">—</span>}
                </td>
                <td className="px-4 py-3 text-gray-500 text-xs">
                  {company.reStrasse
                    ? `${company.reStrasse} ${company.reHausnummer}`.trim()
                    : <span className="text-gray-300">—</span>
                  }
                </td>
                <td className="px-4 py-3 text-gray-500 text-xs">
                  {company.email
                    ? <span className="text-blue-600">{company.email}</span>
                    : <span className="text-gray-300">—</span>
                  }
                </td>
                <td className="px-4 py-3 text-gray-500 text-xs">
                  {company.telefonnummer || <span className="text-gray-300">—</span>}
                </td>
                <td className="px-4 py-3 text-xs" onClick={(e) => e.stopPropagation()}>
                  <a
                    href={gsLink}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 text-yellow-600 hover:text-yellow-700 hover:underline"
                  >
                    <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                        d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                    </svg>
                    GS
                  </a>
                </td>
              </tr>
              )
            })}
          </tbody>
        </table>

        {filtered.length === 0 && (
          <div className="text-center py-12 text-gray-400">
            <svg className="w-10 h-10 mx-auto mb-2 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M9.172 16.172a4 4 0 015.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <p className="text-sm">Eşleşen sonuç bulunamadı</p>
          </div>
        )}
      </div>
    </div>
  )
}
