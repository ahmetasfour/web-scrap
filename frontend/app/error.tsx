'use client'

import { useEffect } from 'react'
import { Button, Result } from 'antd'

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    console.error(error)
  }, [error])

  return (
    <div className="min-h-screen flex items-center justify-center">
      <Result
        status="500"
        title="Bir hata oluştu"
        subTitle="Beklenmeyen bir hata meydana geldi. Lütfen tekrar deneyin."
        extra={
          <Button type="primary" onClick={reset}>
            Tekrar Dene
          </Button>
        }
      />
    </div>
  )
}
