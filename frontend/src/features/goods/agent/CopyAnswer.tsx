import { createContext, ReactNode, useContext, useEffect, useState } from 'react'
import { CopyIcon } from '@/shared/components/CopyCode/CopyIcon'
import '@/shared/components/CopyCode/CopyCode.css'

// Доп. действия в той же строке, что и копирование (например, «Поделиться с сотрудником» у последнего ответа чата).
export const AnswerActions = createContext<ReactNode>(null)

// Внизу ответа ИИ: скопировать ответ текстом, чтобы отправить в WhatsApp или почтой. «Коротко» — только
// главное (код, наименование, ставки), «Подробно» — с обоснованием и примечаниями.
export function CopyAnswer({ short, full }: { short: string; full: string }) {
  const extra = useContext(AnswerActions)
  const [copied, setCopied] = useState<'short' | 'full' | null>(null)
  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(null), 1600)
    return () => window.clearTimeout(timer)
  }, [copied])
  async function copy(kind: 'short' | 'full') {
    try {
      await navigator.clipboard.writeText(kind === 'short' ? short : full)
      setCopied(kind)
    } catch {
      // Буфер обмена недоступен — текст ответа можно выделить мышью.
    }
  }
  if (!short && !full && !extra) return null
  const button = (kind: 'short' | 'full', label: string, title: string) => <button type="button" className={copied === kind ? 'copied' : undefined}
    onClick={() => void copy(kind)} title={title} aria-label={`Скопировать ${label.toLowerCase()}`}><CopyIcon copied={copied === kind} />{copied === kind ? 'Скопировано' : label}</button>
  return <div className="copy-answer" role="group" aria-label="Действия с ответом">
    {short && button('short', 'Коротко', 'Скопировать только главное: код, наименование, ставки')}
    {full && button('full', 'Подробно', 'Скопировать с обоснованием, примечаниями и ТРОИС')}
    {extra}
  </div>
}
