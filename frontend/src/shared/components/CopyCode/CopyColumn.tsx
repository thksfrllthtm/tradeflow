import { useEffect, useState } from 'react'
import { CopyIcon } from './CopyIcon'

// Кнопка в заголовке: колонка в буфер по всем строкам по порядку, чтобы вставить в Excel столбцом.
export function CopyColumn({ label, text, html }: { label: string; text: () => string; html?: () => string }) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 1400)
    return () => window.clearTimeout(timer)
  }, [copied])

  // Через событие copy, а не navigator.clipboard.write: тот очищает HTML и выбрасывает xmlns:x, без которого Excel не читает x:num.
  function copy() {
    const onCopy = (event: ClipboardEvent) => {
      event.preventDefault()
      event.clipboardData?.setData('text/plain', text())
      if (html) event.clipboardData?.setData('text/html', html())
    }
    document.addEventListener('copy', onCopy)
    try {
      if (document.execCommand('copy')) setCopied(true)
    } finally {
      document.removeEventListener('copy', onCopy)
    }
  }

  return <button type="button" className={copied ? 'rs-copy-col copied' : 'rs-copy-col'} onClick={copy}
    title={copied ? 'Скопировано' : `Скопировать колонку «${label}» для вставки в Excel`} aria-label={`Скопировать колонку ${label}`}>
    <CopyIcon copied={copied} size={12} />
  </button>
}
