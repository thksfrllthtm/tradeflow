import { useEffect } from 'react'
import { GoodsItemInput } from '@/shared/types/goods'

// «Отменить» (previous — строки до смены кода) или ошибка (failed).
export type CodeNoticeState = { text: string; failed?: boolean; previous?: GoodsItemInput[] }

type Props = { notice: CodeNoticeState; loading: boolean; onUndo: (previous: GoodsItemInput[]) => void; onClose: () => void }

// Уведомление внизу после смены кода на этапе результата.
export function CodeNotice({ notice, loading, onUndo, onClose }: Props) {
  // Удачное уведомление само скрывается через 8 секунд; ошибка — висит, пока не закроют.
  useEffect(() => {
    if (notice.failed) return
    const timer = window.setTimeout(onClose, 8000)
    return () => window.clearTimeout(timer)
  }, [notice])
  return <div className={notice.failed ? 'code-notice failed' : 'code-notice'} role={notice.failed ? 'alert' : 'status'}>
    <span>{notice.text}</span>
    {notice.previous && <button type="button" className="code-notice-undo" onClick={() => notice.previous && onUndo(notice.previous)} disabled={loading}>Отменить</button>}
    <button type="button" className="code-notice-close" onClick={onClose} aria-label="Закрыть уведомление">×</button>
  </div>
}
