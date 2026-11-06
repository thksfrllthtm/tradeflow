import { ReactNode } from 'react'
import { formatMoney } from '@/shared/lib/formatDisplayNumber'
import { formatRuDate } from '@/shared/lib/dates'
import { moneyText } from '@/shared/lib/rounding'
import { GoodsCalculationResult } from '@/shared/types/goods'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { calculatedRows, customsPaymentsUsd, displayNumber, hasAntiDumping, parseCalculatorNumber } from './resultValues'
import './ResultSummary.css'

type SummaryProps = {
  result: GoodsCalculationResult
  calculating: boolean
  delivery: boolean
  // По СУР: строки без индикатора (по инвойсу) и итог того же расчёта по инвойсу — для «Разницы с инвойсом»;
  // null — режим «По инвойсу».
  sirInvoiceRows: number | null
  invoiceTotal: string | null
  onCompare: () => void
  // «Таможенная стоимость [По инвойсу | По СУР]», условия СУР или, по инвойсу, касса и подбор цен.
  footer?: ReactNode
  // Справа: сохранение, экспорт и меню «⋯».
  tools: ReactNode
}

// Полоса итогов: «К оплате» (таможенная стоимость, доставка и курс — в подсказке при наведении), слагаемые,
// «Таможенная стоимость» с кассой (footer) и инструменты. Широкая полоса — одна строка, уже — две: итоги и
// инструменты, под ними footer (ResultSummary.css). Пока идёт пересчёт, итог приглушён и помечен «пересчёт…».
// Не посчитано — «—», а не ноль: ни одна строка не рассчитана — пошлина, НДС и итог не известны.
export function ResultSummary({ result, calculating, delivery, sirInvoiceRows, invoiceTotal, onCompare, tools, footer }: SummaryProps) {
  const calculated = calculatedRows(result)
  const none = calculated === 0
  const notCalculated = none || !String(result.totals.customsPaymentsKzt ?? '').trim()
  const missing = notCalculated ? 0 : result.totals.totalCount - calculated
  // Разница с расчётом по инвойсу — в тиынах, без ошибок двоичной записи.
  const sirTotal = parseCalculatorNumber(result.totals.customsPaymentsKzt)
  const invoice = invoiceTotal ? parseCalculatorNumber(invoiceTotal) : null
  const diff = sirInvoiceRows !== null && !notCalculated && sirTotal !== null && invoice !== null ? Math.round(sirTotal * 100) - Math.round(invoice * 100) : null
  const usd = notCalculated ? null : customsPaymentsUsd(result)
  const known = (value: string) => none ? '' : value
  // Слагаемые растаможки — как на бэкенде: пошлина + антидемпинг + НДС + сбор. Сбор считается и без строк.
  // Ни одна строка не рассчитана — пошлины, антидемпинга и НДС нет: прочерки не показываем, только сбор.
  const parts = [
    ...(none ? [] : [{ label: 'Пошлина', value: result.totals.dutyKzt }]),
    ...(!none && hasAntiDumping(result) ? [{ label: 'Антидемпинг', value: result.totals.antiDumpingKzt }] : []),
    ...(none ? [] : [{ label: 'НДС', value: result.totals.vatKzt }]),
    { label: 'Сбор', value: result.totals.feeKzt, title: 'Таможенный сбор' },
  ]
  const total = displayNumber(result.totals.customsPaymentsKzt)

  return (
    <section className={calculating ? 'rs-summary calculating' : 'rs-summary'} aria-label="Итоги расчёта" aria-busy={calculating}>
      <div className={notCalculated ? 'rs-sum-in' : 'rs-sum-in done'}>
        <div className="rs-sum-total">
          {/* «≈ USD», «без N строк» и «пересчёт…» — мелко в строке подписи: итог в две строки, как слагаемые. */}
          <span className="rs-sum-label">К оплате{!notCalculated && <span className="rs-sum-sub">
            {` · ≈ ${usd === null ? '—' : formatMoney(moneyText(usd), 'USD')}`}
            {missing > 0 && ` · без ${missing} из ${result.totals.totalCount} строк`}
            {calculating && <em> · пересчёт…</em>}
          </span>}</span>
          <span className="rs-sum-value">
            {/* Сумма по нажатию копируется без пробелов — для вставки в Excel. «—» и «расчёт не завершён» — одной строкой. */}
            <strong aria-live="polite" className={notCalculated ? 'empty' : undefined}>{notCalculated ? '—' : <><CopyCode code={total} number />{' '}KZT</>}</strong>
            {notCalculated && <span className="rs-sum-sub">расчёт не завершён{calculating && <em> · пересчёт…</em>}</span>}
          </span>
          <dl className="rs-sum-tip" role="tooltip">
            <div><dt>Таможенная стоимость</dt><dd>{formatMoney(known(result.totals.customsValueKzt), 'KZT')}</dd></div>
            {delivery && <div><dt>Доставка</dt><dd>{formatMoney(result.totals.totalDeliveryUsd, 'USD')} <small>· не входит в растаможку</small></dd></div>}
            {result.usdKzt && <div><dt>Курс USD</dt><dd>{formatMoney(result.usdKzt, 'KZT')}{result.assessmentDate && <small> на <time dateTime={result.assessmentDate.slice(0, 10)}>{formatRuDate(result.assessmentDate)}</time></small>}</dd></div>}
            {/* Курс EUR — когда он нужен смете (пошлина или стоимость в евро). */}
            {result.eurKzt && <div><dt>Курс EUR</dt><dd>{formatMoney(result.eurKzt, 'KZT')}</dd></div>}
          </dl>
        </div>
        <dl className="rs-sum-parts">
          {parts.map((part) => <div className="rs-part" key={part.label} title={part.title}>
            <dt>{part.label}</dt>
            <dd>{formatMoney(part.value, 'KZT')}</dd>
          </div>)}
          {/* По СУР — насколько больше или меньше, чем по инвойсу; по нажатию — сравнение. */}
          {diff !== null && <div className="rs-part">
            <dt>Разница с инвойсом</dt>
            <dd><button type="button" className="button-link rs-sum-diff" onClick={onCompare} title="Открыть сравнение: по инвойсу и по СУР">{diff > 0 ? '+' : diff < 0 ? '−' : ''}{formatMoney(moneyText(Math.abs(diff) / 100), 'KZT')}</button></dd>
          </div>}
        </dl>
        {footer}
        <div className="rs-sum-tools">{tools}</div>
      </div>
    </section>
  )
}
