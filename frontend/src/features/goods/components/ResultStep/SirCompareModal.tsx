import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { useEffect, useRef, useState } from 'react'
import { Modal } from '@/shared/components/Modal/Modal'
import { createPortal } from 'react-dom'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { decimalText, moneyText, weightText } from '@/shared/lib/rounding'
import { GoodsCalculationResult, GoodsItemInput, SirParams, SirWeight } from '@/shared/types/goods'
import { compareCounts, compareRows, compareTotals, SirCompareRow, totalOf } from '../../lib/sirCompare'
import { itemDisplayName } from '../../lib/resultValues'
import { SirOptions } from './SirControls'
import './SirCompareModal.css'

type Props = {
  result: GoodsCalculationResult
  items: GoodsItemInput[]
  sir: SirParams
  // Пробный расчёт сметы с другими параметрами СУР: смета и черновик не меняются.
  preview: (patch: SirParams) => Promise<GoodsCalculationResult>
  onClose: () => void
}

type Order = 'smeta' | 'diff'

const dash = '—'
const money = (value: number | null) => value === null ? dash : formatRuNumber(moneyText(value))
const perKg = (value: number | null) => value === null ? dash : formatRuNumber(decimalText(value, 4))
const signed = (value: number | null) => value === null ? dash : `${value > 0 ? '+' : value < 0 ? '−' : ''}${formatRuNumber(moneyText(Math.abs(value)))}`
const percent = (value: number) => formatRuNumber(decimalText(value, 1))
// По СУР дороже — тёплым, дешевле — зелёным; ноль — без цвета.
const tone = (value: number | null) => value === null || Math.abs(value) < 0.005 ? '' : value > 0 ? 'up' : 'down'
const discountLabel = (discount: string) => Number(discount) > 0 ? ` −${formatRuNumber(discount)}%` : ''

// Окно «Сравнение: по инвойсу и по СУР»: сверху растаможка к оплате обоими способами и разница,
// под ней — из чего она складывается, ниже — каждая строка. Вес и снижение СУР пробуются здесь же
// и меняют только окно; режим сметы переключается в полосе «Таможенная стоимость».
// Окно — в document.body: общий мелкий стиль таблиц страницы сметы на него не действует.
export function SirCompareModal({ result, items, sir, preview, onClose }: Props) {
  const currentMode = sir.customsValueMode === 'sir' ? 'sir' : 'invoice'
  const currentWeight: SirWeight = sir.sirWeight === 'gross' ? 'gross' : 'net'
  const currentDiscount = sir.sirDiscountPercent ?? ''
  const [weight, setWeight] = useState<SirWeight>(currentWeight)
  const [discount, setDiscount] = useState(currentDiscount)
  const [order, setOrder] = useState<Order>('smeta')
  const [invoice, setInvoice] = useState<GoodsCalculationResult | null>(currentMode === 'invoice' ? result : null)
  const [byIndicator, setByIndicator] = useState<GoodsCalculationResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const requestRef = useRef(0)
  const previewRef = useRef(preview)
  previewRef.current = preview
  const sameAsSmeta = currentMode === 'sir' && weight === currentWeight && Number(discount || 0) === Number(currentDiscount || 0)

  // Расчёт по инвойсу: в режиме инвойса — текущий результат, иначе один пробный расчёт.
  useEffect(() => {
    if (currentMode === 'invoice') return
    previewRef.current({ customsValueMode: 'invoice' }).then(setInvoice).catch((reason) => setError(errorMessage(reason)))
  }, [currentMode])

  // Расчёт по СУР — заново при каждой смене веса или снижения; устаревший ответ отбрасывается.
  useEffect(() => {
    if (sameAsSmeta) { requestRef.current++; setLoading(false); setByIndicator(result); return }
    const id = ++requestRef.current
    setLoading(true); setError('')
    previewRef.current({ customsValueMode: 'sir', sirWeight: weight, sirDiscountPercent: discount })
      .then((value) => { if (id === requestRef.current) setByIndicator(value) })
      .catch((reason) => { if (id === requestRef.current) setError(errorMessage(reason)) })
      .finally(() => { if (id === requestRef.current) setLoading(false) })
  }, [weight, discount, sameAsSmeta, result])

  const ready = invoice && byIndicator
  const rows = ready ? compareRows(invoice, byIndicator, weight) : []
  const lines = ready ? compareTotals(invoice, byIndicator) : []
  const counts = compareCounts(rows)
  const shownRows = order === 'diff' ? [...rows].sort((a, b) => (b.customsDiff ?? -Infinity) - (a.customsDiff ?? -Infinity)) : rows
  const sirTitle = `По СУР${discountLabel(discount)}`
  const weightLabel = weight === 'gross' ? 'брутто' : 'нетто'
  const hasDiscount = Number(discount) > 0
  const payments = lines.find((line) => line.label === 'Растаможка к оплате')
  const paymentsUsd = lines.find((line) => line.label === 'Растаможка к оплате, USD')
  const breakdown = lines.filter((line) => !line.total)
  const growth = payments?.invoice && payments.diff !== null ? payments.diff / payments.invoice * 100 : null

  function change(patch: SirParams) {
    if (patch.sirWeight) setWeight(patch.sirWeight)
    if (patch.sirDiscountPercent !== undefined) setDiscount(patch.sirDiscountPercent)
  }

  const subtitle = `Смета сейчас считается ${currentMode === 'sir' ? `по СУР (${currentWeight === 'gross' ? 'брутто' : 'нетто'}${discountLabel(currentDiscount)})` : 'по инвойсу'}. Вес и снижение здесь — только для сравнения.`
  return createPortal(<Modal title="Сравнение: по инвойсу и по СУР" subtitle={subtitle} onClose={onClose} width={1240} className="rs-step sc-modal">
    <div className="sc-options" role="group" aria-label="Условия СУР для сравнения">
      <span className="sir-field">По СУР:</span>
      <SirOptions weight={weight} discount={discount} disabled={false} onChange={change} />
      <span className="sc-country">страна {sir.sirCountry || 'CN'}</span>
      <span className="sc-status" role="status">{loading ? 'Считаем…' : ''}</span>
    </div>

    {error && <p className="sc-error" role="alert">{error}</p>}
    {!ready ? <p className="sc-wait">{error ? '' : 'Считаем оба варианта…'}</p> : <div className={loading ? 'sc-body sc-stale' : 'sc-body'}>
      <table className="sheet sc-totals">
        <colgroup><col className="sc-col-label" /><col /><col /><col /></colgroup>
        <thead><tr><th /><th scope="col">По инвойсу</th><th scope="col" className="sc-sir">{sirTitle}</th><th scope="col">Разница</th></tr></thead>
        <tbody>
          <tr className="sc-pay">
            <th scope="row">Растаможка к оплате</th>
            <td><b>{money(payments?.invoice ?? null)} KZT</b><small>{money(paymentsUsd?.invoice ?? null)} USD</small></td>
            <td className="sc-sir"><b>{money(payments?.sir ?? null)} KZT</b><small>{money(paymentsUsd?.sir ?? null)} USD</small></td>
            <td className={`sc-diff ${tone(payments?.diff ?? null)}`}><b>{signed(payments?.diff ?? null)} KZT</b><small>{signed(paymentsUsd?.diff ?? null)} USD{growth !== null && ` · ${growth > 0 ? '+' : '−'}${percent(Math.abs(growth))}%`}</small></td>
          </tr>
          {breakdown.map((line) => <tr key={line.label}>
            <th scope="row">{line.label}</th>
            <td>{money(line.invoice)}</td>
            <td className="sc-sir">{money(line.sir)}</td>
            <td className={`sc-diff ${tone(line.diff)}`}>{signed(line.diff)}</td>
          </tr>)}
        </tbody>
      </table>

      <div className="sc-rows-head">
        <p className="sc-note">
          По СУР — <b>{counts.sir}</b> из {counts.total} строк{counts.sir < counts.total && ', у остальных индикатора нет (по инвойсу)'}.
          {counts.below > 0 && <> Цена инвойса за 1 кг ниже СУР — <b className="sc-warn">{counts.below === counts.total ? 'у всех строк' : `у ${counts.below}`}</b>: таможня может откорректировать стоимость.</>}
        </p>
        <div className="dir-seg" role="radiogroup" aria-label="Порядок строк">
          <button type="button" role="radio" aria-checked={order === 'smeta'} className={order === 'smeta' ? 'active' : undefined} onClick={() => setOrder('smeta')}>Как в смете</button>
          <button type="button" role="radio" aria-checked={order === 'diff'} className={order === 'diff' ? 'active' : undefined} onClick={() => setOrder('diff')}>По разнице</button>
        </div>
      </div>

      <div className="sc-rows-wrap">
        <table className="sheet sc-rows">
          <thead>
            <tr className="sc-group">
              <th colSpan={3} />
              <th colSpan={hasDiscount ? 3 : 2} className="sc-span">Цена за 1 кг, USD</th>
              <th colSpan={3} className="sc-span">Таможенная стоимость, KZT</th>
            </tr>
            <tr>
              <th className="sc-num">№</th>
              <th className="sc-name">Товар</th>
              <th>Вес {weightLabel}, кг</th>
              <th className="sc-start">инвойс</th>
              <th className="sc-sir" title="Стоимостной индикатор риска из файла профиля">СУР</th>
              {hasDiscount && <th className="sc-sir">−{formatRuNumber(discount)}%</th>}
              <th className="sc-start">по инвойсу</th>
              <th className="sc-sir">по СУР</th>
              <th>разница</th>
            </tr>
          </thead>
          <tbody>{shownRows.map((row) => <CompareRow key={row.index} row={row} name={itemDisplayName(invoice.items[row.index], items[row.index])} code={invoice.items[row.index].code || items[row.index]?.code || ''} hasDiscount={hasDiscount} />)}</tbody>
          <tfoot><tr>
            <td className="sc-num" /><td className="sc-name">Итого</td>
            <td>{formatRuNumber(weightText(totalOf(rows.map((row) => row.weightKg)) ?? 0, true))}</td>
            <td className="sc-start" /><td className="sc-sir" />{hasDiscount && <td className="sc-sir" />}
            <td className="sc-start">{money(totalOf(rows.map((row) => row.customsInvoice)))}</td>
            <td className="sc-sir">{money(totalOf(rows.map((row) => row.customsSir)))}</td>
            <td className={`sc-diff ${tone(totalOf(rows.map((row) => row.customsDiff)))}`}>{signed(totalOf(rows.map((row) => row.customsDiff)))}</td>
          </tr></tfoot>
        </table>
      </div>
    </div>}
  </Modal>, document.body)
}

function CompareRow({ row, name, code, hasDiscount }: { row: SirCompareRow; name: string; code: string; hasDiscount: boolean }) {
  // Насколько цена инвойса ниже индикатора: «ниже на 71%».
  const below = row.belowSir && row.invoicePerKg !== null && row.sirPerKg ? Math.round((1 - row.invoicePerKg / row.sirPerKg) * 100) : null
  return <tr>
    <td className="sc-num">{row.index + 1}</td>
    <td className="sc-name"><span className="sc-name-text" title={name}>{name}</span>{code ? <CopyCode code={code} /> : <code>{dash}</code>}</td>
    <td>{row.weightKg === null ? dash : formatRuNumber(weightText(row.weightKg, true))}</td>
    <td className={below !== null ? 'sc-start sc-warn' : 'sc-start'} title={below !== null ? 'Цена инвойса за 1 кг ниже индикатора' : undefined}>
      {perKg(row.invoicePerKg)}{below !== null && <small>ниже на {below}%</small>}
    </td>
    <td className="sc-sir" title={row.profile ? `Профиль ${row.profile}` : 'Для кода индикатора нет — строка по инвойсу'}>{row.sirPerKg === null ? <span className="sc-none">нет</span> : perKg(row.sirPerKg)}</td>
    {hasDiscount && <td className="sc-sir"><b>{perKg(row.sirDiscountedPerKg)}</b></td>}
    <td className="sc-start">{money(row.customsInvoice)}</td>
    <td className="sc-sir"><b>{money(row.customsSir)}</b></td>
    <td className={`sc-diff ${tone(row.customsDiff)}`}>{signed(row.customsDiff)}</td>
  </tr>
}
