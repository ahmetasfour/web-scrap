// Company represents one row of the German property management Excel file.
export interface Company {
  id: number
  enObjekt: number
  reName: string
  reName2: string
  objektRechnung: string
  reOrt: string
  reHausnummer: string
  rePlz: string
  reStrasse: string
  reNummer: string
  email: string
  telefonnummer: string
}

export interface ScrapeResult extends Company {
  status: 'pending' | 'scraping' | 'done' | 'not_found' | 'error'
  emails: string[]
  phones: string[]
  source: string
  error: string
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  pageSize: number
}

export interface ApiResponse<T> {
  success: boolean
  data: T
  message?: string
}
