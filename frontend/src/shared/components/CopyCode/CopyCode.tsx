import { ClipboardEvent, useEffect, useState } from 'react'
import { formatCode } from '@/shared/tnved/codes'
import './CopyCode.css'

type Props = { code: string; className?: string }

const digitsOnly = (value: string) => value.replace(/\D/g, '')

// Код ТН ВЭД для чтения с пробелами («6804 30 000 0»), а в буфер — только цифры («6804300000»):
// нажатие копирует код, выделение мышью и Ctrl+C тоже дают код без пробелов.
export function CopyCode({ code, className }: Props) {
  const [copied, setCopied] = useState(false)
  const digits = digitsOnly(code)

  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 1400)
    return () => window.clearTimeout(timer)
  }, [copied])

  async function copy() {
    try {
      await navigator.clipboard.writeText(digits)
      setCopied(true)
    } catch {
      // Буфер обмена недоступен (нет разрешения) — код можно выделить и скопировать вручную.
    }
  }

  // Выделенный текст кода копируется без пробелов.
  function onCopy(event: ClipboardEvent<HTMLElement>) {
    const selected = window.getSelection()?.toString() ?? ''
    if (!selected) return
    event.clipboardData.setData('text/plain', digitsOnly(selected) || selected)
    event.preventDefault()
  }

  return <button type="button" className={['copy-code', copied ? 'copied' : '', className].filter(Boolean).join(' ')} onClick={(event) => { event.stopPropagation(); void copy() }} onCopy={onCopy}
    title={copied ? 'Скопировано' : `Скопировать код без пробелов: ${digits}`} aria-label={`Код ${formatCode(code)}, скопировать`}>
    <span>{formatCode(code)}</span>
    <i aria-hidden="true">{copied ? '✓' : '⧉'}</i>
  </button>
}
