import { useEffect } from 'react'
import { useRegisterSW } from 'virtual:pwa-register/react'
import { useToast } from '../contexts/ToastContext'

/**
 * PWAUpdater 驅動 Service Worker 的更新提示。
 *
 * registerType 為 'prompt'：偵測到新版時不自動 reload（避免播放中被打斷），
 * 改用既有 Toast 推一則持久的「有新版本，點此更新」，使用者點擊才呼叫
 * updateServiceWorker(true) 套用新版並 reload。本身不渲染任何 UI。
 */
export default function PWAUpdater() {
  const toast = useToast()
  const {
    needRefresh: [needRefresh],
    updateServiceWorker,
  } = useRegisterSW()

  useEffect(() => {
    if (!needRefresh) return
    toast.info('有新版本可用', {
      persist: true,
      action: {
        label: '更新',
        onClick: () => {
          void updateServiceWorker(true)
        },
      },
    })
  }, [needRefresh, toast, updateServiceWorker])

  return null
}
