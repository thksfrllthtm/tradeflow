import { ClipboardEvent, useEffect, useState } from 'react'
import { formatCode } from '@/shared/tnved/codes'
import { CopyIcon } from './CopyIcon'
import './CopyCode.css'

type Props = { code: string; className?: string; number?: boolean }

const digitsOnly = (value: string) => value.replace(/\D/g, '')

// Код ТН ВЭД для чтения с пробелами («6804 30 000 0»), а в буфер — только цифры («6804300000»):
// нажатие копирует код, выделение мышью и Ctrl+C тоже дают код без пробелов.
// number — сумма («4 915 199,68»): показывается как есть, в буфер без пробелов («4915199,68»), для Excel.
export function CopyCode({ code, className, number }: Props) {
  const [copied, setCopied] = useState(false)
  const clean = (value: string) => number ? value.replace(/\s/g, '') : digitsOnly(value)
  const digits = clean(code)
  const shown = number ? code : formatCode(code)

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
    event.clipboardData.setData('text/plain', clean(selected) || selected)
    event.preventDefault()
  }

  return <button type="button" className={['copy-code', number ? 'number' : '', copied ? 'copied' : '', className].filter(Boolean).join(' ')} onClick={(event) => { event.stopPropagation(); void copy() }} onCopy={onCopy}
    title={copied ? 'Скопировано' : `Скопировать без пробелов: ${digits}`} aria-label={`${number ? '' : 'Код '}${shown}, скопировать`}>
    <i className="copy-code-icon"><CopyIcon copied={copied} size={13} /></i>
    <span>{shown}</span>
  </button>
}
