'use client'

import * as XLSX from 'xlsx'
import { ScrapeResult } from '@/data'
import { useState, useEffect, useRef, useMemo, memo } from 'react'

const StatusBadge = ({ status }: { status: ScrapeResult['status'] }) => {
  const map = {
    pending:   { label: 'Bekliyor',    cls: 'bg-gray-100 text-gray-500' },
    scraping:  { label: 'İşleniyor…',  cls: 'bg-blue-100 text-blue-600 animate-pulse' },
    done:      { label: 'Bulundu',     cls: 'bg-green-100 text-green-600' },
    not_found: { label: 'Bulunamadı',  cls: 'bg-yellow-100 text-yellow-600' },
    error:     { label: 'Hata',        cls: 'bg-red-100 text-red-600' },
  }
  const s = map[status] ?? map.pending
  return (
    <span className={`inline-block text-xs px-2 py-0.5 rounded-full font-medium whitespace-nowrap ${s.cls}`}>
      {s.label}
    </span>
  )
}

function gsLink(reName: string, reOrt: string) {
  return `https://www.gelbeseiten.de/suche/${encodeURIComponent(reName.replace(/ /g, '-'))}/${encodeURIComponent(reOrt)}`
}

interface ScrapeResultsProps {
  results: ScrapeResult[]
  isScraping: boolean
}

function ScrapeResultsInner({ results, isScraping }: ScrapeResultsProps) {
  const [filterStatus, setFilterStatus] = useState<'all' | 'done' | 'not_found' | 'error'>('all')

  // --- speed / ETA tracking ---
  const startTimeRef = useRef<number | null>(null)
  const [speed, setSpeed] = useState(0)   // items/sec
  const [eta, setEta] = useState<number | null>(null) // seconds remaining
  const prevProcessedRef = useRef(0)

  const stats = {
    total:     results.length,
    done:      results.filter((r) => r.status === 'done').length,
    not_found: results.filter((r) => r.status === 'not_found').length,
    error:     results.filter((r) => r.status === 'error').length,
    pending:   results.filter((r) => r.status === 'pending').length,
  }
  const processed = stats.done + stats.not_found + stats.error
  const pct = stats.total > 0 ? Math.round((processed / stats.total) * 100) : 0

  useEffect(() => {
    if (isScraping && startTimeRef.current === null) {
      startTimeRef.current = Date.now()
    }
    if (!isScraping) {
      startTimeRef.current = null
      setSpeed(0)
      setEta(null)
      prevProcessedRef.current = 0
      return
    }
    if (startTimeRef.current === null) return

    const elapsed = (Date.now() - startTimeRef.current) / 1000
    const spd = elapsed > 0 ? processed / elapsed : 0
    setSpeed(spd)

    const remaining = stats.total - processed
    setEta(spd > 0 ? remaining / spd : null)

    prevProcessedRef.current = processed
  }, [processed, isScraping, stats.total])

  const filtered = useMemo(
    () => filterStatus === 'all' ? results : results.filter((r) => r.status === filterStatus),
    [results, filterStatus]
  )

  const handleExportExcel = () => {
    const done = results.filter((r) => r.status === 'done')
    const rows = done.map((r) => ({
      'En Objekt':      r.enObjekt,
      'Re Name':        r.reName,
      'Re Name2':       r.reName2,
      'Objekt Rechnung':r.objektRechnung,
      'Re Ort':         r.reOrt,
      'Re Hausnummer':  r.reHausnummer,
      'Re Plz':         r.rePlz,
      'Re Strasse':     r.reStrasse,
      'Re Nummer':      r.reNummer,
      'email':          r.emails?.[0] ?? r.email,
      'Telefonnummer':  r.phones?.[0] ?? r.telefonnummer,
      'Tüm E-postalar': (r.emails ?? []).join(', '),
      'Tüm Telefonlar': (r.phones ?? []).join(', '),
      'Kaynak':         r.source,
    }))
    const ws = XLSX.utils.json_to_sheet(rows)
    const wb = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(wb, ws, 'Sonuçlar')
    XLSX.writeFile(wb, `scrape-sonuclari-${new Date().toISOString().slice(0, 10)}.xlsx`)
  }

  if (results.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-gray-400">
        <svg className="w-12 h-12 mb-3 text-gray-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
            d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
        </svg>
        <p className="text-sm">Henüz sonuç yok</p>
        <p className="text-xs mt-1">Şirket seçin ve &quot;Tara&quot; butonuna tıklayın</p>
      </div>
    )
  }

  return (
    <div>
      {/* Toolbar */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-100">
        <div className="flex items-center gap-2">
          {isScraping && (
            <div className="flex items-center gap-2 text-xs text-blue-600">
              <span className="w-3 h-3 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
              Tarama devam ediyor...
            </div>
          )}
          <div className="flex bg-gray-100 rounded-lg p-0.5 gap-0.5">
            {([
              { key: 'all',       label: `Tümü (${stats.total})` },
              { key: 'done',      label: `Bulundu (${stats.done})` },
              { key: 'not_found', label: `Bulunamadı (${stats.not_found})` },
              { key: 'error',     label: `Hata (${stats.error})` },
            ] as const).map(({ key, label }) => (
              <button
                key={key}
                onClick={() => setFilterStatus(key)}
                className={`px-3 py-1 text-xs rounded-md transition-all ${
                  filterStatus === key
                    ? 'bg-white text-gray-800 shadow-sm font-medium'
                    : 'text-gray-500 hover:text-gray-700'
                }`}
              >
                {label}
              </button>
            ))}
          </div>
        </div>

        {stats.done > 0 && (
          <button
            onClick={handleExportExcel}
            className="flex items-center gap-1.5 text-xs text-gray-500 hover:text-gray-700
              border border-gray-200 rounded-lg px-3 py-1.5 hover:bg-gray-50 transition-colors"
          >
            <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
            </svg>
            Excel Dışa Aktar
          </button>
        )}
      </div>

      {/* Progress bar */}
      <div className="px-4 py-2.5 border-b border-gray-50 bg-gray-50/50">
        {/* Row 1: counts + speed/ETA */}
        <div className="flex items-center justify-between text-xs mb-1.5">
          <div className="flex items-center gap-3 text-gray-500">
            <span className="font-semibold text-gray-700">{processed} / {stats.total}</span>
            {stats.done > 0 && <span className="text-green-600">✓ {stats.done} bulundu</span>}
            {stats.not_found > 0 && <span className="text-yellow-500">— {stats.not_found} bulunamadı</span>}
            {stats.error > 0 && <span className="text-red-400">✕ {stats.error} hata</span>}
          </div>
          <div className="flex items-center gap-3 text-gray-400">
            {isScraping && speed > 0 && (
              <span>{speed.toFixed(1)} şirket/sn</span>
            )}
            {isScraping && eta !== null && (
              <span>
                ~{eta >= 60
                  ? `${Math.floor(eta / 60)}d ${Math.round(eta % 60)}sn`
                  : `${Math.round(eta)}sn`} kaldı
              </span>
            )}
            <span className={`font-bold tabular-nums ${isScraping ? 'text-blue-600' : 'text-gray-600'}`}>
              {pct}%
            </span>
          </div>
        </div>

        {/* Progress bar */}
        <div className="h-2 bg-gray-200 rounded-full overflow-hidden">
          <div
            className="h-full rounded-full transition-all duration-300 ease-out"
            style={{
              width: `${pct}%`,
              background: pct === 100
                ? 'linear-gradient(90deg, #10b981, #059669)'
                : 'linear-gradient(90deg, #3b82f6, #6366f1)',
            }}
          />
        </div>
      </div>

      {/* Table */}
      <div className="overflow-x-auto max-h-[500px] overflow-y-auto scrollbar-thin">
        <table className="w-full text-sm">
          <thead className="sticky top-0 z-10">
            <tr className="bg-gray-50 text-gray-500 text-xs border-b border-gray-100">
              <th className="px-4 py-3 text-left font-medium whitespace-nowrap">Durum</th>
              <th className="px-4 py-3 text-left font-medium">Firma Adı</th>
              <th className="px-4 py-3 text-left font-medium whitespace-nowrap">Şehir</th>
              <th className="px-4 py-3 text-left font-medium whitespace-nowrap">PLZ</th>
              <th className="px-4 py-3 text-left font-medium">E-posta</th>
              <th className="px-4 py-3 text-left font-medium whitespace-nowrap">Telefon</th>
              <th className="px-4 py-3 text-left font-medium whitespace-nowrap">Kaynak</th>
              <th className="px-4 py-3 text-left font-medium whitespace-nowrap">Link</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-50">
            {filtered.map((r) => {
              const rowBg =
                r.status === 'done'      ? 'bg-green-50/20' :
                r.status === 'not_found' ? 'bg-yellow-50/20' :
                r.status === 'error'     ? 'bg-red-50/20' : ''
              return (
                <tr key={r.id} className={`hover:bg-gray-50 transition-colors ${rowBg}`}>
                  <td className="px-4 py-2.5 whitespace-nowrap">
                    <StatusBadge status={r.status} />
                    {r.status === 'error' && r.error && (
                      <p className="text-xs text-red-400 mt-0.5">{r.error}</p>
                    )}
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-2">
                      <div className="w-6 h-6 rounded-md bg-gradient-to-br from-indigo-500 to-purple-600
                        flex items-center justify-center text-white text-xs font-bold flex-shrink-0">
                        {r.reName.charAt(0).toUpperCase()}
                      </div>
                      <div className="min-w-0">
                        <p className="font-medium text-gray-800 text-xs truncate max-w-[180px]">{r.reName}</p>
                        {r.reName2 && <p className="text-gray-400 text-xs truncate max-w-[180px]">{r.reName2}</p>}
                      </div>
                    </div>
                  </td>
                  <td className="px-4 py-2.5 text-gray-600 text-xs whitespace-nowrap">
                    {r.reOrt || <span className="text-gray-300">—</span>}
                  </td>
                  <td className="px-4 py-2.5 text-gray-500 text-xs font-mono whitespace-nowrap">
                    {r.rePlz || <span className="text-gray-300">—</span>}
                  </td>
                  <td className="px-4 py-2.5 text-xs">
                    {r.emails && r.emails.length > 0 ? (
                      <div className="flex flex-col gap-0.5">
                        {r.emails.map((email, i) => (
                          <a key={i} href={`mailto:${email}`}
                            className="text-blue-600 hover:underline truncate max-w-[160px] block">
                            {email}
                          </a>
                        ))}
                      </div>
                    ) : r.email ? (
                      <a href={`mailto:${r.email}`} className="text-blue-600 hover:underline">{r.email}</a>
                    ) : (
                      <span className="text-gray-300">—</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-xs">
                    {r.phones && r.phones.length > 0 ? (
                      <div className="flex flex-col gap-0.5">
                        {r.phones.map((p, i) => (
                          <span key={i} className="text-green-700 whitespace-nowrap">{p}</span>
                        ))}
                      </div>
                    ) : r.telefonnummer ? (
                      <span className="text-green-700">{r.telefonnummer}</span>
                    ) : (
                      <span className="text-gray-300">—</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-400 whitespace-nowrap">
                    {r.source || <span className="text-gray-300">—</span>}
                  </td>
                  <td className="px-4 py-2.5 text-xs whitespace-nowrap">
                    <a
                      href={gsLink(r.reName, r.reOrt)}
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
      </div>
    </div>
  )
}

// memo prevents re-render when parent re-renders for unrelated state changes.
const ScrapeResults = memo(ScrapeResultsInner)
export default ScrapeResults
