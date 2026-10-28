import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { KeyboardEvent, MouseEvent, ReactNode, useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { GoodsCalculationResult, GoodsItemInput, ItpSaved, SirParams } from '@/shared/types/goods'
import { moneyText } from '@/shared/lib/rounding'
import { AiDot, budgetAmount, BudgetMeter, BudgetRunButton, BudgetStatus, RunMode } from '../BudgetTool/BudgetTool'
import { BudgetDistributionState } from '../../hooks/useBudgetDistribution'
import { changedFromExcel, changedFromRows, priceDiffers, shownPrice } from '../../lib/priceChanges'
import { storageFailed } from '../../lib/storageNotice'
import { CalculatorActivity } from '../../types'
import { CalculationErrors } from './CalculationErrors'
import { AntiDumpingChooser, AntiDumpingInfo, antiDumpingTone } from './AntiDumpingChooser'
import { ItpCalculator } from './ItpCalculator'
import { ResultSummary } from './ResultSummary'
import { RowPanel } from './RowPanel'
import { SirControls } from './SirControls'
import { SirCompareModal } from './SirCompareModal'
import { commonProblems, plural, problemGroups, ProblemTone, rowProblems } from './errorGroups'
import { calculatedRows, customsPaymentsUsd, displayNumber, fixedNumber, excelColumn, excelColumnHtml, hasAntiDumping, itemDisplayName, itemNames, parseCalculatorNumber, RowState, rowPaymentKzt, rowState, rowTotals, sirTitle, textColumn } from './resultValues'
import { CopyColumn } from '@/shared/components/CopyCode/CopyColumn'
import { spaceBelow } from '../../lib/spaceBelow'
import '../../goods.css'
import './ResultStep.css'

type Props = {
  result: GoodsCalculationResult
  items: GoodsItemInput[]
  loading: boolean
  activity: CalculatorActivity
  storageNotice: string
  // Название сметы, дата, клиент и этапы — слева в строке заголовка.
  header: ReactNode
  // Есть правки, которых ещё нет на сервере.
  unsaved: boolean
  saveDraft: () => void
  exportExcel: () => void
  onBackToReview: () => void
  onPriceChange: (index: number, value: string) => void
  onPriceBlur: (index: number, value: string) => void
  // Вариант антидемпинга строки ('' — как по стране, 'none' — не начислять); строка пересчитывается.
  onAntiDumpingChoice: (index: number, key: string) => void
  // «Сменить» код строки: выбор из справочника, затем пересчёт с текущими ценами.
  onChangeCode: (index: number) => void
  // «Учитывать строку в расчёте»; смета пересчитывается.
  onToggleIncluded: (index: number, included: boolean) => void
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

type Filter = 'all' | 'attention' | 'ai'

// «Показать все колонки» запоминается в браузере.
const ALL_COLUMNS_KEY = 'tradeflow.result-all-columns'
const storedAllColumns = () => {
  try { return localStorage.getItem(ALL_COLUMNS_KEY) === '1' } catch { return false }
}

const positive = (value: unknown) => (parseCalculatorNumber(value as string) ?? 0) > 0

// Значок у номера: ошибка, предупреждение, следующий шаг (нет цены), исключена.
type Mark = 'bad' | 'warn' | 'todo' | 'off'
const MARK_LABEL: Record<Mark, string> = { bad: 'Ошибка', warn: 'Проверить', todo: 'Не указана цена', off: 'Исключена' }
const MARK: Record<ProblemTone, Mark> = { error: 'bad', warn: 'warn', todo: 'todo' }

// Значок состояния строки: форма отличается, а не только цвет (!, ?, –).
function StateIcon({ state }: { state: RowState | Mark }) {
  return <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
    {state === 'ok' && <path d="M2.8 6.3 5 8.4l4.2-4.6" />}
    {state === 'bad' && <><path d="M6 2.8v4" /><circle cx="6" cy="9.2" r=".5" fill="currentColor" /></>}
    {state === 'warn' && <><path d="M4.4 4.4a1.7 1.7 0 1 1 2.4 1.5c-.5.3-.8.6-.8 1.2v.2" /><circle cx="6" cy="9.4" r=".5" fill="currentColor" /></>}
    {state === 'off' && <path d="M3.4 6h5.2" />}
    {state === 'todo' && <circle cx="6" cy="6" r="1.6" fill="currentColor" />}
  </svg>
}

// Поле в фокусе (ввод текста) — стрелки принадлежат ему, а не таблице.
const typing = (target: EventTarget | null) => target instanceof HTMLElement && (target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName))

// Этап 3: строка заголовка, полоса итогов, таблица строк и панель выбранной строки справа.
// Прокручивается страница; шапка таблицы прилипает. Действия, которых нет на экране, — в меню «⋯».
export function ResultStep({
  result,
  items,
  loading,
  activity,
  storageNotice,
  header,
  unsaved,
  saveDraft,
  exportExcel,
  onBackToReview,
  onPriceChange,
  onPriceBlur,
  onAntiDumpingChoice,
  onChangeCode,
  onToggleIncluded,
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
  const [filter, setFilter] = useState<Filter>('all')
  const [query, setQuery] = useState('')
  const [allColumns, setAllColumns] = useState(storedAllColumns)
  // Панель строки открывается только по щелчку по строке — при входе на этап таблица видна целиком.
  const [selected, setSelected] = useState<number | null>(null)
  const [flashRow, setFlashRow] = useState<number | null>(null)
  const [compareOpen, setCompareOpen] = useState(false)
  const [itpOpen, setItpOpen] = useState(false)
  // Подбор, который запустится, когда пересчёт после «Вернуть цены из файла» закончится.
  const [pendingRun, setPendingRun] = useState<RunMode | null>(null)
  const tableRef = useRef<HTMLTableElement>(null)
  // Выбранная ячейка, как в Excel: колонка — номер ячейки в строке (0 и 1 — номер и значок, их не выбрать).
  // Рамка у ячейки и подсветка заголовка её колонки ставятся классами после каждого рендера: колонки
  // зависят от режима («Все колонки», СУР, доставка), а номер ячейки — нет.
  const [activeCol, setActiveCol] = useState(2)
  useLayoutEffect(() => {
    const table = tableRef.current
    if (!table) return
    table.querySelectorAll('.rs-active-cell, .rs-active-col').forEach((cell) => cell.classList.remove('rs-active-cell', 'rs-active-col'))
    const row = selected === null ? null : document.getElementById(`rs-row-${selected}`) as HTMLTableRowElement | null
    if (!row) return
    const col = Math.min(activeCol, row.cells.length - 1)
    row.cells[col]?.classList.add('rs-active-cell')
    table.tHead?.rows[0]?.cells[col]?.classList.add('rs-active-col')
  })
  const menuRef = useRef<HTMLDetailsElement>(null)
  // Цена до начала ввода (для Esc) и строка, выход из которой не должен пересчитывать.
  const editStartRef = useRef<{ index: number; value: string } | null>(null)
  const skipBlurRef = useRef<number | null>(null)
  // Таблица и панель строки — до низа окна, как лист этапа проверки: строки прокручиваются внутри, шапка
  // и «Итого» всегда на виду. Мерим блок «таблица + панель» (.rs-layout): --rs-top — где он начинается,
  // --rs-after — что под ним (подпись «Предварительный расчёт» и нижний отступ страницы), --rs-around —
  // строка фильтров и рамка карточки вокруг прокрутки.
  const tableWrapRef = useRef<HTMLDivElement | null>(null)
  const measureTable = useCallback(() => {
    const wrap = tableWrapRef.current
    const layout = wrap?.closest<HTMLElement>('.rs-layout')
    const card = wrap?.closest('.rs-card')
    if (!wrap || !layout || !card) return
    const box = wrap.getBoundingClientRect()
    const area = layout.getBoundingClientRect()
    const frame = card.getBoundingClientRect()
    layout.style.setProperty('--rs-top', `${area.top + window.scrollY}px`)
    layout.style.setProperty('--rs-after', `${spaceBelow(layout)}px`)
    layout.style.setProperty('--rs-around', `${box.top - frame.top + frame.bottom - box.bottom}px`)
  }, [])
  useLayoutEffect(measureTable)
  // Высота страницы сменилась без рендера этапа (раскрылась плашка ошибок, догрузились шрифты и фото) — тоже
  // перемер: иначе таблица останется по старой высоте и под ней будет пустота. Окно — для высоты окна.
  useEffect(() => {
    const page = tableWrapRef.current?.closest('.goods-page')
    const observer = new ResizeObserver(measureTable)
    if (page) observer.observe(page)
    window.addEventListener('resize', measureTable)
    return () => { observer.disconnect(); window.removeEventListener('resize', measureTable) }
  }, [measureTable])
  const closeCompare = useCallback(() => setCompareOpen(false), [])
  // По СУР: итог того же расчёта по инвойсу — для строки разницы под «К оплате». Пробный расчёт, смета не меняется.
  const [invoiceTotal, setInvoiceTotal] = useState<string | null>(null)
  const previewRef = useRef(previewSir)
  previewRef.current = previewSir
  const sirOn = sir.customsValueMode === 'sir'
  useEffect(() => {
    setInvoiceTotal(null)
    if (!sirOn) return
    let active = true
    previewRef.current({ customsValueMode: 'invoice' }).then((preview) => { if (active) setInvoiceTotal(preview.totals.customsPaymentsKzt || null) }, () => { /* без разницы — только сумма */ })
    return () => { active = false }
  }, [sirOn, result])

  const checkTotals = rowTotals(result)
  const antiDumping = hasAntiDumping(result)
  // Колонка доставки нужна, только если доставка есть.
  const delivery = positive(result.totals.totalDeliveryUsd) || result.items.some((item) => positive(item.deliveryUsd))
  // По СУР цены инвойса на ТС не влияют: подбирать их под бюджет бессмысленно.
  const sirMode = sir.customsValueMode === 'sir'
  const includedRows = result.items.filter((item) => !item.excluded)
  const sirRows = includedRows.filter((item) => item.customsBasis === 'sir').length
  const sirGross = sir.sirWeight === 'gross'
  const sirDiscount = Number(sir.sirDiscountPercent || 0)
  const sirValueUsd = includedRows.reduce((total, item) => total + (parseCalculatorNumber(item.sir ? item.sir.valueUsd : item.costUsd) ?? 0), 0)
  const calculating = activity === 'calculate'

  // Цены нет ни у одной строки: следующий шаг — подбор цен (главная кнопка), выгружать ещё нечего. Тогда «нет цены»
  // не проблема строк: ни баннера, ни вкладки «Внимание», ни значков.
  const noPrices = !sirMode && includedRows.length > 0 && result.items.every((item, index) => item.excluded || !positive(item.pricePerPair || item.pairPrice || items[index]?.pricePerPair))
  const rows = result.items.map((item, index) => {
    const state = rowState(item)
    const antiDumpingInfo = item.directorySnapshot?.antiDumping as AntiDumpingInfo | undefined
    const adAsk = !item.excluded && antiDumpingTone(antiDumpingInfo) === 'ask'
    const names = itemNames(item, items[index])
    const code = item.code || items[index]?.code || ''
    const problems = item.excluded ? [] : rowProblems(item, code).filter((problem) => !(noPrices && problem.kind === 'price'))

    // «Внимание» — строки с проблемами (строка по инвойсу без индикатора СУР — не проблема) и невыбранным антидемпингом.
    return { item, index, state, problems, antiDumpingInfo, names, code, aiNote: budget.notes[index], attention: problems.length > 0 || adAsk }
  })
  // Проблемы большинства строк уже в баннере — под названием только то, чем строка отличается.
  const common = commonProblems(rows.filter((row) => !row.item.excluded).map((row) => row.problems.map((problem) => problem.text)))
  const groups = problemGroups(result, rows.map((row) => row.code))
  const blocking = groups.some((group) => group.tone === 'error')
  const hasCodeProblems = groups.some((group) => group.kind === 'code')
  // Пока нет кодов, «нет цены» — не следующий шаг: баннер о ценах появится, когда коды будут указаны.
  const shownGroups = hasCodeProblems || noPrices ? groups.filter((group) => group.kind !== 'price') : groups
  // Ни одна строка не рассчитана — итога нет: прогресс бюджета скрыт, выгружать нечего.
  const notCalculated = calculatedRows(result) === 0
  // Строки с ценой прошлого подбора (её нет в строке сметы): при новом подборе их цены заменяются.
  const replaceable = changedFromRows(items, result).length
  // У всех строк своя цена (из файла, по ИТП, вручную) — подбор их не меняет, подбирать нечего.
  const nothingToPrice = includedRows.length > 0 && replaceable === 0 && result.items.every((item, index) => item.excluded || positive(shownPrice(items, result, index)))
    ? 'У всех строк цена из файла или введена вручную — подбирать нечего'
    : undefined
  const counts = {
    bad: rows.filter((row) => row.problems.some((problem) => problem.tone === 'error')).length,
    warn: rows.filter((row) => row.problems.length > 0 && !row.problems.some((problem) => problem.tone === 'error')).length,
    attention: rows.filter((row) => row.attention).length,
    ai: rows.filter((row) => row.aiNote).length,
  }
  const needle = query.trim().toLowerCase()
  const needleDigits = needle.replace(/\D/g, '')
  const visible = rows.filter((row) => {
    if (filter === 'attention' && !row.attention) return false
    if (filter === 'ai' && !row.aiNote) return false
    if (!needle) return true
    return `${row.names.main} ${row.names.sub}`.toLowerCase().includes(needle) || (needleDigits.length > 0 && row.code.replace(/\D/g, '').includes(needleDigits))
  })
  const selectedRow = selected === null ? null : rows[selected] ?? null
  const selectedPosition = selectedRow ? visible.indexOf(selectedRow) : -1
  // Копируются все строки, даже при фильтре: иначе в Excel значения съедут.
  const copyCol = (label: string, get: (item: GoodsCalculationResult['items'][number], index: number) => string | undefined) =>
    <CopyColumn label={label} text={() => excelColumn(result.items.map(get))} html={() => excelColumnHtml(result.items.map(get))} />
  const copyNames = <CopyColumn label="Товар" text={() => textColumn(result.items.map((item, index) => itemDisplayName(item, items[index])))} />
  // Код — только цифры, как у CopyCode.
  const copyCodes = <CopyColumn label="Код ТН ВЭД" text={() => textColumn(rows.map((row) => row.code.replace(/\D/g, '')))} />
  const changed = changedFromExcel(items, result)
  const priceOf = (index: number) => draftPrices[index] ?? (result.items[index]?.pricePerPair || result.items[index]?.pairPrice || items[index]?.pricePerPair || '')

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

  // ↑ / ↓ вне полей ввода и окон — предыдущая / следующая строка в панели; ← / → — соседняя ячейка строки.
  const moveRef = useRef<(step: number) => void>(() => {})
  moveRef.current = (step: number) => {
    const at = selectedPosition
    const next = at < 0 ? visible[0] : visible[at + step]
    if (!next) return
    setSelected(next.index)
    window.requestAnimationFrame(() => document.getElementById(`rs-row-${next.index}`)?.scrollIntoView({ block: 'nearest' }))
  }
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (!['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(event.key)) return
      if (event.altKey || event.ctrlKey || event.metaKey || typing(event.target)) return
      if (document.querySelector('[aria-modal="true"], [role="dialog"]')) return
      event.preventDefault()
      if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
        const last = (tableRef.current?.tHead?.rows[0]?.cells.length ?? 3) - 1
        setActiveCol((col) => Math.max(2, Math.min(last, Math.min(col, last) + (event.key === 'ArrowLeft' ? -1 : 1))))
        return
      }
      moveRef.current(event.key === 'ArrowUp' ? -1 : 1)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // Меню «⋯» закрывается щелчком мимо и клавишей Esc.
  useEffect(() => {
    const close = (event: Event) => {
      const menu = menuRef.current
      if (!menu?.open) return
      if (event instanceof globalThis.KeyboardEvent) {
        if (event.key !== 'Escape') return
        menu.open = false
        menu.querySelector('summary')?.focus()
      } else if (!event.composedPath().includes(menu)) menu.open = false
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', close)
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', close) }
  }, [])

  function closeMenuOnItem(event: MouseEvent<HTMLElement>) {
    if ((event.target as HTMLElement).closest('.rs-menu-item') && menuRef.current) menuRef.current.open = false
  }

  function toggleAllColumns(value: boolean) {
    setAllColumns(value)
    try { localStorage.setItem(ALL_COLUMNS_KEY, value ? '1' : '0') } catch { /* без памяти — выбор на эту сессию */ }
  }

  // Номер строки из плашки ошибок: выбрать строку, прокрутить к ней, подсветить и поставить курсор в цену.
  function showRow(rowNumber: number) {
    const index = rowNumber - 1
    if (filter !== 'all' || query) { setFilter('all'); setQuery('') }
    setSelected(index)
    setFlashRow(index)
    window.requestAnimationFrame(() => {
      const row = document.getElementById(`rs-row-${index}`)
      row?.scrollIntoView({ block: 'center' })
      row?.querySelector<HTMLInputElement>('input.rs-price:not(:disabled)')?.focus({ preventScroll: true })
    })
  }

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

  // Поле цены строки — одно и то же в таблице и в панели.
  function priceInput(index: number, inTable: boolean, label: string) {
    const item = result.items[index]
    const source = items[index]
    const price = priceOf(index)
    const edited = !item.excluded && !budget.notes[index] && priceDiffers(source?.sourcePricePerPair, price)
    return <input
      className={[inTable ? 'rs-price' : 'rp-price', edited ? 'edited' : ''].filter(Boolean).join(' ')}
      inputMode="decimal"
      value={price.replace('.', ',')}
      placeholder="—"
      disabled={item.excluded || (sirMode && !inTable)}
      title={edited ? `В Excel: ${displayNumber(source?.sourcePricePerPair)} USD` : undefined}
      onFocus={(event) => {
        editStartRef.current = { index, value: price }
        setSelected(index)
        // Поле в таблице — рамка выбранной ячейки на цене (щелчок по полю до строки не доходит, фокус — с клавиатуры).
        const cell = event.currentTarget.closest('td')
        if (cell) setActiveCol(cell.cellIndex)
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

  function backToReview() {
    const count = changed.length
    const bulk = canUndoPrices || Object.keys(budget.notes).length > 0
    if (!window.confirm(count > 0
      ? `Цены этапа «Результат» вернутся к ценам из Excel: изменено ${count} ${plural(count, ['цена', 'цены', 'цен'])}. Перейти к проверке строк?`
      : bulk ? 'Цены подбора под кассу сбросятся. Перейти к проверке строк?'
      : 'Перейти к проверке строк? Цены, изменённые на этапе «Результат», вернутся к ценам из Excel.')) return
    onBackToReview()
  }

  function restoreInitial() {
    // Пункт неактивен, пока менять нечего (replaceable = 0).
    if (!window.confirm(`Вернуть цены из файла? Изменится ${replaceable} ${plural(replaceable, ['цена', 'цены', 'цен'])} после подбора под кассу; цены, введённые вручную, останутся. Вернуть подбор можно пунктом «Отменить подбор цен».`)) return
    onRestoreInitial()
  }

  // «Подобрать цены» из баннера: бюджет задан — подбор; нет — курсор в поле бюджета.
  // Подбор цен: ИИ (главная кнопка) или распределение под бюджет. Без бюджета — курсор в поле бюджета.
  // Цены прошлого подбора подбор не меняет (строки с ценой неподвижны), поэтому их замена — после
  // подтверждения: «Вернуть цены из файла», а когда пересчёт закончится — тот же подбор (pendingRun).
  function startRun(mode: RunMode) {
    if (budgetAmount(budget) === null) { document.getElementById('rs-budget')?.focus(); return }
    if (replaceable > 0) {
      if (!window.confirm(`Заменить текущие цены у ${replaceable} ${plural(replaceable, ['строки', 'строк', 'строк'])}? Отменить можно через «Отменить подбор цен».`)) return
      setPendingRun(mode)
      onRestoreInitial()
      return
    }
    void budget.run(mode)
  }

  useEffect(() => {
    if (!pendingRun || loading) return
    setPendingRun(null)
    void budget.run(pendingRun)
  }, [pendingRun, loading, budget])

  // Кнопка «Подобрать цены» в баннере — то же, что главная кнопка подбора.
  const pickPrices = () => startRun('ai')

  // Перед выгрузкой — предупредить о неполном итоге и невыбранном антидемпинге.
  function exportChecked() {
    const adAsk = rows.filter((row) => !row.item.excluded && antiDumpingTone(row.antiDumpingInfo) === 'ask').length
    const notes = [
      calculatedRows(result) < result.totals.totalCount && `рассчитано ${calculatedRows(result)} из ${result.totals.totalCount} строк — итог неполный`,
      adAsk > 0 && `антидемпинг не выбран: ${adAsk} ${plural(adAsk, ['строка', 'строки', 'строк'])}`,
    ].filter(Boolean)
    if (notes.length > 0 && !window.confirm(`Перед выгрузкой: ${notes.join('; ')}. Выгрузить Excel всё равно?`)) return
    exportExcel()
  }

  const failed = storageFailed(storageNotice)
  // «Все колонки»: ширины колонок, px (CSS), таблица не уже их суммы — «Товар» не уже 160 px и забирает остальное место.
  const wideMinWidth = 36 + 22 + 160 + 128 + 88 + 96 + 96 + 104 + (sirMode && sirDiscount > 0 ? 96 : 0) + (delivery ? 104 : 0) + 120 + 112 + (antiDumping ? 112 : 0) + 112 + 96 + 136
  // Итог колонки: значения нет ни у одной строки — «—», а не 0,00. calculatedOnly — суммы, которых нет у строк с ошибкой.
  const colTotal = (get: (item: GoodsCalculationResult['items'][number]) => string | undefined, total: string, calculatedOnly = false) =>
    includedRows.some((item) => !(calculatedOnly && item.errors?.length) && String(get(item) ?? '').trim() !== '') ? total : '—'
  // Стоимость, USD — и в обычном виде (8 колонок), остальные — только во «Всех колонках».
  const columns = 8 + (allColumns ? 1 + (sirMode && sirDiscount > 0 ? 1 : 0) + (delivery ? 1 : 0) + 4 + (antiDumping ? 1 : 0) : 0)
  const tabs: { value: Filter; label: string; count: number }[] = [
    { value: 'all', label: 'Все', count: rows.length },
    { value: 'attention', label: 'Внимание', count: counts.attention },
    { value: 'ai', label: 'Цена оценена', count: counts.ai },
  ]

  // Пока у строк нет кода ТН ВЭД, подбирать цены бессмысленно: пошлину и НДС не посчитать.
  const codeFirst = hasCodeProblems ? 'Сначала укажите коды ТН ВЭД' : undefined

  // Меню «⋯» — проверки и вид; подбор и откат цен — в стрелке кнопки «Подобрать цены ИИ».
  const menu = <details className="rs-more" ref={menuRef}>
    <summary className="rs-icon" title="Ещё действия" aria-label="Ещё действия">
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="currentColor"><circle cx="3.5" cy="8" r="1.3" /><circle cx="8" cy="8" r="1.3" /><circle cx="12.5" cy="8" r="1.3" /></svg>
    </summary>
    <div className="rs-menu" onClick={closeMenuOnItem}>
      <p className="rs-menu-label">Проверки</p>
      <button type="button" className="rs-menu-item" onClick={() => setCompareOpen(true)} disabled={loading} title="Сравнить инвойс и СУР: итоги и каждая строка рядом">Сравнить с СУР</button>
      <button type="button" className="rs-menu-item" onClick={() => setItpOpen(true)}>Сверка по ИТП{itp?.value ? ' · задана' : ''}</button>
      <hr />
      <p className="rs-menu-label">Вид</p>
      <label className="rs-menu-item rs-menu-check"><input type="checkbox" checked={allColumns} onChange={(event) => toggleAllColumns(event.target.checked)} />Показать все колонки</label>
    </div>
  </details>


  return (
    <div className="goods-step-page rs-step">
      <div className="estimate-bar rs-titlebar">
        {header}
      </div>

      <CalculationErrors groups={shownGroups} onBackToReview={backToReview} onPickPrices={sirMode ? undefined : pickPrices} onShowRow={showRow} />

      <ResultSummary result={result} calculating={calculating} delivery={delivery} sirInvoiceRows={sirMode ? includedRows.length - sirRows : null} invoiceTotal={invoiceTotal} onCompare={() => setCompareOpen(true)} tools={<>
        <div className="rs-sum-row">
          {/* Ошибка сохранения и «Сохраняем…» важнее; иначе — есть ли несохранённое, или когда сохранено. */}
          {unsaved && !failed && !storageNotice.startsWith('Сохраняем')
            ? <span className="rs-save-state unsaved" role="status">Есть несохранённые изменения</span>
            : <span className={failed ? 'rs-save-state failed' : 'rs-save-state'} role={failed ? 'alert' : 'status'} title={storageNotice}>{storageNotice || 'Изменения сохраняются автоматически'}</span>}
          <button type="button" className="rs-btn" onClick={saveDraft} disabled={loading} title="Сохранить на сервере сейчас, не дожидаясь автосохранения (Ctrl+S)">{activity === 'save' ? 'Сохраняем…' : 'Сохранить'}</button>
          <button type="button" className={noPrices || notCalculated ? 'rs-btn' : 'rs-btn primary'} onClick={exportChecked} disabled={loading || noPrices || notCalculated} title={noPrices || notCalculated ? 'Сначала завершите расчёт' : undefined}>
            <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M8 2.5v8M4.5 7 8 10.5 11.5 7M3 13.5h10" /></svg>
            {activity === 'export' ? 'Готовим файл…' : 'Экспорт в Excel'}
          </button>
          {menu}
        </div>
      </>} footer={<SirControls sir={sir} sirRows={sirRows} totalRows={includedRows.length} disabled={loading} onChange={onSirChange}>
        {/* Бюджет и подбор цен — в нижней строке справа; по СУР цены не подбираются. */}
        {!sirMode && <>
          <BudgetMeter budget={budget} totalUsd={notCalculated ? null : customsPaymentsUsd(result)} />
          <BudgetRunButton budget={budget} disabled={loading || pendingRun !== null} primary={noPrices && !hasCodeProblems} blockedReason={codeFirst ?? nothingToPrice} onRun={startRun} onUndo={onUndoPrices} canUndo={canUndoPrices} onRestore={restoreInitial} canRestore={replaceable > 0} />
        </>}
      </SirControls>} />

      {/* Пока есть ошибки строк, причина неудачного подбора — та же, что в баннере. */}
      {!sirMode && !blocking && <BudgetStatus budget={budget} />}
      {itpOpen && <ItpCalculator result={result} saved={itp} onSave={onItpChange} onClose={() => setItpOpen(false)} />}

      <div className={selectedRow ? 'rs-layout with-panel' : 'rs-layout'}>
        <section className="rs-card" aria-label="Строки расчёта">
          <div className="rs-filterbar">
            <div className="rs-tabs" role="tablist" aria-label="Какие строки показать">
              {tabs.map((tab) => <button key={tab.value} type="button" role="tab" aria-selected={filter === tab.value} className={filter === tab.value ? 'active' : undefined} onClick={() => setFilter(tab.value)} disabled={tab.count === 0 && filter !== tab.value}>
                {tab.value === 'ai' && <span className="ai-dot" aria-hidden="true" />}{tab.label} <b>{tab.count}</b>
              </button>)}
            </div>
            {filter === 'attention' && (counts.bad > 0 || counts.warn > 0) && <p className="rs-counts">
              {counts.bad > 0 && <span className="bad"><StateIcon state="bad" /> ошибки {counts.bad}</span>}
              {counts.warn > 0 && <span className="warn"><StateIcon state="warn" /> предупреждения {counts.warn}</span>}
            </p>}
            {changed.length > 0 && <p className="rs-changed" title="Цена единицы отличается от цены в Excel: правка или подбор под кассу">изменено цен: <b>{changed.length}</b></p>}
            <label className="rs-search">
              <svg width="15" height="15" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><circle cx="7" cy="7" r="4.5" /><path d="M10.5 10.5 14 14" /></svg>
              <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Товар или код" aria-label="Найти товар или код" />
            </label>
          </div>


          <div className={allColumns ? 'rs-table-wrap wide' : 'rs-table-wrap'} ref={tableWrapRef}>
            <table className="rs-table" ref={tableRef} style={allColumns ? { ['--rs-min' as string]: `${wideMinWidth}px` } : undefined}>
              <caption className="rs-sr">Строки расчёта: нажмите на строку, чтобы открыть подробности; ↑ ↓ — соседняя строка. Цену можно менять в таблице: Enter — следующая строка, Esc — вернуть цену</caption>
              <thead>
                <tr>
                  <th className="rs-num" scope="col">№</th>
                  <th className="rs-mark" scope="col"><span className="rs-sr">Состояние строки</span></th>
                  <th className="rs-name" scope="col">{copyNames}Товар</th>
                  <th className="rs-code" scope="col">{copyCodes}Код ТН ВЭД</th>
                  <th className="rs-qty" scope="col">{copyCol('Кол-во', (item) => item.pairs)}Кол-во</th>
                  {allColumns && (sirMode ? <>
                    <th className="rs-w" scope="col" title={sirGross ? 'Вес брутто — по нему считается ТС' : 'Вес нетто — по нему считается ТС'}>{copyCol(sirGross ? 'Брутто' : 'Нетто', (item) => sirGross ? item.grossKg : item.netKg)}{sirGross ? 'Брутто' : 'Нетто'}, кг</th>
                  </> : <th className="rs-w" scope="col">{copyCol('Нетто', (item) => item.netKg)}Нетто, кг</th>)}
                  {/* Главная колонка СУР — цена со снижением, по ней считается ТС; исходный индикатор — только во «Всех колонках». */}
                  {allColumns && sirMode && sirDiscount > 0 && <th scope="col" className="rs-sir-col" title="Стоимостной индикатор риска из файла профиля, USD за 1 кг">{copyCol('СУР', (item) => item.sir?.priceUsd)}СУР, USD/кг</th>}
                  {sirMode
                    ? <th scope="col" className="rs-sir-col" title={sirDiscount > 0 ? `Индикатор со снижением: берётся ${100 - sirDiscount} %` : 'Стоимостной индикатор риска из файла профиля, USD за 1 кг'}>{copyCol(sirDiscount > 0 ? 'СУР со снижением' : 'СУР', (item) => item.sir?.pricePerKgUsd)}{sirDiscount > 0 ? `СУР −${displayNumber(sir.sirDiscountPercent)}%` : 'СУР'}, USD/кг</th>
                    : <th scope="col" className="rs-price-col" title="Цену единицы можно менять прямо в таблице">{copyCol('Цена ед.', (_item, index) => priceOf(index))}Цена, USD</th>}
                  {sirMode
                    ? <th className="rs-usd" scope="col" title="Вес × СУР со снижением; у строк без индикатора — стоимость по инвойсу">{copyCol('ТС, USD', (item) => item.sir ? item.sir.valueUsd : item.goodsCost || item.costInput)}ТС, USD</th>
                    : <th className="rs-usd" scope="col">{copyCol('Стоимость', (item) => item.goodsCost || item.costInput)}Стоимость, USD</th>}
                  {allColumns && <>
                    {delivery && <th className="rs-usd" scope="col">{copyCol('Доставка', (item) => item.deliveryUsd)}Доставка, USD</th>}
                    <th className="rs-cv" scope="col" title="Таможенная стоимость">{copyCol('ТС, KZT', (item) => item.customsValueKzt)}ТС, KZT</th>
                    <th className="rs-tax" scope="col">{copyCol('Пошлина', (item) => item.dutyKzt)}Пошлина, KZT</th>
                    {antiDumping && <th className="rs-tax" scope="col">{copyCol('Антидемпинг', (item) => item.antiDumpingKzt)}Антидемпинг, KZT</th>}
                    <th className="rs-tax" scope="col">{copyCol('НДС', (item) => item.vatKzt)}НДС, KZT</th>
                    <th className="rs-fee" scope="col" title="Таможенный сбор">{copyCol('Сбор', (item) => item.feeKzt)}Сбор, KZT</th>
                  </>}
                  <th className="rs-pay" scope="col" title="Пошлина + антидемпинг + НДС + сбор">{copyCol('К оплате', (item) => rowPaymentKzt(item))}К оплате, KZT</th>
                </tr>
              </thead>
              <tbody>
                {visible.length === 0 && <tr className="rs-empty"><td colSpan={columns}>
                  {needle ? 'Ничего не найдено.' : filter === 'attention' ? 'Строк, требующих внимания, больше нет.' : 'Таких строк нет.'} <button type="button" className="button-link" onClick={() => { setFilter('all'); setQuery('') }}>Показать все строки</button>
                </td></tr>}
                {visible.map(({ item, index, state, problems, antiDumpingInfo, names, code, aiNote }) => {
                  const source = items[index]
                  // Значок и текст — только у проблем, которых нет у большинства строк (общие — в баннере).
                  const reason = problems.filter((problem) => !common.has(problem.text))
                  const tone = (['error', 'warn', 'todo'] as const).find((value) => reason.some((problem) => problem.tone === value))
                  const mark: Mark | null = item.excluded ? 'off' : tone ? MARK[tone] : null
                  const currency = item.goodsCostCurrency || item.costCurrency || 'USD'
                  const rowClass = [
                    `rs-row-${state}`,
                    item.excluded ? 'rv-excluded' : '',
                    selected === index ? 'selected' : '',
                    flashRow === index ? 'flash' : '',
                  ].filter(Boolean).join(' ')

                  return (
                    <tr key={`${item.productId}-${index}`} id={`rs-row-${index}`} className={rowClass} onClick={(event) => { const cell = (event.target as HTMLElement).closest('td'); if (cell && cell.cellIndex > 1) setActiveCol(cell.cellIndex); setSelected(index) }} aria-current={selected === index ? 'true' : undefined}>
                      <td className="rs-num">{index + 1}</td>
                      <td className="rs-mark">{mark && <span className={`rs-state ${mark}`} role="img" aria-label={MARK_LABEL[mark]} title={(item.excluded ? [] : reason).map((problem) => problem.text).join('\n') || MARK_LABEL[mark]}><StateIcon state={mark} /></span>}</td>
                      <td className="rs-name">
                        {/* Одна строка, как на этапе проверки: русское название и серым — китайское. «Все колонки» — только русское, полное — в подсказке. */}
                        <span className="rs-name-main" title={names.sub ? `${names.main} / ${names.sub}` : names.main}>{names.main}{names.sub && !allColumns && <span className="rs-name-sub"> · {names.sub}</span>}</span>
                        {reason.length > 0 && <span className={`rs-reason ${reason[0].tone}`} title={reason.map((problem) => problem.text).join('\n')}>{reason[0].text}{reason.length > 1 && ` и ещё ${reason.length - 1}`}</span>}
                      </td>
                      <td className="rs-code">{code ? <CopyCode code={code} /> : '—'}</td>
                      <td>{displayNumber(item.pairs)}</td>
                      {allColumns && <td>{fixedNumber(sirMode && sirGross ? item.grossKg : item.netKg, 1)}</td>}
                      {allColumns && sirMode && sirDiscount > 0 && <td className="rs-sir-col">{item.sir ? fixedNumber(item.sir.priceUsd, 2) : '—'}</td>}
                      {sirMode
                        ? <td className="rs-sir-col" title={item.sir ? `Профиль риска ${item.sir.profile}, страна ${item.sir.country}` : 'Для кода индикатора нет — строка считается по инвойсу'}>{item.sir ? fixedNumber(item.sir.pricePerKgUsd, 2) : <span className="rs-sir-none">нет</span>}</td>
                        : <td className="rs-price-col"><span className="price-field"><AiDot note={aiNote} />{priceInput(index, true, `Цена за единицу, USD, строка ${index + 1}: ${names.main}`)}</span></td>}
                      {sirMode
                        ? <td>{item.sir ? displayNumber(item.sir.valueUsd) : <>{displayNumber(item.goodsCost || item.costInput)}{currency !== 'USD' && <small> {currency}</small>}</>}</td>
                        : <td>{displayNumber(item.goodsCost || item.costInput)}{currency !== 'USD' && <small> {currency}</small>}</td>}
                      {allColumns && <>
                        {delivery && <td>{displayNumber(item.deliveryUsd)}</td>}
                        <td title={item.sir ? sirTitle(item.sir) : undefined}>{displayNumber(item.customsValueKzt)}</td>
                        <td onClick={(event) => event.stopPropagation()}>{displayNumber(item.dutyKzt)}{antiDumpingInfo && <AntiDumpingChooser info={antiDumpingInfo} choice={source?.antiDumpingChoice} disabled={loading || item.excluded} onChoose={(key) => onAntiDumpingChoice(index, key)} />}</td>
                        {antiDumping && <td title={antiDumpingInfo?.text}>{displayNumber(item.antiDumpingKzt)}</td>}
                        <td>{displayNumber(item.vatKzt)}</td>
                        <td>{displayNumber(item.feeKzt)}</td>
                      </>}
                      <td className="rs-pay">{displayNumber(rowPaymentKzt(item))}</td>
                    </tr>
                  )
                })}
              </tbody>
              <tfoot>
                <tr>
                  <td className="rs-num" />
                  <td className="rs-mark" />
                  <td className="rs-name">Итого{(filter !== 'all' || needle) && <small> · по всем строкам</small>}</td>
                  <td className="rs-code" />
                  <td>{displayNumber(checkTotals.quantity)}</td>
                  {allColumns && <td>{colTotal((item) => sirMode && sirGross ? item.grossKg : item.netKg, fixedNumber(sirMode && sirGross ? checkTotals.grossKg : checkTotals.netKg, 1))}</td>}
                  <td className={sirMode ? 'rs-sir-col' : 'rs-price-col'} />
                  {allColumns && sirMode && sirDiscount > 0 && <td className="rs-sir-col" />}
                  <td>{sirMode ? colTotal((item) => item.sir ? item.sir.valueUsd : item.goodsCost || item.costInput, displayNumber(moneyText(sirValueUsd))) : colTotal((item) => item.goodsCost || item.costInput, displayNumber(result.totals.totalCostUsd))}</td>
                  {allColumns && <>
                    {delivery && <td>{colTotal((item) => item.deliveryUsd, displayNumber(result.totals.totalDeliveryUsd))}</td>}
                    <td>{colTotal((item) => item.customsValueKzt, displayNumber(result.totals.customsValueKzt), true)}</td>
                    <td>{colTotal((item) => item.dutyKzt, displayNumber(result.totals.dutyKzt), true)}</td>
                    {antiDumping && <td>{colTotal((item) => item.antiDumpingKzt, displayNumber(result.totals.antiDumpingKzt), true)}</td>}
                    <td>{colTotal((item) => item.vatKzt, displayNumber(result.totals.vatKzt), true)}</td>
                    <td>{colTotal((item) => item.feeKzt, displayNumber(result.totals.feeKzt))}</td>
                  </>}
                  <td className="rs-pay">{notCalculated ? '—' : displayNumber(result.totals.customsPaymentsKzt)}</td>
                </tr>
              </tfoot>
            </table>
          </div>
        </section>

        {selectedRow && <>
          <div className="rs-backdrop" onClick={() => setSelected(null)} aria-hidden="true" />
          <RowPanel
            key={selectedRow.index}
            item={selectedRow.item}
            source={items[selectedRow.index]}
            index={selectedRow.index}
            problems={selectedRow.problems}
            antiDumpingInfo={selectedRow.antiDumpingInfo}
            position={selectedPosition >= 0 ? selectedPosition + 1 : selectedRow.index + 1}
            count={selectedPosition >= 0 ? visible.length : rows.length}
            onPrev={selectedPosition > 0 ? () => moveRef.current(-1) : undefined}
            onNext={selectedPosition >= 0 && selectedPosition < visible.length - 1 ? () => moveRef.current(1) : undefined}
            onClose={() => setSelected(null)}
            priceField={<label className="rp-price-field">{priceInput(selectedRow.index, false, 'Цена за штуку, USD')}<i>USD</i></label>}
            aiNote={selectedRow.aiNote}
            edited={!selectedRow.item.excluded && !selectedRow.aiNote && priceDiffers(items[selectedRow.index]?.sourcePricePerPair, priceOf(selectedRow.index))}
            sirMode={sirMode}
            loading={loading}
            onChangeCode={() => onChangeCode(selectedRow.index)}
            onToggleIncluded={(included) => onToggleIncluded(selectedRow.index, included)}
            onAntiDumpingChoice={(key) => onAntiDumpingChoice(selectedRow.index, key)}
          />
        </>}
      </div>
      {compareOpen && <SirCompareModal result={result} items={items} sir={sir} preview={previewSir} onClose={closeCompare} />}
    </div>
  )
}
