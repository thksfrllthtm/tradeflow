import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { KeyboardEvent, useCallback, useEffect, useRef, useState } from 'react'
import { GoodsCalculationResult, GoodsItemInput, ItpSaved, SirParams, SirResult } from '@/shared/types/goods'
import { moneyText, weightText } from '@/shared/lib/rounding'
import { BudgetControls, BudgetNote, BudgetStatus } from '../BudgetTool/BudgetTool'
import { BudgetDistributionState } from '../../hooks/useBudgetDistribution'
import { useSyncedHorizontalScroll } from '../../hooks/useSyncedHorizontalScroll'
import { changedFromExcel, changedFromRows, priceDiffers } from '../../lib/priceChanges'
import { CalculatorActivity } from '../../types'
import { CalculationErrors } from './CalculationErrors'
import { AntiDumpingChooser, AntiDumpingInfo, antiDumpingTone } from './AntiDumpingChooser'
import { ResultSummary } from './ResultSummary'
import { SirControls } from './SirControls'
import { SirCompareModal } from './SirCompareModal'
import { plural } from './errorGroups'
import { displayNumber, dutyRateText, excelColumn, excelColumnHtml, textColumn, hasAntiDumping, itemDisplayName, parseCalculatorNumber, RowState, rowState, rowTotals } from './resultValues'
import '../../goods.css'
import './ResultStep.css'

type Props = {
  result: GoodsCalculationResult
  items: GoodsItemInput[]
  loading: boolean
  activity: CalculatorActivity
  storageNotice: string
  saveDraft: () => void
  exportExcel: () => void
  onBackToReview: () => void
  onPriceChange: (index: number, value: string) => void
  onPriceBlur: (index: number, value: string) => void
  // Вариант антидемпинга строки ('' — как по стране, 'none' — не начислять); строка пересчитывается.
  onAntiDumpingChoice: (index: number, key: string) => void
  // Таможенная стоимость по инвойсу или по СУР и параметры СУР; смета пересчитывается.
  sir: SirParams
  onSirChange: (patch: SirParams) => void
  // Пробный расчёт с другими параметрами СУР — для окна сравнения; смета не меняется.
  previewSir: (patch: SirParams) => Promise<GoodsCalculationResult>
  // «Сверка по ИТП», сохраняемая в смете.
  itp?: ItpSaved
  onItpChange: (value: ItpSaved | undefined) => void
  onRestoreInitial: () => void
  // Откат последнего подбора под бюджет или сброса цен.
  onUndoPrices: () => void
  canUndoPrices: boolean
  budget: BudgetDistributionState
}

type Density = 'full' | 'compact'
const DENSITY_KEY = 'tradeflow.result-density'
// Выбор сотрудника запоминается; пока его нет — на низком экране (ноутбук) компактно.
const storedDensity = (): Density => {
  try {
    const saved = localStorage.getItem(DENSITY_KEY)
    if (saved === 'compact' || saved === 'full') return saved
  } catch { /* без памяти — по высоте окна */ }
  return window.innerHeight < 900 ? 'compact' : 'full'
}

const positive = (value: unknown) => (parseCalculatorNumber(value as string) ?? 0) > 0

const STATE_LABEL: Record<RowState, string> = { ok: 'Рассчитано', bad: 'Ошибка', warn: 'Проверить', off: 'Исключена' }

// Значок состояния строки: форма отличается, а не только цвет (✓, !, ?, –).
function StateIcon({ state }: { state: RowState }) {
  return <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
    {state === 'ok' && <path d="M2.8 6.3 5 8.4l4.2-4.6" />}
    {state === 'bad' && <><path d="M6 2.8v4" /><circle cx="6" cy="9.2" r=".5" fill="currentColor" /></>}
    {state === 'warn' && <><path d="M4.4 4.4a1.7 1.7 0 1 1 2.4 1.5c-.5.3-.8.6-.8 1.2v.2" /><circle cx="6" cy="9.4" r=".5" fill="currentColor" /></>}
    {state === 'off' && <path d="M3.4 6h5.2" />}
  </svg>
}

// Кнопка в заголовке: колонка в буфер по всем строкам по порядку, чтобы вставить в Excel столбцом.
function CopyColumn({ label, text, html }: { label: string; text: () => string; html?: () => string }) {
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
    {copied ? '✓' : '⧉'}
  </button>
}

const WEIGHT_LABEL = { net: 'нетто', gross: 'брутто' }

// Как посчитана ТС строки по СУР — подсказка ячейки ТС.
function sirTitle(sir: SirResult) {
  const discount = Number(sir.discountPercent) > 0 ? ` − ${displayNumber(sir.discountPercent)} % = ${displayNumber(sir.pricePerKgUsd)}` : ''
  return `ТС по СУР (${sir.country}, профиль ${sir.profile}): ${displayNumber(sir.priceUsd)} USD/кг${discount} × ${displayNumber(sir.weightKg)} кг ${WEIGHT_LABEL[sir.weight] ?? sir.weight} = ${displayNumber(sir.valueUsd)} USD, плюс доставка`
}

// Этап 3: полоса итогов, панель цен, фильтр «требуют внимания» и таблица на всю оставшуюся высоту.
// Состояние строки — в закреплённой левой зоне рядом с №, причина — строкой под названием.
export function ResultStep({
  result,
  items,
  loading,
  activity,
  storageNotice,
  saveDraft,
  exportExcel,
  onBackToReview,
  onPriceChange,
  onPriceBlur,
  onAntiDumpingChoice,
  sir,
  onSirChange,
  previewSir,
  itp,
  onItpChange,
  onRestoreInitial,
  onUndoPrices,
  canUndoPrices,
  budget,
}: Props) {
  // Цена, которую сейчас вводят: пока поле в фокусе, показываем ровно её (в том числе пустую),
  // иначе стёртое поле сразу заполнилось бы рассчитанной ценой. Пересчёт — при выходе из поля.
  const [draftPrices, setDraftPrices] = useState<Record<number, string>>({})
  const [filter, setFilter] = useState<'all' | 'attention'>('all')
  const [density, setDensity] = useState<Density>(storedDensity)
  const [expanded, setExpanded] = useState<number[]>([])
  const [flashRow, setFlashRow] = useState<number | null>(null)
  const tableRef = useRef<HTMLTableElement>(null)
  // Цена до начала ввода (для Esc) и строка, выход из которой не должен пересчитывать.
  const editStartRef = useRef<{ index: number; value: string } | null>(null)
  const skipBlurRef = useRef<number | null>(null)
  const { tableWrapRef, topScrollRef, syncHorizontalScroll } = useSyncedHorizontalScroll(true)

  const checkTotals = rowTotals(result)
  const antiDumping = hasAntiDumping(result)
  // Колонка доставки нужна, только если доставка есть.
  const delivery = positive(result.totals.totalDeliveryUsd) || result.items.some((item) => positive(item.deliveryUsd))
  // По СУР цены инвойса на ТС не влияют: подбирать их под бюджет бессмысленно.
  const sirMode = sir.customsValueMode === 'sir'
  const includedRows = result.items.filter((item) => !item.excluded)
  const sirRows = includedRows.filter((item) => item.customsBasis === 'sir').length
  // По СУР колонки цены инвойса заменяются колонками, из которых считается ТС: вес, СУР, СУР со снижением, ТС в USD.
  const sirGross = sir.sirWeight === 'gross'
  const sirDiscount = Number(sir.sirDiscountPercent || 0)
  const sirValueUsd = includedRows.reduce((total, item) => total + (parseCalculatorNumber(item.sir ? item.sir.valueUsd : item.costUsd) ?? 0), 0)
  const [compareOpen, setCompareOpen] = useState(false)
  const closeCompare = useCallback(() => setCompareOpen(false), [])

  const rows = result.items.map((item, index) => {
    const state = rowState(item)
    const antiDumpingInfo = item.directorySnapshot?.antiDumping as AntiDumpingInfo | undefined
    const adAsk = !item.excluded && antiDumpingTone(antiDumpingInfo) === 'ask'
    return { item, index, state, antiDumpingInfo, adAsk, attention: state === 'bad' || state === 'warn' || adAsk }
  })
  const counts = {
    bad: rows.filter((row) => row.state === 'bad').length,
    warn: rows.filter((row) => row.state === 'warn').length,
    ad: rows.filter((row) => row.adAsk).length,
    attention: rows.filter((row) => row.attention).length,
  }
  const visible = filter === 'attention' ? rows.filter((row) => row.attention) : rows
  // Копируются все строки, даже при фильтре «требуют внимания»: иначе в Excel значения съедут.
  const copyCol = (label: string, get: (item: GoodsCalculationResult['items'][number], index: number) => string | undefined) =>
    <CopyColumn label={label} text={() => excelColumn(result.items.map(get))} html={() => excelColumnHtml(result.items.map(get))} />
  // Код — только цифры, как у CopyCode.
  const copyNames = <CopyColumn label="Товар" text={() => textColumn(result.items.map((item, index) => itemDisplayName(item, items[index])))} />
  const copyCodes = <CopyColumn label="Код ТН ВЭД" text={() => textColumn(result.items.map((item, index) => (item.code || items[index]?.code || '').replace(/\D/g, '')))} />
  const changed = changedFromExcel(items, result)
  // Ширина колонки № — по числу цифр: от неё зависит отступ закреплённой колонки «Товар».
  const numWidth = `${String(result.items.length).length * 8 + 46}px`

  useEffect(() => {
    if (flashRow === null) return
    const timer = window.setTimeout(() => setFlashRow(null), 1800)
    return () => window.clearTimeout(timer)
  }, [flashRow])

  // Ctrl+S — сохранить сразу, а не ждать автосохранения.
  const saveRef = useRef(saveDraft)
  saveRef.current = saveDraft
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 's') {
        event.preventDefault()
        saveRef.current()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // Запоминается только явный выбор сотрудника.
  function chooseDensity(value: Density) {
    setDensity(value)
    try { localStorage.setItem(DENSITY_KEY, value) } catch { /* без памяти — выбор на эту сессию */ }
  }

  function toggleExpanded(index: number) {
    setExpanded((current) => current.includes(index) ? current.filter((value) => value !== index) : [...current, index])
  }

  // Номер строки из плашки ошибок: прокрутить к строке, подсветить и поставить курсор в цену.
  function showRow(rowNumber: number) {
    const index = rowNumber - 1
    if (filter === 'attention' && !rows[index]?.attention) setFilter('all')
    setFlashRow(index)
    window.requestAnimationFrame(() => {
      const row = document.getElementById(`rs-row-${index}`)
      row?.scrollIntoView({ block: 'center' })
      row?.querySelector<HTMLInputElement>('input.rs-price:not(:disabled)')?.focus({ preventScroll: true })
    })
  }

  // Excel-привычка: Enter / ↓ — следующая цена, Shift+Enter / ↑ — предыдущая, Esc — вернуть цену.
  function priceKeyDown(event: KeyboardEvent<HTMLInputElement>, index: number) {
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
    const back = event.key === 'ArrowUp' || (event.key === 'Enter' && event.shiftKey)
    if (event.key !== 'Enter' && event.key !== 'ArrowDown' && !back) return
    event.preventDefault()
    const inputs = Array.from(tableRef.current?.querySelectorAll<HTMLInputElement>('input.rs-price:not(:disabled)') ?? [])
    const next = inputs[inputs.indexOf(event.currentTarget) + (back ? -1 : 1)]
    if (next) { next.focus(); next.select() } else event.currentTarget.blur()
  }

  function backToReview() {
    const count = changed.length
    const bulk = canUndoPrices || Object.keys(budget.notes).length > 0
    if ((count > 0 || bulk) && !window.confirm(count > 0
      ? `Цены этапа «Результат» вернутся к ценам из Excel: изменено ${count} ${plural(count, ['цена', 'цены', 'цен'])}. Перейти к проверке строк?`
      : 'Цены подбора под бюджет сбросятся. Перейти к проверке строк?')) return
    onBackToReview()
  }

  function restoreInitial() {
    const count = new Set([...changedFromRows(items, result), ...Object.keys(budget.notes).map(Number)]).size
    if (count > 0 && !window.confirm(`Вернуть цены строк сметы? Изменится ${count} ${plural(count, ['цена', 'цены', 'цен'])} после подбора под бюджет. Вернуть их можно кнопкой «Отменить».`)) return
    onRestoreInitial()
  }

  // Перед выгрузкой — предупредить о неполном итоге и невыбранном антидемпинге.
  function exportChecked() {
    const notes = [
      result.totals.calculatedCount < result.totals.totalCount && `рассчитано ${result.totals.calculatedCount} из ${result.totals.totalCount} строк — итог неполный`,
      counts.ad > 0 && `антидемпинг не выбран: ${counts.ad} ${plural(counts.ad, ['строка', 'строки', 'строк'])}`,
    ].filter(Boolean)
    if (notes.length > 0 && !window.confirm(`Перед выгрузкой: ${notes.join('; ')}. Выгрузить Excel всё равно?`)) return
    exportExcel()
  }

  const columns = 10 + (antiDumping ? 1 : 0) + (delivery ? 1 : 0) + (sirMode && sirDiscount > 0 ? 1 : 0)

  return (
    <div className={`goods-step-page rs-step${density === 'compact' ? ' compact' : ''}`}>
      <CalculationErrors result={result} onBackToReview={backToReview} onShowRow={showRow} />

      <ResultSummary result={result} loading={loading} activity={activity} storageNotice={storageNotice} delivery={delivery} saveDraft={saveDraft} exportExcel={exportChecked} itp={itp} onItpChange={onItpChange} />

      <section className="rv-card rs-card" aria-label="Строки расчёта">
        {/* Одна полоса: как считать ТС, подбор под бюджет (по инвойсу) или условия СУР, откат цен, сравнение. */}
        <SirControls sir={sir} sirRows={sirRows} totalRows={includedRows.length} disabled={loading} onChange={onSirChange} onCompare={() => setCompareOpen(true)}
          invoiceTools={<BudgetControls budget={budget} disabled={loading} />}
          priceTools={<>
            <button type="button" className="rs-icon" onClick={onUndoPrices} disabled={!canUndoPrices || loading} title="Отменить: вернуть цены до последнего подбора или сброса" aria-label="Отменить последний подбор цен">
              <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><path d="M5.5 3.5 2.5 6.5l3 3" /><path d="M2.8 6.5h6.7a4 4 0 0 1 0 8H7" /></svg>
            </button>
            <button type="button" className="rs-icon" onClick={restoreInitial} disabled={loading} title="Сбросить цены: вернуть цены строк сметы, отменяет подбор под бюджет; цены, введённые вручную, остаются" aria-label="Сбросить цены к ценам сметы">
              <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9" /><path d="M13.5 2.5v3.2h-3.2" /></svg>
            </button>
          </>} />

        <div className="rs-filterbar">
          <div className="rs-seg" role="radiogroup" aria-label="Какие строки показать">
            <button type="button" role="radio" aria-checked={filter === 'all'} className={filter === 'all' ? 'active' : undefined} onClick={() => setFilter('all')}>Все строки <b>{result.items.length}</b></button>
            <button type="button" role="radio" aria-checked={filter === 'attention'} className={filter === 'attention' ? 'active' : undefined} onClick={() => setFilter('attention')} disabled={counts.attention === 0 && filter !== 'attention'}>Требуют внимания <b>{counts.attention}</b></button>
          </div>
          {counts.attention > 0 && <p className="rs-counts">
            {counts.bad > 0 && <span className="bad"><StateIcon state="bad" /> ошибки {counts.bad}</span>}
            {counts.warn > 0 && <span className="warn"><StateIcon state="warn" /> предупреждения {counts.warn}</span>}
            {counts.ad > 0 && <span className="dump">АД: выбрать {counts.ad}</span>}
          </p>}
          {changed.length > 0 && <p className="rs-changed" title="Цена единицы отличается от цены в Excel: правка или подбор под бюджет">изменено цен: <b>{changed.length}</b></p>}
          <div className="rs-seg rs-density" role="radiogroup" aria-label="Плотность таблицы">
            <button type="button" role="radio" aria-checked={density === 'full'} className={density === 'full' ? 'active' : undefined} onClick={() => chooseDensity('full')} title="С подписями: места, брутто, ставка">Подробно</button>
            <button type="button" role="radio" aria-checked={density === 'compact'} className={density === 'compact' ? 'active' : undefined} onClick={() => chooseDensity('compact')} title="Одна строка на товар: больше строк на экране">Компактно</button>
          </div>
        </div>
        {!sirMode && <BudgetStatus budget={budget} />}

        <div className="goods-table-top-scroll rs-top-scroll" ref={topScrollRef} onScroll={() => syncHorizontalScroll('top')} aria-hidden="true"><div /></div>
        <div className="rs-table-wrap" ref={tableWrapRef} onScroll={() => syncHorizontalScroll('body')}>
          <table className="sheet rs-table" ref={tableRef} style={{ ['--rs-num-w' as string]: numWidth }}>
            <caption className="rs-sr">Строки расчёта: цену единицы можно менять; Enter — следующая строка, Esc — вернуть цену</caption>
            <thead>
              <tr>
                <th className="rs-num" scope="col">№</th>
                <th className="rs-name" scope="col"><span className="rs-name-head"><span>Товар{copyNames}</span><span className="rs-th-hint">код ТН ВЭД{copyCodes}</span></span></th>
                <th scope="col" title="Количество; под ним — места × штук в месте">Кол-во, шт{copyCol('Кол-во', (item) => item.pairs)}</th>
                {sirMode ? <>
                  <th scope="col" title={sirGross ? 'Вес брутто — по нему считается ТС; под ним — нетто' : 'Вес нетто — по нему считается ТС; под ним — брутто'}>{sirGross ? 'Брутто' : 'Нетто'}, кг{copyCol(sirGross ? 'Брутто' : 'Нетто', (item) => sirGross ? item.grossKg : item.netKg)}</th>
                  <th scope="col" className="rs-sir-col" title="Стоимостной индикатор риска из файла профиля, USD за 1 кг">СУР, $/кг{copyCol('СУР', (item) => item.sir?.priceUsd)}</th>
                  {sirDiscount > 0 && <th scope="col" className="rs-sir-col" title={`Индикатор со снижением: берётся ${100 - sirDiscount} %`}>−{displayNumber(sir.sirDiscountPercent)}%, $/кг{copyCol('СУР со снижением', (item) => item.sir?.pricePerKgUsd)}</th>}
                  <th scope="col" title="Вес × СУР со снижением; у строк без индикатора — стоимость по инвойсу">ТС, USD{copyCol('ТС, USD', (item) => item.sir ? item.sir.valueUsd : item.goodsCost || item.costInput)}</th>
                </> : <>
                  <th scope="col" title="Вес нетто; под ним — брутто">Нетто, кг{copyCol('Нетто', (item) => item.netKg)}</th>
                  <th scope="col" className="rs-price-col" title="Цену единицы можно менять прямо в таблице">Цена ед., USD{copyCol('Цена ед.', (item, index) => draftPrices[index] ?? (item.pricePerPair || item.pairPrice || items[index]?.pricePerPair))}</th>
                  <th scope="col">Стоимость, USD{copyCol('Стоимость', (item) => item.goodsCost || item.costInput)}</th>
                </>}
                {delivery && <th scope="col">Доставка, USD{copyCol('Доставка', (item) => item.deliveryUsd)}</th>}
                <th scope="col" className="rs-kzt-start" title="Таможенная стоимость">ТС, ₸{copyCol('ТС, ₸', (item) => item.customsValueKzt)}</th>
                <th scope="col">Пошлина, ₸{copyCol('Пошлина', (item) => item.dutyKzt)}</th>
                {antiDumping && <th scope="col">Антидемпинг, ₸{copyCol('Антидемпинг', (item) => item.antiDumpingKzt)}</th>}
                <th scope="col">НДС, ₸{copyCol('НДС', (item) => item.vatKzt)}</th>
                <th scope="col">Сбор, ₸{copyCol('Сбор', (item) => item.feeKzt)}</th>
              </tr>
            </thead>
            <tbody>
              {visible.length === 0 && <tr className="rs-empty"><td colSpan={columns}>
                Строк, требующих внимания, больше нет. <button type="button" className="button-link" onClick={() => setFilter('all')}>Показать все строки</button>
              </td></tr>}
              {visible.map(({ item, index, state, antiDumpingInfo, attention }) => {
                const source = items[index]
                // The value typed by the user must have priority over the
                // calculated pair price. Otherwise the input immediately
                // jumps back to the old value after every keystroke.
                const price = draftPrices[index] ?? (item.pricePerPair || item.pairPrice || source?.pricePerPair || '')
                const sourceRow = index + 1
                const issues = [...(item.errors ?? []), ...(item.warnings ?? [])]
                const name = itemDisplayName(item, source)
                const currency = item.goodsCostCurrency || item.costCurrency || 'USD'
                const isExpanded = expanded.includes(index)
                const reasonId = issues.length > 0 ? `rs-reason-${index}` : undefined
                const aiNote = budget.notes[index]
                const edited = !item.excluded && !aiNote && priceDiffers(source?.sourcePricePerPair, price)
                const rate = dutyRateText(item.directorySnapshot)
                const rowClass = [
                  `rs-row-${state}`,
                  attention ? 'attention' : '',
                  item.excluded ? 'rv-excluded' : '',
                  flashRow === index ? 'flash' : '',
                ].filter(Boolean).join(' ')

                return (
                  <tr key={`${item.productId}-${index}`} id={`rs-row-${index}`} className={rowClass}>
                    <td className="rs-num">
                      <span className="rs-num-cell">
                        <span>{sourceRow}</span>
                        <span className={`rs-state ${state}`} role="img" aria-label={STATE_LABEL[state]} title={issues.join('\n') || STATE_LABEL[state]}><StateIcon state={state} /></span>
                      </span>
                    </td>
                    <td className={isExpanded ? 'rs-name open' : 'rs-name'}>
                      <span className="rs-name-line">
                        <span className="rs-name-text" title={name}>{name}</span>
                        {item.code || source?.code ? <CopyCode code={item.code || source?.code || ""} /> : <code>—</code>}
                      </span>
                      {issues.length > 0 && <button type="button" id={reasonId} className={`rs-reason ${state}`} aria-expanded={isExpanded} onClick={() => toggleExpanded(index)} title={isExpanded ? 'Свернуть' : issues.join('\n')}>
                        {isExpanded
                          ? issues.map((issue) => <span key={issue}>{issue}</span>)
                          : <span>{issues[0]}{issues.length > 1 && <b> +{issues.length - 1}</b>}</span>}
                      </button>}
                    </td>
                    <td>{displayNumber(item.pairs)}{(item.places || item.pairsPerPlace) && <small className="rs-sub">{displayNumber(item.places)} × {displayNumber(item.pairsPerPlace)}</small>}</td>
                    {sirMode ? <>
                      <td>{displayNumber(sirGross ? item.grossKg : item.netKg)}{(sirGross ? item.netKg : item.grossKg) && <small className="rs-sub">{sirGross ? 'нетто' : 'брутто'} {displayNumber(sirGross ? item.netKg : item.grossKg)}</small>}</td>
                      <td className="rs-sir-col" title={item.sir ? `Профиль риска ${item.sir.profile}, страна ${item.sir.country}` : 'Для кода индикатора нет — строка считается по инвойсу'}>{item.sir ? displayNumber(item.sir.priceUsd) : <span className="rs-sir-none">нет</span>}</td>
                      {sirDiscount > 0 && <td className="rs-sir-col">{item.sir ? displayNumber(item.sir.pricePerKgUsd) : '—'}</td>}
                      <td title={`Инвойс: ${displayNumber(price)} × ${displayNumber(item.pairs)} = ${displayNumber(item.goodsCost || item.costInput)} ${currency}${item.sir ? '' : '. Цену меняют в режиме «По инвойсу»'}`}>{item.sir ? displayNumber(item.sir.valueUsd) : <>{displayNumber(item.goodsCost || item.costInput)}{currency !== 'USD' && <small> {currency}</small>}<small className="rs-sub">по инвойсу</small></>}</td>
                    </> : <>
                    <td>{displayNumber(item.netKg)}{item.grossKg && <small className="rs-sub">брутто {displayNumber(item.grossKg)}</small>}</td>
                    <td className="rs-price-col">
                      <span className="price-field">
                        <BudgetNote note={aiNote} />
                        {edited && <span className="rs-tag edited" title={`В Excel: ${displayNumber(source?.sourcePricePerPair)} USD`} aria-hidden="true">изм.</span>}
                        <input
                          className={edited || aiNote ? 'rs-price changed' : 'rs-price'}
                          inputMode="decimal"
                          value={price.replace('.', ',')}
                          disabled={item.excluded}
                          onFocus={() => { editStartRef.current = { index, value: price } }}
                          onChange={(event) => { const value = event.target.value.replace(',', '.'); setDraftPrices((current) => ({ ...current, [index]: value })); onPriceChange(index, value) }}
                          onBlur={() => {
                            if (skipBlurRef.current === index) { skipBlurRef.current = null; return }
                            const value = draftPrices[index]
                            if (value === undefined) return
                            setDraftPrices(({ [index]: _done, ...rest }) => rest)
                            // Пустое поле — убрать цену: строка считается без неё (по ИТП или с пометкой «Укажите цену»).
                            onPriceBlur(index, value.trim())
                          }}
                          onKeyDown={(event) => priceKeyDown(event, index)}
                          aria-label={`Цена за единицу, USD, строка ${sourceRow}: ${name}${edited ? `; в Excel ${source?.sourcePricePerPair}` : ''}`}
                          aria-describedby={reasonId}
                        />
                      </span>
                    </td>
                    <td>{displayNumber(item.goodsCost || item.costInput)}{currency !== 'USD' && <small> {currency}</small>}</td>
                    </>}
                    {delivery && <td>{displayNumber(item.deliveryUsd)}</td>}
                    <td className="rs-kzt-start" title={item.sir ? sirTitle(item.sir) : undefined}>{displayNumber(item.customsValueKzt)}{item.sir && !sirMode && <small className="rs-sub sir-sub">СУР {displayNumber(item.sir.pricePerKgUsd)} $/кг</small>}</td>
                    <td>{displayNumber(item.dutyKzt)}{rate && <small className="rs-sub">{rate}</small>}{antiDumpingInfo && <AntiDumpingChooser info={antiDumpingInfo} choice={source?.antiDumpingChoice} disabled={loading || item.excluded} onChoose={(key) => onAntiDumpingChoice(index, key)} />}</td>
                    {antiDumping && <td title={antiDumpingInfo?.text}>{displayNumber(item.antiDumpingKzt)}{(antiDumpingInfo?.status === 'applied' || antiDumpingInfo?.status === 'chosen') && <small className="rs-sub">{antiDumpingInfo.number} · {String(antiDumpingInfo.percent ?? '').replace('.', ',')}%</small>}</td>}
                    <td>{displayNumber(item.vatKzt)}</td>
                    <td>{displayNumber(item.feeKzt)}</td>
                  </tr>
                )
              })}
            </tbody>
            <tfoot>
              <tr>
                <td className="rs-num" />
                <td className="rs-name">Итого{filter === 'attention' && <small className="rs-foot-note"> · по всем строкам</small>}</td>
                <td>{displayNumber(checkTotals.quantity)}<small className="rs-sub">мест {displayNumber(checkTotals.places)}</small></td>
                {sirMode ? <>
                  <td>{displayNumber(weightText(sirGross ? checkTotals.grossKg : checkTotals.netKg))}<small className="rs-sub">{sirGross ? 'нетто' : 'брутто'} {displayNumber(weightText(sirGross ? checkTotals.netKg : checkTotals.grossKg))}</small></td>
                  <td className="rs-sir-col" />
                  {sirDiscount > 0 && <td className="rs-sir-col" />}
                  <td>{displayNumber(moneyText(sirValueUsd))}</td>
                </> : <>
                  <td>{displayNumber(weightText(checkTotals.netKg))}<small className="rs-sub">брутто {displayNumber(weightText(checkTotals.grossKg))}</small></td>
                  <td className="rs-price-col" />
                  <td>{displayNumber(result.totals.totalCostUsd)}</td>
                </>}
                {delivery && <td>{displayNumber(result.totals.totalDeliveryUsd)}</td>}
                <td className="rs-kzt-start">{displayNumber(result.totals.customsValueKzt)}</td>
                <td>{displayNumber(result.totals.dutyKzt)}</td>
                {antiDumping && <td>{displayNumber(result.totals.antiDumpingKzt)}</td>}
                <td>{displayNumber(result.totals.vatKzt)}</td>
                <td>{displayNumber(result.totals.feeKzt)}</td>
              </tr>
            </tfoot>
          </table>
        </div>
      </section>
      {compareOpen && <SirCompareModal result={result} items={items} sir={sir} preview={previewSir} onClose={closeCompare} />}
    </div>
  )
}
