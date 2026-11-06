import { KeyboardEvent, RefObject, useRef, useState } from 'react'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { priceDiffers } from '../../lib/priceChanges'
import { displayNumber } from '../../lib/resultValues'

type Options = {
  result: GoodsCalculationResult
  items: GoodsItemInput[]
  // Строки с ценой, оценённой ИИ: их цена не считается правкой.
  notes: Record<number, string>
  sirMode: boolean
  tableRef: RefObject<HTMLTableElement | null>
  onPriceChange: (index: number, value: string) => void
  onPriceBlur: (index: number, value: string) => void
  // Фокус в поле цены выбирает строку и ячейку (номер ячейки в строке).
  onFocusCell: (index: number, cellIndex: number | null) => void
}

// Поле цены строки — одно и то же в таблице и в панели.
export function usePriceInput({ result, items, notes, sirMode, tableRef, onPriceChange, onPriceBlur, onFocusCell }: Options) {
  // Цена, которую сейчас вводят: пока поле в фокусе, показываем ровно её (в том числе пустую),
  // иначе стёртое поле сразу заполнилось бы рассчитанной ценой. Пересчёт — при выходе из поля.
  const [draftPrices, setDraftPrices] = useState<Record<number, string>>({})
  // Цена до начала ввода (для Esc) и строка, выход из которой не должен пересчитывать.
  const editStartRef = useRef<{ index: number; value: string } | null>(null)
  const skipBlurRef = useRef<number | null>(null)
  const priceOf = (index: number) => draftPrices[index] ?? (result.items[index]?.pricePerPair || result.items[index]?.pairPrice || items[index]?.pricePerPair || '')

  // Excel-привычка в таблице: Enter / ↓ — следующая цена, Shift+Enter / ↑ — предыдущая, Esc — вернуть цену.
  // В панели Enter применяет цену.
  function priceKeyDown(event: KeyboardEvent<HTMLInputElement>, index: number, inTable: boolean) {
    if (event.key === 'Escape') {
      const start = editStartRef.current
      if (start?.index === index && draftPrices[index] !== undefined) {
        event.preventDefault()
        onPriceChange(index, start.value)
        setDraftPrices(({ [index]: _done, ...rest }) => rest)
        skipBlurRef.current = index
        event.currentTarget.blur()
      }
      return
    }
    if (!inTable) {
      if (event.key === 'Enter') { event.preventDefault(); event.currentTarget.blur() }
      return
    }
    const back = event.key === 'ArrowUp' || (event.key === 'Enter' && event.shiftKey)
    if (event.key !== 'Enter' && event.key !== 'ArrowDown' && !back) return
    event.preventDefault()
    const inputs = Array.from(tableRef.current?.querySelectorAll<HTMLInputElement>('input.rs-price:not(:disabled)') ?? [])
    const next = inputs[inputs.indexOf(event.currentTarget) + (back ? -1 : 1)]
    if (next) { next.focus(); next.select() } else event.currentTarget.blur()
  }

  function priceInput(index: number, inTable: boolean, label: string) {
    const item = result.items[index]
    const source = items[index]
    const price = priceOf(index)
    const edited = !item.excluded && !notes[index] && priceDiffers(source?.sourcePricePerPair, price)
    return <input
      className={[inTable ? 'rs-price' : 'rp-price', edited ? 'edited' : ''].filter(Boolean).join(' ')}
      inputMode="decimal"
      value={price.replace('.', ',')}
      placeholder="—"
      disabled={item.excluded || (sirMode && !inTable)}
      title={edited ? `В Excel: ${displayNumber(source?.sourcePricePerPair)} USD` : undefined}
      onFocus={(event) => {
        editStartRef.current = { index, value: price }
        // Поле в таблице — рамка выбранной ячейки на цене (щелчок по полю до строки не доходит, фокус — с клавиатуры).
        onFocusCell(index, event.currentTarget.closest('td')?.cellIndex ?? null)
      }}
      onClick={(event) => event.stopPropagation()}
      onChange={(event) => { const value = event.target.value.replace(',', '.'); setDraftPrices((current) => ({ ...current, [index]: value })); onPriceChange(index, value) }}
      onBlur={() => {
        if (skipBlurRef.current === index) { skipBlurRef.current = null; return }
        const value = draftPrices[index]
        if (value === undefined) return
        setDraftPrices(({ [index]: _done, ...rest }) => rest)
        // Пустое поле — убрать цену: строка считается без неё (по ИТП или с пометкой «Укажите цену»).
        onPriceBlur(index, value.trim())
      }}
      onKeyDown={(event) => priceKeyDown(event, index, inTable)}
      aria-label={label}
    />
  }

  return { priceOf, priceInput }
}
