import { MouseEvent, useEffect, useState } from 'react'
import { CopyIcon } from './CopyIcon'
import './CopyCode.css'

// Кнопка-значок «копировать» рядом с текстом (наименование, обоснование, заметка): копирует текст целиком,
// на 1,4 с показывает зелёную галочку. Щелчок не открывает строку таблицы, в которой стоит кнопка.
export function CopyButton({ text, label }: { text: string; label: string }) {
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 1400)
    return () => window.clearTimeout(timer)
  }, [copied])
  async function copy(event: MouseEvent) {
    event.stopPropagation()
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
    } catch {
      // Буфер обмена недоступен — текст можно выделить мышью.
    }
  }
  return <button type="button" className={copied ? 'copy-btn copied' : 'copy-btn'} onClick={(event) => void copy(event)}
    title={copied ? 'Скопировано' : `Скопировать ${label}`} aria-label={`Скопировать ${label}`}>
    <CopyIcon copied={copied} />
  </button>
}
