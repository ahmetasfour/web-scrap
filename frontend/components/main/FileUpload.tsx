'use client'

import { useState, useRef, useCallback } from 'react'
import * as XLSX from 'xlsx'
import { Company } from '@/data'

interface FileUploadProps {
  onCompaniesLoaded: (companies: Company[]) => void
}

// Maps possible Excel header spellings → our field keys.
const HEADER_MAP: Record<string, keyof Company> = {
  'en objekt': 'enObjekt',
  'enobjekt': 'enObjekt',
  're name': 'reName',
  'rename': 'reName',
  'firmaadı': 'reName',
  'firma adı': 'reName',
  'name': 'reName',
  're name2': 'reName2',
  'rename2': 'reName2',
  'objekt rechnung': 'objektRechnung',
  'objektrechnung': 'objektRechnung',
  're ort': 'reOrt',
  'reort': 'reOrt',
  'ort': 'reOrt',
  'şehir': 'reOrt',
  're hausnummer': 'reHausnummer',
  'rehausnummer': 'reHausnummer',
  'hausnummer': 'reHausnummer',
  're plz': 'rePlz',
  'replz': 'rePlz',
  'plz': 'rePlz',
  'posta kodu': 'rePlz',
  're strasse': 'reStrasse',
  'restrasse': 'reStrasse',
  'strasse': 'reStrasse',
  'sokak': 'reStrasse',
  're nummer': 'reNummer',
  'renummer': 'reNummer',
  'nummer': 'reNummer',
  'email': 'email',
  'e-posta': 'email',
  'eposta': 'email',
  'telefonnummer': 'telefonnummer',
  'telefon': 'telefonnummer',
  'phone': 'telefonnummer',
  'tel': 'telefonnummer',
}

export default function FileUpload({ onCompaniesLoaded }: FileUploadProps) {
  const [isDragging, setIsDragging] = useState(false)
  const [fileName, setFileName] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const parseExcel = (file: File) => {
    setLoading(true)
    setError('')
    const reader = new FileReader()

    reader.onload = (e) => {
      try {
        const data = new Uint8Array(e.target!.result as ArrayBuffer)
        const workbook = XLSX.read(data, { type: 'array' })
        const sheet = workbook.Sheets[workbook.SheetNames[0]]
        const rows = XLSX.utils.sheet_to_json<(string | number)[]>(sheet, { header: 1 })

        if (rows.length < 2) {
          setError('Dosya boş veya yeterli veri içermiyor.')
          setLoading(false)
          return
        }

        // Build column index map from headers.
        const rawHeaders = rows[0].map((h) => String(h || '').toLowerCase().trim().replace(/\s+/g, ' '))
        const colIdx: Partial<Record<keyof Company, number>> = {}
        rawHeaders.forEach((h, i) => {
          const mapped = HEADER_MAP[h]
          if (mapped) colIdx[mapped] = i
        })

        const str = (row: (string | number)[], key: keyof Company): string => {
          const idx = colIdx[key]
          return idx !== undefined ? String(row[idx] ?? '').trim() : ''
        }
        const num = (row: (string | number)[], key: keyof Company): number => {
          const idx = colIdx[key]
          if (idx === undefined) return 0
          const v = Number(row[idx])
          return isNaN(v) ? 0 : v
        }

        const companies: Company[] = rows
          .slice(1)
          .filter((row) => row.some((cell) => cell !== '' && cell !== undefined && cell !== null))
          .map((row, i) => ({
            id: i + 1,
            enObjekt: num(row, 'enObjekt'),
            reName: str(row, 'reName') || `Şirket ${i + 1}`,
            reName2: str(row, 'reName2'),
            objektRechnung: str(row, 'objektRechnung'),
            reOrt: str(row, 'reOrt'),
            reHausnummer: str(row, 'reHausnummer'),
            rePlz: str(row, 'rePlz'),
            reStrasse: str(row, 'reStrasse'),
            reNummer: str(row, 'reNummer'),
            email: str(row, 'email'),
            telefonnummer: str(row, 'telefonnummer'),
          }))

        if (companies.length === 0) {
          setError('Dosyada geçerli veri bulunamadı.')
          setLoading(false)
          return
        }

        setFileName(file.name)
        onCompaniesLoaded(companies)
      } catch {
        setError('Dosya okunamadı. Lütfen geçerli bir Excel dosyası (.xlsx veya .xls) yükleyin.')
      } finally {
        setLoading(false)
      }
    }

    reader.readAsArrayBuffer(file)
  }

  const handleFile = (file: File | null | undefined) => {
    if (!file) return
    const ext = file.name.split('.').pop()?.toLowerCase()
    if (!['xlsx', 'xls', 'csv'].includes(ext ?? '')) {
      setError('Lütfen bir Excel (.xlsx, .xls) veya CSV dosyası yükleyin.')
      return
    }
    parseExcel(file)
  }

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setIsDragging(false)
    handleFile(e.dataTransfer.files[0])
  }, [])

  const onDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    setIsDragging(true)
  }

  return (
    <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-6">
      <div className="flex items-center gap-2 mb-4">
        <svg className="w-5 h-5 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
            d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
        </svg>
        <h2 className="text-sm font-semibold text-gray-700">Şirket Dosyası Yükle</h2>
      </div>

      <div
        onDrop={onDrop}
        onDragOver={onDragOver}
        onDragLeave={() => setIsDragging(false)}
        onClick={() => inputRef.current?.click()}
        className={`relative border-2 border-dashed rounded-xl p-8 text-center cursor-pointer transition-all ${
          isDragging
            ? 'border-blue-400 bg-blue-50'
            : fileName
            ? 'border-green-300 bg-green-50'
            : 'border-gray-200 hover:border-blue-300 hover:bg-gray-50'
        }`}
      >
        <input
          ref={inputRef}
          type="file"
          accept=".xlsx,.xls,.csv"
          className="hidden"
          onChange={(e) => handleFile(e.target.files?.[0])}
        />

        {loading ? (
          <div className="flex flex-col items-center gap-3">
            <div className="w-10 h-10 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
            <p className="text-sm text-gray-500">Dosya okunuyor...</p>
          </div>
        ) : fileName ? (
          <div className="flex flex-col items-center gap-2">
            <div className="w-12 h-12 bg-green-100 rounded-xl flex items-center justify-center">
              <svg className="w-6 h-6 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                  d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <p className="text-sm font-medium text-green-700">{fileName}</p>
            <p className="text-xs text-gray-400">Değiştirmek için tıklayın</p>
          </div>
        ) : (
          <div className="flex flex-col items-center gap-3">
            <div className="w-14 h-14 bg-gray-100 rounded-2xl flex items-center justify-center">
              <svg className="w-7 h-7 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                  d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
            </div>
            <div>
              <p className="text-sm font-medium text-gray-700">
                Dosyayı buraya sürükleyin veya{' '}
                <span className="text-blue-600">seçmek için tıklayın</span>
              </p>
              <p className="text-xs text-gray-400 mt-1">Desteklenen formatlar: .xlsx, .xls, .csv</p>
            </div>
          </div>
        )}
      </div>

      {error && (
        <div className="mt-3 flex items-center gap-2 text-red-600 text-xs bg-red-50 px-3 py-2 rounded-lg">
          <svg className="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
              d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          {error}
        </div>
      )}

      <div className="mt-4 p-3 bg-blue-50 rounded-xl">
        <p className="text-xs font-medium text-blue-700 mb-1">Desteklenen Excel sütunları:</p>
        <div className="flex flex-wrap gap-1.5 text-xs text-blue-600 font-mono">
          {['En Objekt', 'Re Name', 'Re Ort', 'Re Plz', 'Re Strasse', 'email', 'Telefonnummer'].map((col) => (
            <span key={col} className="bg-blue-100 px-2 py-0.5 rounded">{col}</span>
          ))}
        </div>
      </div>
    </div>
  )
}
