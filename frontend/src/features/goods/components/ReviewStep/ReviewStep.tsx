import { ChangeEvent, Dispatch, ReactNode, SetStateAction, useEffect, useRef, useState } from 'react'
import { GoodsCalculationResult, GoodsItemInput, GoodsParams } from '@/shared/types/goods'
import { photosOf } from '@/shared/lib/itemPhotos'
import { reviewStatus, rowMissing } from '../ReviewRow/reviewStatus'
import { ReviewCheck } from '../../lib/reviewCheck'
import { problemChips, reviewProblems } from '../../lib/reviewProblems'
import { trademarkProblems, useTrademarkHits } from '../../lib/trademarkCheck'
import { reviewTotals } from './reviewTotals'
import { useStableCallback } from '@/shared/hooks/useStableCallback'
import { AgentRowStatus } from '../../agent/agentApi'
import { storageFailed } from '../../lib/storageNotice'
import { RowDecisionState } from '../../hooks/useRowDecisions'
import { emptyItem } from '../../lib/rowActions'
import { rowId } from '../../lib/excelColumns'
import { lookupNomenclature, withNomenclatureRates } from '../../lib/nomenclatureRates'
import { itemNames } from '../ResultStep/resultValues'
import { ReviewSheet, RowAction, SheetApi, SheetRowView } from '../ReviewSheet/ReviewSheet'
import { applySheetChanges, SheetChange } from '../ReviewSheet/sheetModel'
import { ParamChips } from './ParamChips'
import { SearchIcon } from '@/shared/components/icons'
import '../../goods.css'
import '@/shared/styles/list-page.css'
import './ReviewStep.css'

type Props = {
  params: GoodsParams
  items: GoodsItemInput[]
  result: GoodsCalculationResult | null
  // Ошибки последнего расчёта по id строк: показываются, пока нет нового расчёта.
  reviewCheck: ReviewCheck | null
  selectedForDirectory: string[]
  loading: boolean
  usdRateLoading: boolean
  usdRateNotice: string
  storageNotice: string
  // Есть правки, которых ещё нет на сервере.
  unsaved: boolean
  // Название сметы, дата, клиент и этапы — слева в строке заголовка.
  header: ReactNode
  includedCount: number
  // Новые строки сметы из таблицы (правка, вставка, отмена); сбрасывает результат расчёта.
  editItems: (items: GoodsItemInput[]) => void
  setSelectedForDirectory: Dispatch<SetStateAction<string[]>>
  setParam: (field: keyof GoodsParams, value: string) => void
  loadUsdRate: () => void
  openDirectoryPicker: (ids: string[]) => void
  removeRow: (id: string) => void
  // Последняя удалённая строка: её можно вернуть на место.
  removedRow: { rowNumber: number; name: string } | null
  restoreRemovedRow: () => void
  dismissRemovedRow: () => void
  openRowPhotos: (id: string, index: number) => void
  addManualPhoto: (id: string, event: ChangeEvent<HTMLInputElement>) => void
  addEmptyRow: () => void
  calculate: () => void
  saveDraft: () => void
  clearAll: () => void
  // Подбор кода ИИ: состояние по строкам, открыть чат строки, подобрать для всех строк без кода.
  agentStatuses: Record<string, AgentRowStatus>
  // База решений: состояние кнопки «В базу» строки и сохранение строк.
  rowDecisions: { stateOf: (item: GoodsItemInput) => RowDecisionState; save: (ids: string[]) => Promise<{ created: number; duplicates: number; skipped: number; failed: number }> }
  agentBatch: { done: number; total: number } | null
  openAgent: (id: string) => void
  startAgentBatch: () => void
  // Строка, открытая в панели «Подбор кода»: выделена в листе.
  agentRowId?: string | null
}

// Фильтр строк: все, требующие внимания, проблема одного типа (чип), код от ИИ.
type Filter = 'all' | 'attention' | 'ai' | `type:${string}`
type Density = 'full' | 'compact'
const DENSITY_KEY = 'tradeflow.review-density'
// Выбор сотрудника запоминается; пока его нет — на низком экране (ноутбук) компактно.
const storedDensity = (): Density => {
  try {
    const saved = localStorage.getItem(DENSITY_KEY)
    if (saved === 'compact' || saved === 'full') return saved
  } catch { /* без памяти — по высоте окна */ }
  return window.innerHeight < 900 ? 'compact' : 'full'
}

const checkTime = (value: string) => {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

// Отмена и повтор по строкам сметы (Ctrl+Z / Ctrl+Y в таблице): запоминается каждое изменение
// строк — правка ячеек, вставка, выбор кода, фото, удаление. mergeNext — следующее изменение
// дополняет последний шаг (ставки, подтянутые к введённому коду, не отдельный шаг отмены).
function useItemsHistory(items: GoodsItemInput[], replace: (items: GoodsItemInput[]) => void) {
  const state = useRef({ past: [] as GoodsItemInput[][], future: [] as GoodsItemInput[][], last: items, skip: false, merge: false })
  useEffect(() => {
    const history = state.current
    if (items === history.last) return
    if (!history.skip && !history.merge) {
      history.past.push(history.last)
      if (history.past.length > 100) history.past.shift()
      history.future = []
    }
    history.skip = false
    history.merge = false
    history.last = items
  }, [items])
  return {
    undo() {
      const history = state.current
      const previous = history.past.pop()
      if (!previous) return
      history.future.push(history.last)
      history.skip = true
      replace(previous)
    },
    redo() {
      const history = state.current
      const next = history.future.pop()
      if (!next) return
      history.past.push(history.last)
      history.skip = true
      replace(next)
    },
    mergeNext() { state.current.merge = true },
  }
}

// Этап 2 «Проверка строк»: строка заголовка, чипы параметров, панель фильтров, строка значения и
// лист «как в Excel». Прокручивается страница: шапка листа прилипает к верху, «Итого» — к низу окна.
export function ReviewStep({ params, items, result, reviewCheck, selectedForDirectory, loading, usdRateLoading, usdRateNotice, storageNotice, unsaved, header, includedCount, editItems, setSelectedForDirectory, setParam, loadUsdRate, openDirectoryPicker, removeRow, removedRow, restoreRemovedRow, dismissRemovedRow, openRowPhotos, addManualPhoto, addEmptyRow, calculate, saveDraft, clearAll, agentStatuses, rowDecisions, agentBatch, openAgent, startAgentBatch, agentRowId }: Props) {
  const [filter, setFilter] = useState<Filter>('all')
  const [query, setQuery] = useState('')
  const [density, setDensity] = useState<Density>(storedDensity)
  // Колонки «Упаковка» и «Фото», включённые в меню «⋯» (иначе — только когда нужны).
  const [forcePackage, setForcePackage] = useState(false)
  const [forcePhoto, setForcePhoto] = useState(false)
  const sheetApi = useRef<SheetApi | null>(null)
  const menuRef = useRef<HTMLDetailsElement>(null)
  const itemsRef = useRef(items)
  itemsRef.current = items
  const history = useItemsHistory(items, editItems)
  const trademarkHits = useTrademarkHits(items)
  const failed = storageFailed(storageNotice)
  const setSelectedExcluded = (excluded: boolean) => editItems(items.map((item) => selectedForDirectory.includes(item.id ?? '') ? { ...item, excluded } : item))

  // Статус и проблемы каждой строки — для чипов, фильтра, точки у номера и подсветки ячеек.
  // Код от ИИ — подбор закончен и предложил ровно этот код (поменяли код — метки нет). Это не проблема.
  const rows = items.map((item, index) => {
    const id = item.id ?? ''
    const status = reviewStatus(item, result?.items[index], result ? undefined : reviewCheck?.rows[id], rowMissing(item, params.packageWeightKg))
    const agent = agentStatuses[id]
    const aiCode = Boolean(item.code) && agent?.status === 'done' && agent.bestCode === item.code
    return { item, index, id, status, problems: [...reviewProblems(item, status), ...(item.excluded ? [] : trademarkProblems(trademarkHits[id]))], aiCode }
  })
  const counts = {
    attention: rows.filter((row) => row.status.attention).length,
    noCode: rows.filter((row) => row.status.noCode).length,
    changed: rows.filter((row) => row.status.tone === 'changed').length,
    ai: rows.filter((row) => row.aiCode).length,
  }
  const chips = problemChips(rows.map((row) => row.problems))
  const needle = query.trim().toLowerCase()
  const needleDigits = needle.replace(/\D/g, '')
  const shownInFilter = (row: (typeof rows)[number]) => {
    if (filter === 'attention' && !row.status.attention) return false
    if (filter === 'ai' && !row.aiCode) return false
    if (filter.startsWith('type:') && !row.problems.some((problem) => `type:${problem.type}` === filter)) return false
    if (!needle) return true
    const names = itemNames(row.item, row.item)
    return `${names.main} ${names.sub} ${row.item.name}`.toLowerCase().includes(needle) || (needleDigits.length > 0 && row.item.code.includes(needleDigits))
  }
  const visible = rows.filter(shownInFilter)
  const visibleIds = visible.map((row) => row.id)
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selectedForDirectory.includes(id))
  // Откуда ошибки: актуальный расчёт или прошлый (строки с тех пор менялись).
  const pastCheck = !result && reviewCheck && Object.keys(reviewCheck.rows).length > 0 ? reviewCheck : null

  // В листе все строки сметы по порядку; фильтр их только скрывает.
  const selectedIds = new Set(selectedForDirectory)
  const sheetRows: SheetRowView[] = rows.map((row) => ({ item: row.item, status: row.status, problems: row.problems, aiCode: row.aiCode, visible: shownInFilter(row), selected: selectedIds.has(row.id), agent: agentStatuses[row.id], decision: rowDecisions.stateOf(row.item) }))

  // Правки листа (и строки значения) → строки сметы и ставки для введённых кодов.
  function applyChanges(changes: SheetChange[]) {
    const current = itemsRef.current
    const applied = applySheetChanges(current, changes, params.packageWeightKg, (index) => rowId(0, index))
    if (applied.items) editItems(applied.items)
    if (applied.codes.size > 0) void loadCodeRates(applied.codes)
  }

  // Ставки введённого кода — пошлина ЕТТ и НДС из номенклатуры, как при выборе кода в справочнике.
  async function loadCodeRates(codes: Map<string, string>) {
    const found = await lookupNomenclature([...codes.values()]).catch(() => null)
    if (!found) return
    let changed = false
    const next = itemsRef.current.map((item) => {
      const code = codes.get(item.id ?? '')
      const nomenclature = code && item.code === code ? found.byCode.get(code) : undefined
      if (!nomenclature) return item
      changed = true
      const snapshot = withNomenclatureRates(nomenclature, found.source, { code })
      return { ...item, standardName: String(snapshot.name ?? ''), directorySnapshot: snapshot }
    })
    if (!changed) return
    history.mergeNext()
    editItems(next)
  }

  // Фото добавляется скрытым полем выбора файла: кнопка «+» в ячейке открывает его для строки.
  const photoInput = useRef<HTMLInputElement>(null)
  const photoTarget = useRef('')
  function rowAction(action: RowAction, index: number) {
    const id = items[index]?.id
    if (!id) return
    if (action === 'select') setSelectedForDirectory((current) => current.includes(id) ? current.filter((value) => value !== id) : [...current, id])
    else if (action === 'photos') openRowPhotos(id, 0)
    else if (action === 'upload') { photoTarget.current = id; photoInput.current?.click() }
    else if (action === 'picker') openDirectoryPicker([id])
    else if (action === 'agent') openAgent(id)
    else if (action === 'decision') void rowDecisions.save([id])
  }

  function insertRow(index: number) {
    const current = itemsRef.current
    editItems([...current.slice(0, index), emptyItem(rowId(0, current.length), params.packageWeightKg), ...current.slice(index)])
  }

  // Одна строка — с сообщением «Вернуть»; несколько — одним шагом, вернуть можно Ctrl+Z.
  function deleteRows(indexes: number[]) {
    const ids = indexes.map((index) => items[index]?.id).filter((id): id is string => Boolean(id))
    if (ids.length === 1) { removeRow(ids[0]); setSelectedForDirectory((current) => current.filter((id) => id !== ids[0])); return }
    if (ids.length === 0 || !window.confirm(`Удалить строки: ${ids.length}? Вернуть можно Ctrl+Z.`)) return
    dismissRemovedRow()
    editItems(itemsRef.current.filter((item) => !ids.includes(item.id ?? '')))
    setSelectedForDirectory((current) => current.filter((id) => !ids.includes(id)))
  }

  function toggleAllVisible() {
    setSelectedForDirectory((current) => allVisibleSelected ? current.filter((id) => !visibleIds.includes(id)) : [...new Set([...current, ...visibleIds])])
  }

  // Итог сохранения отмеченных строк в базу решений.
  const [decisionNote, setDecisionNote] = useState('')
  async function saveSelectedDecisions() {
    setDecisionNote('Сохраняем в базу решений…')
    const saved = await rowDecisions.save(selectedForDirectory)
    setDecisionNote([
      `В базу решений: сохранено ${saved.created}`,
      saved.duplicates > 0 && `уже были в этой смете ${saved.duplicates}`,
      saved.skipped > 0 && `без кода или уже в базе ${saved.skipped}`,
      saved.failed > 0 && `ошибка ${saved.failed}`,
    ].filter(Boolean).join(', '))
  }
  const agentPending = items.filter((item) => item.id && !item.excluded && !item.code && (!agentStatuses[item.id] || agentStatuses[item.id].status === 'ready')).length
  const agentStarted = counts.noCode - agentPending
  const ownPackageCount = items.filter((item) => item.packageWeightKg !== params.packageWeightKg).length
  const showPackage = forcePackage || ownPackageCount > 0
  const showPhoto = forcePhoto || items.some((item) => photosOf(item).length > 0)
  const totals = reviewTotals(items, params.packageWeightKg)

  function chooseDensity(value: Density) {
    setDensity(value)
    try { localStorage.setItem(DENSITY_KEY, value) } catch { /* без памяти — выбор на эту сессию */ }
  }

  // Следующая строка, требующая внимания, после строки с курсором (по кругу): прокрутить и
  // поставить курсор в ячейку с проблемой.
  function nextProblem() {
    const problems = visible.filter((row) => row.status.attention)
    if (problems.length === 0) return
    const at = sheetApi.current?.currentRow() ?? -1
    const row = problems.find((entry) => entry.index > at) ?? problems[0]
    sheetApi.current?.focusCell(row.index, row.problems[0]?.column ?? 'name')
  }

  // Ctrl+S — сохранить сейчас, F8 — следующая строка, требующая внимания.
  const keysRef = useRef({ saveDraft, nextProblem })
  keysRef.current = { saveDraft, nextProblem }
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 's') { event.preventDefault(); keysRef.current.saveDraft() }
      else if (event.key === 'F8') { event.preventDefault(); keysRef.current.nextProblem() }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // Меню «⋯» закрывается щелчком мимо и клавишей Esc.
  useEffect(() => {
    const close = (event: Event) => {
      const menu = menuRef.current
      if (!menu?.open) return
      if (event instanceof globalThis.KeyboardEvent) { if (event.key === 'Escape') { menu.open = false; menu.querySelector('summary')?.focus() } }
      else if (!event.composedPath().includes(menu)) menu.open = false
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', close)
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', close) }
  }, [])
  const closeMenu = () => { if (menuRef.current) menuRef.current.open = false }

  // Сообщение об удалённой строке исчезает само через 12 секунд.
  const onDismissRemoved = useStableCallback(dismissRemovedRow)
  useEffect(() => {
    if (!removedRow) return
    const timer = window.setTimeout(onDismissRemoved, 12000)
    return () => window.clearTimeout(timer)
  }, [removedRow, onDismissRemoved])

  const tab = (value: Filter, label: string, count: number, hint: string) => (
    <button type="button" role="tab" aria-selected={filter === value} className={filter === value ? 'active' : undefined} onClick={() => setFilter(value)} disabled={count === 0 && filter !== value} title={hint}>{label} <span className="dir-count">{count}</span></button>
  )

  const menu = <details className="rv-more" ref={menuRef}>
    <summary className="icon-button" title="Ещё" aria-label="Ещё действия">
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="currentColor"><circle cx="3.5" cy="8" r="1.3" /><circle cx="8" cy="8" r="1.3" /><circle cx="12.5" cy="8" r="1.3" /></svg>
    </summary>
    <div className="rv-menu">
      <button type="button" className="rv-menu-item" onClick={() => { closeMenu(); addEmptyRow() }} disabled={loading}>+ Строка</button>
      <hr />
      <p className="rv-menu-label">Колонки</p>
      <label className="rv-menu-item rv-menu-check" title={ownPackageCount > 0 ? 'У строк есть своя упаковка — колонка видна всегда' : 'Вес упаковки одного места для авто-нетто'}><input type="checkbox" checked={showPackage} disabled={ownPackageCount > 0} onChange={(event) => setForcePackage(event.target.checked)} />Упаковка</label>
      <label className="rv-menu-item rv-menu-check" title={showPhoto && !forcePhoto ? 'У строк есть фото — колонка видна всегда' : 'Фото строк'}><input type="checkbox" checked={showPhoto} disabled={showPhoto && !forcePhoto} onChange={(event) => setForcePhoto(event.target.checked)} />Фото</label>
      <hr />
      <p className="rv-menu-label">Вид</p>
      <div className="dir-seg" role="radiogroup" aria-label="Плотность таблицы">
        <button type="button" role="radio" aria-checked={density === 'full'} className={density === 'full' ? 'active' : undefined} onClick={() => chooseDensity('full')} title="Выше строки и крупнее фото">Подробно</button>
        <button type="button" role="radio" aria-checked={density === 'compact'} className={density === 'compact' ? 'active' : undefined} onClick={() => chooseDensity('compact')} title="Одна строка на товар: больше строк на экране">Компактно</button>
      </div>
      <hr />
      <button type="button" className="rv-menu-item danger" onClick={() => { closeMenu(); clearAll() }} disabled={loading || items.length === 0}>Очистить смету…</button>
    </div>
  </details>

  return <div className={`goods-step-page rv-step${density === 'compact' ? ' compact' : ''}`}>
    <div className="estimate-bar rv-titlebar">{header}</div>

    {/* Панель расчёта: слева параметры, справа сохранение и «Рассчитать». */}
    <div className="rv-toolbar rv-calcbar">
      <ParamChips params={params} setParam={setParam} loadUsdRate={loadUsdRate} usdRateLoading={usdRateLoading} usdRateNotice={usdRateNotice} ownPackageCount={ownPackageCount} />
      <div className="rv-title-actions">
        {unsaved && !failed && !storageNotice.startsWith('Сохраняем')
          ? <span className="rv-save-state unsaved" role="status" title="Есть несохранённые изменения">Есть несохранённые изменения</span>
          : <span className={failed ? 'rv-save-state failed' : 'rv-save-state'} role={failed ? 'alert' : 'status'} title={storageNotice || 'Изменения сохраняются автоматически'}>{storageNotice || 'Изменения сохраняются автоматически'}</span>}
        <button type="button" className="secondary" onClick={() => void saveDraft()} disabled={loading || items.length === 0} title="Сохранить на сервере сейчас (Ctrl+S)">Сохранить</button>
        <button type="button" onClick={() => void calculate()} disabled={loading || includedCount === 0}>{loading ? 'Считаем…' : 'Рассчитать'} <span aria-hidden="true">→</span></button>
      </div>
    </div>

    <section className="rv-card" aria-label="Строки сметы">
      {selectedForDirectory.length > 0 ? <div className="rv-panel selection">
        <strong>Выбрано: {selectedForDirectory.length}</strong>
        <button type="button" className="secondary" onClick={() => openDirectoryPicker(selectedForDirectory)}>Сменить код</button>
        <button type="button" className="secondary" onClick={() => void saveSelectedDecisions()} title="Записать отмеченные строки с кодом в базу решений; тот же товар этой сметы записывается один раз">В базу решений</button>
        <button type="button" className="secondary" onClick={() => setSelectedExcluded(true)}>Исключить</button>
        <button type="button" className="secondary" onClick={() => setSelectedExcluded(false)}>Включить</button>
        <button type="button" className="danger" onClick={() => deleteRows(items.flatMap((item, index) => selectedIds.has(item.id ?? '') ? [index] : []))}>Удалить</button>
        <button type="button" className="button-link" onClick={() => { setSelectedForDirectory([]); setDecisionNote('') }}>Снять выбор</button>
        {decisionNote && <span className="rv-panel-note" role="status">{decisionNote}</span>}
      </div> : <div className="rv-panel">
        <div className="dir-seg" role="tablist" aria-label="Какие строки показать">
          {tab('all', 'Все', items.length, 'Все строки сметы')}
          {tab('attention', 'Требуют внимания', counts.attention, 'Ошибки и предупреждения расчёта, строки без кода, изменённые после расчёта строки с ошибками')}
        </div>
        {/* Пилюли проблем в одну строку: не помещаются — прокручиваются вбок, ряд не растёт. */}
        <span className="rv-chips">
          {chips.length === 0 && counts.attention === 0 && items.length > 0 && <span className="rv-all-ok">✓ Все коды указаны, количества и вес сходятся</span>}
          {chips.map((chip) => <button key={chip.type} type="button" className={[`rv-pchip ${chip.tone}`, filter === `type:${chip.type}` ? 'active' : ''].filter(Boolean).join(' ')} onClick={() => setFilter(filter === `type:${chip.type}` ? 'all' : `type:${chip.type}`)} aria-pressed={filter === `type:${chip.type}`} title={chip.label}>
            <i aria-hidden="true" />{chip.label} · {chip.rows}
          </button>)}
          {counts.ai > 0 && <button type="button" className={filter === 'ai' ? 'rv-pchip neutral active' : 'rv-pchip neutral'} onClick={() => setFilter(filter === 'ai' ? 'all' : 'ai')} aria-pressed={filter === 'ai'} title="Строки, где код предложил подбор ИИ — просмотреть и подтвердить">Код от ИИ · {counts.ai}</button>}
        </span>
        {counts.attention > 0 && <button type="button" className="secondary sm rv-next" onClick={nextProblem} aria-label="Следующая проблема (F8)" title="Следующая проблема (F8)">
          <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M8 3v10M3.5 8.5 8 13l4.5-4.5" /></svg><kbd>F8</kbd>
        </button>}
        <span className="rv-panel-end">
          {(result || pastCheck) && <span className={pastCheck ? 'rv-check-source past' : 'rv-check-source'} title={result ? 'Статусы строк — по актуальному расчёту' : 'Строки менялись после расчёта: ошибки — из прошлого расчёта, нужен пересчёт'}>
            {result ? 'Актуальный расчёт' : <>Прошлый расчёт{pastCheck && checkTime(pastCheck.at) && <> <time dateTime={pastCheck.at}>{checkTime(pastCheck.at)}</time></>}{counts.changed > 0 && <> · изменено {counts.changed}</>} · нужен пересчёт</>}
          </span>}
          {agentBatch ? <span className="rv-panel-note">Запуск подбора: {agentBatch.done} из {agentBatch.total}</span>
            : agentPending > 0 && <button type="button" className="secondary sm rv-ai-batch" onClick={startAgentBatch} disabled={loading} title={`Подобрать коды для включённых строк без кода, где подбор ещё не запускали${agentStarted > 0 ? ` (ещё ${agentStarted} уже в подборе)` : ''}; результат — в чате строки`}>Подобрать коды ИИ ({agentPending})</button>}
          <label className="dir-search rv-search">
            <SearchIcon />
            <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Товар или код" aria-label="Найти товар или код" />
          </label>
          {menu}
        </span>
      </div>}

      {removedRow && <div className="rv-undo" role="status">
        <span>Строка {removedRow.rowNumber}{removedRow.name ? ` «${removedRow.name.length > 60 ? `${removedRow.name.slice(0, 60)}…` : removedRow.name}»` : ''} удалена</span>
        <button type="button" className="button-link" onClick={restoreRemovedRow}>Вернуть</button>
        <button type="button" className="icon-button ghost sm rv-undo-close" onClick={dismissRemovedRow} aria-label="Закрыть сообщение"><svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M4.5 4.5l7 7M11.5 4.5l-7 7" /></svg></button>
      </div>}

      {items.length === 0 && <p className="dir-empty rv-empty-filter">Строк пока нет. Загрузите XLSX или выберите «+ Строка» в меню «⋯» — в неё можно вставить строки из Excel (Ctrl+V).</p>}
      {items.length > 0 && visible.length === 0 && <p className="dir-empty rv-empty-filter">В этом фильтре строк нет. <button type="button" className="button-link" onClick={() => { setFilter('all'); setQuery('') }}>Показать все строки</button></p>}
      <ReviewSheet rows={sheetRows} defaultPackageWeight={params.packageWeightKg} showPhoto={showPhoto} showPackage={showPackage} compact={density === 'compact'} apiRef={sheetApi}
        totals={{ ...totals, included: includedCount, all: items.length }}
        onChanges={applyChanges} onAction={rowAction} onSelectAll={toggleAllVisible} onInsertRow={insertRow} onDeleteRows={deleteRows}
        agentRowId={agentRowId} onPickRows={(indexes) => openDirectoryPicker(indexes.map((index) => items[index]?.id ?? '').filter(Boolean))} onUndo={history.undo} onRedo={history.redo} />
      <input ref={photoInput} type="file" accept="image/jpeg,image/png" style={{ display: 'none' }} onChange={(event) => { if (photoTarget.current) addManualPhoto(photoTarget.current, event) }} />
    </section>
  </div>
}
