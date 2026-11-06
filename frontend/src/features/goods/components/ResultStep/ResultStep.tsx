import { MouseEvent, ReactNode, useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { GoodsCalculationResult, GoodsItemInput, ItpSaved, SirParams } from '@/shared/types/goods'
import { BudgetMeter, BudgetRunButton, BudgetStatus, budgetAmount, RunMode } from '../BudgetTool/BudgetTool'
import { BudgetDistributionState } from '../../hooks/useBudgetDistribution'
import { changedFromExcel, changedFromRows, priceDiffers, shownPrice } from '../../lib/priceChanges'
import { storageFailed } from '../../lib/storageNotice'
import { CalculatorActivity } from '../../types'
import { CalculationErrors } from './CalculationErrors'
import { AntiDumpingInfo, antiDumpingTone } from './AntiDumpingChooser'
import { ItpCalculator } from './ItpCalculator'
import { ResultMenu } from './ResultMenu'
import { ResultSummary } from './ResultSummary'
import { ResultRow, ResultTable, StateIcon } from './ResultTable'
import { RowPanel } from './RowPanel'
import { SirControls } from './SirControls'
import { SirCompareModal } from './SirCompareModal'
import { usePriceInput } from './usePriceInput'
import { useTableFit } from './useTableFit'
import { plural } from '@/shared/lib/plural'
import { commonProblems, problemGroups, rowProblems } from '../../lib/errorGroups'
import { calculatedRows, customsPaymentsUsd, itemNames, parseCalculatorNumber, rowState } from '../../lib/resultValues'
import { SearchIcon } from '@/shared/components/icons'
import '../../goods.css'
import '@/shared/styles/list-page.css'
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
  // Чат подбора кода ИИ для строки (нет — кнопки нет).
  onAgent?: (index: number) => void
  // Панель «Подбор кода» открыта. Справа одна панель: с ней панель строки закрыта, а от 1440px под агента
  // оставлено место справа. Открыли панель строки — агент закрывается (onCloseAgent).
  agentOpen?: boolean
  onCloseAgent?: () => void
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
  onAgent,
  agentOpen = false,
  onCloseAgent,
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
  const [filter, setFilter] = useState<Filter>('all')
  const [query, setQuery] = useState('')
  const [allColumns, setAllColumns] = useState(storedAllColumns)
  // Выбранная строка (рамка ячейки, как в Excel) и открыта ли её карточка справа — отдельно: щелчок по цене,
  // количеству и другим ячейкам только выбирает ячейку (менять цену, копировать), карточку открывают щелчок по
  // номеру или наименованию, Enter и номер строки из плашки ошибок.
  const [selected, setSelected] = useState<number | null>(null)
  const [panelOpen, setPanelOpen] = useState(false)
  // Справа одна панель: открылся агент — панель строки закрывается; открылась панель строки — закрывается агент.
  useEffect(() => { if (agentOpen) setPanelOpen(false) }, [agentOpen])
  const closeAgentRef = useRef(onCloseAgent)
  closeAgentRef.current = agentOpen ? onCloseAgent : undefined
  useEffect(() => { if (panelOpen && selected !== null) closeAgentRef.current?.() }, [panelOpen, selected])
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
  const tableWrapRef = useRef<HTMLDivElement | null>(null)
  useTableFit(tableWrapRef, tableRef)
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

  // Колонка доставки нужна, только если доставка есть.
  const delivery = positive(result.totals.totalDeliveryUsd) || result.items.some((item) => positive(item.deliveryUsd))
  // По СУР цены инвойса на ТС не влияют: подбирать их под бюджет бессмысленно.
  const sirMode = sir.customsValueMode === 'sir'
  const includedRows = result.items.filter((item) => !item.excluded)
  const sirRows = includedRows.filter((item) => item.customsBasis === 'sir').length
  const calculating = activity === 'calculate'
  const { priceOf, priceInput } = usePriceInput({
    result, items, notes: budget.notes, sirMode, tableRef, onPriceChange, onPriceBlur,
    onFocusCell: (index, cellIndex) => { setSelected(index); if (cellIndex !== null) setActiveCol(cellIndex) },
  })

  // Цены нет ни у одной строки: следующий шаг — подбор цен (главная кнопка), выгружать ещё нечего. Тогда «нет цены»
  // не проблема строк: ни баннера, ни вкладки «Внимание», ни значков.
  const noPrices = !sirMode && includedRows.length > 0 && result.items.every((item, index) => item.excluded || !positive(item.pricePerPair || item.pairPrice || items[index]?.pricePerPair))
  const rows: ResultRow[] = result.items.map((item, index) => {
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
  const changed = changedFromExcel(items, result)

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
      if (!['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Enter'].includes(event.key)) return
      if (event.altKey || event.ctrlKey || event.metaKey || typing(event.target)) return
      // Стрелки в панели «Подбор кода» — её, а не таблицы.
      if (event.target instanceof Element && event.target.closest('.ag-panel')) return
      if (document.querySelector('[aria-modal="true"], [role="dialog"]')) return
      // Enter на выбранной строке (фокус не на кнопке или ссылке) — открыть её карточку.
      if (event.key === 'Enter') {
        if (event.target instanceof Element && event.target.closest('button, a, summary, [role="tab"]')) return
        setPanelOpen(true)
        return
      }
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

  function toggleAllColumns(value: boolean) {
    setAllColumns(value)
    try { localStorage.setItem(ALL_COLUMNS_KEY, value ? '1' : '0') } catch { /* без памяти — выбор на эту сессию */ }
  }

  // Номер строки из плашки ошибок: выбрать строку, прокрутить к ней, подсветить и поставить курсор в цену.
  function showRow(rowNumber: number) {
    const index = rowNumber - 1
    if (filter !== 'all' || query) { setFilter('all'); setQuery('') }
    setSelected(index)
    setPanelOpen(true)
    setFlashRow(index)
    window.requestAnimationFrame(() => {
      const row = document.getElementById(`rs-row-${index}`)
      row?.scrollIntoView({ block: 'center' })
      row?.querySelector<HTMLInputElement>('input.rs-price:not(:disabled)')?.focus({ preventScroll: true })
    })
  }

  // Щелчок по строке выбирает ячейку; по номеру или наименованию (не по полю, кнопке, ссылке) — открывает карточку строки.
  function rowClick(event: MouseEvent<HTMLTableRowElement>, index: number) {
    const target = event.target as HTMLElement
    const cell = target.closest('td')
    if (cell && cell.cellIndex > 1) setActiveCol(cell.cellIndex)
    setSelected(index)
    if (cell && cell.cellIndex <= 2 && !target.closest('input, button, a')) setPanelOpen(true)
  }

  function backToReview() {
    const count = changed.length
    const bulk = canUndoPrices || Object.keys(budget.notes).length > 0
    if (!window.confirm(count > 0
      ? `Цены этапа «Результат» вернутся к ценам из Excel: изменено ${count} ${plural(count, 'цена', 'цены', 'цен')}. Перейти к проверке строк?`
      : bulk ? 'Цены подбора под кассу сбросятся. Перейти к проверке строк?'
      : 'Перейти к проверке строк? Цены, изменённые на этапе «Результат», вернутся к ценам из Excel.')) return
    onBackToReview()
  }

  function restoreInitial() {
    // Пункт неактивен, пока менять нечего (replaceable = 0).
    if (!window.confirm(`Вернуть цены из файла? Изменится ${replaceable} ${plural(replaceable, 'цена', 'цены', 'цен')} после подбора под кассу; цены, введённые вручную, останутся. Вернуть подбор можно пунктом «Отменить подбор цен».`)) return
    onRestoreInitial()
  }

  // «Подобрать цены» из баннера: бюджет задан — подбор; нет — курсор в поле бюджета.
  // Подбор цен: ИИ (главная кнопка) или распределение под бюджет. Без бюджета — курсор в поле бюджета.
  // Цены прошлого подбора подбор не меняет (строки с ценой неподвижны), поэтому их замена — после
  // подтверждения: «Вернуть цены из файла», а когда пересчёт закончится — тот же подбор (pendingRun).
  function startRun(mode: RunMode) {
    if (budgetAmount(budget) === null) { document.getElementById('rs-budget')?.focus(); return }
    if (replaceable > 0) {
      if (!window.confirm(`Заменить текущие цены у ${replaceable} ${plural(replaceable, 'строки', 'строк', 'строк')}? Отменить можно через «Отменить подбор цен».`)) return
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
      adAsk > 0 && `антидемпинг не выбран: ${adAsk} ${plural(adAsk, 'строка', 'строки', 'строк')}`,
    ].filter(Boolean)
    if (notes.length > 0 && !window.confirm(`Перед выгрузкой: ${notes.join('; ')}. Выгрузить Excel всё равно?`)) return
    exportExcel()
  }

  const failed = storageFailed(storageNotice)
  const tabs: { value: Filter; label: string; count: number }[] = [
    { value: 'all', label: 'Все', count: rows.length },
    { value: 'attention', label: 'Внимание', count: counts.attention },
    { value: 'ai', label: 'Цена оценена', count: counts.ai },
  ]

  // Пока у строк нет кода ТН ВЭД, подбирать цены бессмысленно: пошлину и НДС не посчитать.
  const codeFirst = hasCodeProblems ? 'Сначала укажите коды ТН ВЭД' : undefined

  return (
    <div className={agentOpen ? 'goods-step-page rs-step with-agent' : 'goods-step-page rs-step'}>
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
          <button type="button" className="secondary" onClick={saveDraft} disabled={loading} title="Сохранить на сервере сейчас, не дожидаясь автосохранения (Ctrl+S)">{activity === 'save' ? 'Сохраняем…' : 'Сохранить'}</button>
          <button type="button" className={noPrices || notCalculated ? 'secondary' : undefined} onClick={exportChecked} disabled={loading || noPrices || notCalculated} title={noPrices || notCalculated ? 'Сначала завершите расчёт' : undefined}>
            <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M8 2.5v8M4.5 7 8 10.5 11.5 7M3 13.5h10" /></svg>
            {activity === 'export' ? 'Готовим файл…' : 'Экспорт в Excel'}
          </button>
          <ResultMenu loading={loading} itpSet={Boolean(itp?.value)} allColumns={allColumns} onCompare={() => setCompareOpen(true)} onItp={() => setItpOpen(true)} onAllColumns={toggleAllColumns} />
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

      <div className={selectedRow && panelOpen ? 'rs-layout with-panel' : 'rs-layout'}>
        <section className="rs-card" aria-label="Строки расчёта">
          <div className="rs-filterbar">
            <div className="dir-seg" role="tablist" aria-label="Какие строки показать">
              {tabs.map((tab) => <button key={tab.value} type="button" role="tab" aria-selected={filter === tab.value} className={filter === tab.value ? 'active' : undefined} onClick={() => setFilter(tab.value)} disabled={tab.count === 0 && filter !== tab.value}>
                {tab.value === 'ai' && <span className="ai-dot" aria-hidden="true" />}{tab.label} <span className="dir-count">{tab.count}</span>
              </button>)}
            </div>
            {filter === 'attention' && (counts.bad > 0 || counts.warn > 0) && <p className="rs-counts">
              {counts.bad > 0 && <span className="bad"><StateIcon state="bad" /> ошибки {counts.bad}</span>}
              {counts.warn > 0 && <span className="warn"><StateIcon state="warn" /> предупреждения {counts.warn}</span>}
            </p>}
            {changed.length > 0 && <p className="rs-changed" title="Цена единицы отличается от цены в Excel: правка или подбор под кассу">изменено цен: <b>{changed.length}</b></p>}
            <label className="dir-search rs-search">
              <SearchIcon />
              <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Товар или код" aria-label="Найти товар или код" />
            </label>
          </div>

          <ResultTable result={result} items={items} rows={rows} visible={visible} common={common} allColumns={allColumns} sir={sir} delivery={delivery} notCalculated={notCalculated}
            filtered={filter !== 'all' || Boolean(needle)} emptyText={needle ? 'Ничего не найдено.' : filter === 'attention' ? 'Строк, требующих внимания, больше нет.' : 'Таких строк нет.'} onShowAll={() => { setFilter('all'); setQuery('') }}
            selected={selected} flashRow={flashRow} loading={loading} onRowClick={rowClick} priceOf={priceOf} priceInput={priceInput} onAntiDumpingChoice={onAntiDumpingChoice} wrapRef={tableWrapRef} tableRef={tableRef} />
        </section>

        {selectedRow && panelOpen && <>
          <div className="rs-backdrop" onClick={() => setPanelOpen(false)} aria-hidden="true" />
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
            onClose={() => setPanelOpen(false)}
            priceField={<label className="rp-price-field">{priceInput(selectedRow.index, false, 'Цена за штуку, USD')}<i>USD</i></label>}
            aiNote={selectedRow.aiNote}
            edited={!selectedRow.item.excluded && !selectedRow.aiNote && priceDiffers(items[selectedRow.index]?.sourcePricePerPair, priceOf(selectedRow.index))}
            sirMode={sirMode}
            loading={loading}
            onChangeCode={() => onChangeCode(selectedRow.index)}
            onAgent={onAgent ? () => { const index = selectedRow.index; setPanelOpen(false); onAgent(index) } : undefined}
            onToggleIncluded={(included) => onToggleIncluded(selectedRow.index, included)}
            onAntiDumpingChoice={(key) => onAntiDumpingChoice(selectedRow.index, key)}
          />
        </>}
      </div>
      {compareOpen && <SirCompareModal result={result} items={items} sir={sir} preview={previewSir} onClose={closeCompare} />}
    </div>
  )
}
