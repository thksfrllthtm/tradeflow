import { MouseEvent, ReactNode, RefObject } from 'react'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { CopyColumn } from '@/shared/components/CopyCode/CopyColumn'
import { excelColumn, excelColumnHtml } from '@/shared/lib/excelColumn'
import { moneyText } from '@/shared/lib/rounding'
import { GoodsCalculationResult, GoodsItemInput, SirParams } from '@/shared/types/goods'
import { AiDot } from '../BudgetTool/BudgetTool'
import { AntiDumpingChooser, AntiDumpingInfo } from './AntiDumpingChooser'
import { ProblemTone, RowProblem } from '../../lib/errorGroups'
import { displayNumber, fixedNumber, hasAntiDumping, itemDisplayName, parseCalculatorNumber, RowState, rowPaymentKzt, rowTotals, sirTitle, textColumn } from '../../lib/resultValues'

type ResultItem = GoodsCalculationResult['items'][number]

// Строка таблицы результата: состояние, проблемы, антидемпинг, названия, код и оценка цены ИИ.
export type ResultRow = { item: ResultItem; index: number; state: RowState; problems: RowProblem[]; antiDumpingInfo?: AntiDumpingInfo; names: { main: string; sub: string }; code: string; aiNote?: string; attention: boolean }

// Значок у номера: ошибка, предупреждение, следующий шаг (нет цены), исключена.
type Mark = 'bad' | 'warn' | 'todo' | 'off'
const MARK_LABEL: Record<Mark, string> = { bad: 'Ошибка', warn: 'Проверить', todo: 'Не указана цена', off: 'Исключена' }
const MARK: Record<ProblemTone, Mark> = { error: 'bad', warn: 'warn', todo: 'todo' }

// Значок состояния строки: форма отличается, а не только цвет (!, ?, –).
export function StateIcon({ state }: { state: RowState | Mark }) {
  return <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
    {state === 'ok' && <path d="M2.8 6.3 5 8.4l4.2-4.6" />}
    {state === 'bad' && <><path d="M6 2.8v4" /><circle cx="6" cy="9.2" r=".5" fill="currentColor" /></>}
    {state === 'warn' && <><path d="M4.4 4.4a1.7 1.7 0 1 1 2.4 1.5c-.5.3-.8.6-.8 1.2v.2" /><circle cx="6" cy="9.4" r=".5" fill="currentColor" /></>}
    {state === 'off' && <path d="M3.4 6h5.2" />}
    {state === 'todo' && <circle cx="6" cy="6" r="1.6" fill="currentColor" />}
  </svg>
}

type Props = {
  result: GoodsCalculationResult
  items: GoodsItemInput[]
  // Все строки (для копирования колонок) и показанные фильтром.
  rows: ResultRow[]
  visible: ResultRow[]
  // Проблемы большинства строк — они в баннере, у строк не повторяются.
  common: Set<string>
  allColumns: boolean
  sir: SirParams
  delivery: boolean
  notCalculated: boolean
  // Включён фильтр или поиск: в «Итого» — пометка «по всем строкам», пустой список — с текстом emptyText.
  filtered: boolean
  emptyText: string
  onShowAll: () => void
  selected: number | null
  flashRow: number | null
  loading: boolean
  onRowClick: (event: MouseEvent<HTMLTableRowElement>, index: number) => void
  priceOf: (index: number) => string
  priceInput: (index: number, inTable: boolean, label: string) => ReactNode
  onAntiDumpingChoice: (index: number, key: string) => void
  wrapRef: RefObject<HTMLDivElement | null>
  tableRef: RefObject<HTMLTableElement | null>
}

// Таблица строк результата: шапка с копированием колонок в Excel, строки и «Итого».
export function ResultTable({ result, items, rows, visible, common, allColumns, sir, delivery, notCalculated, filtered, emptyText, onShowAll, selected, flashRow, loading, onRowClick, priceOf, priceInput, onAntiDumpingChoice, wrapRef, tableRef }: Props) {
  const checkTotals = rowTotals(result)
  const antiDumping = hasAntiDumping(result)
  const sirMode = sir.customsValueMode === 'sir'
  const includedRows = result.items.filter((item) => !item.excluded)
  const sirGross = sir.sirWeight === 'gross'
  const sirDiscount = Number(sir.sirDiscountPercent || 0)
  const sirValueUsd = includedRows.reduce((total, item) => total + (parseCalculatorNumber(item.sir ? item.sir.valueUsd : item.costUsd) ?? 0), 0)
  // Копируются все строки, даже при фильтре: иначе в Excel значения съедут.
  const copyCol = (label: string, get: (item: GoodsCalculationResult['items'][number], index: number) => string | undefined) =>
    <CopyColumn label={label} text={() => excelColumn(result.items.map(get))} html={() => excelColumnHtml(result.items.map(get))} />
  const copyNames = <CopyColumn label="Товар" text={() => textColumn(result.items.map((item, index) => itemDisplayName(item, items[index])))} />
  // Код — только цифры, как у CopyCode.
  const copyCodes = <CopyColumn label="Код ТН ВЭД" text={() => textColumn(rows.map((row) => row.code.replace(/\D/g, '')))} />
  // «Все колонки»: ширины колонок, px (CSS), таблица не уже их суммы — «Товар» не уже 160 px и забирает остальное место.
  const wideMinWidth = 36 + 22 + 160 + 128 + 88 + 96 + 96 + 104 + (sirMode && sirDiscount > 0 ? 96 : 0) + (delivery ? 104 : 0) + 120 + 112 + (antiDumping ? 112 : 0) + 112 + 96 + 136
  // Итог колонки: значения нет ни у одной строки — «—», а не 0,00. calculatedOnly — суммы, которых нет у строк с ошибкой.
  const colTotal = (get: (item: GoodsCalculationResult['items'][number]) => string | undefined, total: string, calculatedOnly = false) =>
    includedRows.some((item) => !(calculatedOnly && item.errors?.length) && String(get(item) ?? '').trim() !== '') ? total : '—'
  // Стоимость, USD — и в обычном виде (8 колонок), остальные — только во «Всех колонках».
  const columns = 8 + (allColumns ? 1 + (sirMode && sirDiscount > 0 ? 1 : 0) + (delivery ? 1 : 0) + 4 + (antiDumping ? 1 : 0) : 0)

  return <div className={allColumns ? 'rs-table-wrap wide' : 'rs-table-wrap'} ref={wrapRef}>
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
        {visible.length === 0 && <tr className="rs-empty"><td className="dir-empty" colSpan={columns}>
          {emptyText} <button type="button" className="button-link" onClick={onShowAll}>Показать все строки</button>
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
            <tr key={index} id={`rs-row-${index}`} className={rowClass} onClick={(event) => onRowClick(event, index)} aria-current={selected === index ? 'true' : undefined}>
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
          <td className="rs-name">Итого{filtered && <small> · по всем строкам</small>}</td>
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
}
