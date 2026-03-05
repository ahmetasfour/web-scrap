import { Company, ScrapeResult } from './index'

function normalizeResult(r: ScrapeResult): ScrapeResult {
  return {
    ...r,
    status:
      r.status === 'done' || r.status === 'not_found' || r.status === 'error'
        ? r.status
        : 'done',
  }
}

/**
 * Scrapes companies via SSE streaming endpoint.
 *
 * UI updates are throttled to ~60 fps: we accumulate all SSE events that
 * arrive within the same 16 ms window and only call onProgress once per
 * animation frame — this prevents dozens of React re-renders per second
 * when 30+ workers finish roughly simultaneously.
 */
export async function scrapeCompanies(
  companies: Company[],
  onProgress: (results: ScrapeResult[]) => void
): Promise<ScrapeResult[]> {
  const accumulated: ScrapeResult[] = companies.map((c) => ({
    ...c,
    status: 'pending' as const,
    emails: [],
    phones: [],
    source: '',
    error: '',
  }))

  const idToIdx = new Map(companies.map((c, i) => [c.id, i]))

  onProgress([...accumulated])

  let response: Response
  try {
    // SSE isteğini Next.js proxy'sini atlayarak doğrudan backend'e gönder
    response = await fetch('http://localhost:8080/api/scrape/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ companies }),
    })
  } catch {
    return accumulated.map((r) => ({ ...r, status: 'error' as const, error: 'Bağlantı hatası' }))
  }

  if (!response.ok || !response.body) {
    return accumulated.map((r) => ({
      ...r,
      status: 'error' as const,
      error: `Sunucu hatası (${response.status})`,
    }))
  }

  // --- 60fps throttle ---
  const THROTTLE_MS = 16
  let lastFlush = 0
  let dirty = false

  const flush = () => {
    if (!dirty) return
    onProgress([...accumulated])
    lastFlush = Date.now()
    dirty = false
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop()!

    for (const line of lines) {
      if (!line.startsWith('data: ')) continue
      const raw = line.slice(6).trim()
      if (!raw || raw === '{}') continue

      try {
        const parsed = JSON.parse(raw) as Record<string, unknown>
        if (parsed.type === 'total') continue

        const result = parsed as unknown as ScrapeResult
        const idx = idToIdx.get(result.id)
        if (idx !== undefined) {
          accumulated[idx] = normalizeResult(result)
          dirty = true
        }
      } catch {
        // ignore malformed SSE frames
      }
    }

    // Flush at most once per ~16ms (60fps).
    if (dirty && Date.now() - lastFlush >= THROTTLE_MS) {
      flush()
    }
  }

  // Always emit final state.
  dirty = true
  flush()

  return accumulated
}

export async function getScrapeHistory(): Promise<ScrapeResult[]> {
  const res = await fetch('/api/scrape/history')
  if (!res.ok) return []
  return res.json()
}
