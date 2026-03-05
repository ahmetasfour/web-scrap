import { useState } from 'react'
import { Modal, message } from 'antd'

interface UseDeleteOptions {
  onSuccess?: () => void
  confirmTitle?: string
  confirmContent?: string
}

export function useDelete(
  deleteFn: (id: number) => Promise<void>,
  options: UseDeleteOptions = {}
) {
  const [loading, setLoading] = useState(false)

  const handleDelete = (id: number) => {
    Modal.confirm({
      title: options.confirmTitle ?? 'Silmek istediğinize emin misiniz?',
      content: options.confirmContent ?? 'Bu işlem geri alınamaz.',
      okText: 'Evet, Sil',
      cancelText: 'İptal',
      okType: 'danger',
      onOk: async () => {
        setLoading(true)
        try {
          await deleteFn(id)
          message.success('Başarıyla silindi')
          options.onSuccess?.()
        } catch {
          message.error('Silme işlemi başarısız')
        } finally {
          setLoading(false)
        }
      },
    })
  }

  return { handleDelete, loading }
}
