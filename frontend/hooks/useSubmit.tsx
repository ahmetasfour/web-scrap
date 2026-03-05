import { useState } from 'react'
import { message } from 'antd'

interface UseSubmitOptions<T> {
  onSuccess?: (data: T) => void
  onError?: (err: Error) => void
  successMessage?: string
  errorMessage?: string
}

export function useSubmit<T, P = unknown>(
  submitFn: (payload: P) => Promise<T>,
  options: UseSubmitOptions<T> = {}
) {
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (payload: P) => {
    setLoading(true)
    try {
      const data = await submitFn(payload)
      if (options.successMessage) message.success(options.successMessage)
      options.onSuccess?.(data)
      return data
    } catch (err) {
      const error = err instanceof Error ? err : new Error('Bilinmeyen hata')
      if (options.errorMessage) message.error(options.errorMessage)
      options.onError?.(error)
      throw error
    } finally {
      setLoading(false)
    }
  }

  return { handleSubmit, loading }
}
