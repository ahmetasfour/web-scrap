import useSWR, { SWRConfiguration } from 'swr'

const fetcher = (url: string) =>
  fetch(url).then((res) => {
    if (!res.ok) throw new Error('İstek başarısız')
    return res.json()
  })

export function useRequest<T>(url: string | null, options?: SWRConfiguration) {
  const { data, error, isLoading, mutate } = useSWR<T>(url, fetcher, {
    revalidateOnFocus: false,
    ...options,
  })

  return {
    data,
    error,
    isLoading,
    mutate,
  }
}
