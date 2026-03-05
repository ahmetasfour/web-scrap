import Link from 'next/link'
import { Button, Result } from 'antd'

export default function NotFound() {
  return (
    <div className="min-h-screen flex items-center justify-center">
      <Result
        status="404"
        title="404"
        subTitle="Aradığınız sayfa bulunamadı."
        extra={
          <Link href="/">
            <Button type="primary">Ana Sayfaya Dön</Button>
          </Link>
        }
      />
    </div>
  )
}
