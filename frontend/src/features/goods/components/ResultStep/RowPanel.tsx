import { ReactNode } from 'react'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { formatMoney } from '@/shared/lib/formatDisplayNumber'
import { nameParts } from '@/shared/tnved/codes'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { AntiDumpingChooser, AntiDumpingInfo } from './AntiDumpingChooser'
import { RowProblem } from './errorGroups'
import { displayNumber, dutyRateText, fixedNumber, itemNames, rowPaymentKzt, sirTitle, vatRateText } from './resultValues'

// Вес — один знак после запятой: «20 230,0 кг».
const weight = (value?: string) => { const shown = fixedNumber(value, 1); return shown === '—' ? shown : `${shown} кг` }

type Props = {
  item: GoodsCalculationResult['items'][number]
  source?: GoodsItemInput
  index: number
  // Проблемы строки без дублей — каждая показывается у своего поля.
  problems: RowProblem[]
  antiDumpingInfo?: AntiDumpingInfo
  // «Строка 3 из 30»: место в показанном списке.
  position: number
  count: number
  onPrev?: () => void
  onNext?: () => void
  onClose: () => void
  // Поле цены — то же, что в таблице (общая логика правки).
  priceField: ReactNode
  aiNote?: string
  edited: boolean
  sirMode: boolean
  loading: boolean
  onChangeCode: () => void
  // Открыть чат подбора кода ИИ для строки (нет — кнопки нет).
  onAgent?: () => void
  onToggleIncluded: (included: boolean) => void
  onAntiDumpingChoice: (key: string) => void
}

const arrow = (d: string) => <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><path d={d} /></svg>

// Панель выбранной строки справа (на узком экране — выезжающая): код и ставки, цена с пояснением ИИ,
// количество и вес, расчёт по строке, включение строки в расчёт.
export function RowPanel({ item, source, index, problems, antiDumpingInfo, position, count, onPrev, onNext, onClose, priceField, aiNote, edited, sirMode, loading, onChangeCode, onAgent, onToggleIncluded, onAntiDumpingChoice }: Props) {
  const names = itemNames(item, source)
  const code = item.code || source?.code || ''
  const snapshot = item.directorySnapshot
  // Описание позиции из номенклатуры: сервер кладёт его в name снимка, если товар из справочника не выбран
  // (иначе там название товара). tariffText — текст ставки («5%»), не описание.
  const productChosen = Boolean(source?.productDirectoryId) || snapshot?.productDirectoryId != null
  const nomenclatureName = !productChosen && typeof snapshot?.name === 'string' ? nameParts(snapshot.name).join(' ') : ''
  const description = nomenclatureName && nomenclatureName !== names.main && nomenclatureName !== names.sub ? nomenclatureName : ''
  const duty = dutyRateText(snapshot).replace(/^ЕТТ /, '')
  const vat = vatRateText(snapshot)
  const codeProblem = problems.find((problem) => problem.kind === 'code')
  const priceProblem = problems.some((problem) => problem.kind === 'price')
  const otherProblems = problems.filter((problem) => problem.kind === 'other')
  const currency = item.goodsCostCurrency || item.costCurrency || 'USD'
  const positive = (value?: string) => Number(String(value ?? '').replace(/\s/g, '')) > 0
  // Строка с ошибкой не рассчитана: вместо нулей — «—»; сбор распределяется и без расчёта строки.
  const known = (value: string) => item.errors?.length ? '' : value
  const lines: { label: string; value: string; title?: string }[] = [
    { label: 'Стоимость', value: formatMoney(priceProblem ? '' : item.goodsCost || item.costInput, currency) },
    ...(positive(item.deliveryUsd) ? [{ label: 'Доставка', value: formatMoney(item.deliveryUsd, 'USD') }] : []),
    { label: 'Таможенная стоимость', value: formatMoney(known(item.customsValueKzt), 'KZT'), title: item.sir ? sirTitle(item.sir) : undefined },
    { label: `Пошлина${duty ? ` ${duty}` : ''}`, value: formatMoney(known(item.dutyKzt), 'KZT') },
    ...(positive(item.antiDumpingKzt) ? [{ label: 'Антидемпинг', value: formatMoney(item.antiDumpingKzt, 'KZT') }] : []),
    { label: `НДС${vat ? ` ${vat}` : ''}`, value: formatMoney(known(item.vatKzt), 'KZT') },
    { label: 'Сбор', value: formatMoney(item.feeKzt, 'KZT'), title: 'Таможенный сбор, распределённый на строку' },
  ]

  return <aside className="rp" aria-label={`Строка ${index + 1}`}>
    <div className="rp-head">
      <span>Строка {position} из {count}</span>
      <button type="button" className="icon-button ghost" onClick={onPrev} disabled={!onPrev} title="Предыдущая строка (↑)" aria-label="Предыдущая строка">{arrow('M4 10l4-4 4 4')}</button>
      <button type="button" className="icon-button ghost" onClick={onNext} disabled={!onNext} title="Следующая строка (↓)" aria-label="Следующая строка">{arrow('M4 6l4 4 4-4')}</button>
      <button type="button" className="icon-button ghost" onClick={onClose} title="Закрыть панель" aria-label="Закрыть панель строки">{arrow('M4.5 4.5l7 7M11.5 4.5l-7 7')}</button>
    </div>

    <div className="rp-title">
      <h2>{names.main}</h2>
      {names.sub && <p>{names.sub}</p>}
    </div>

    <section className="rp-block">
      <div className="rp-block-head">
        <h3>Код ТН ВЭД</h3>
        {code && <span className="rp-head-actions">
          {onAgent && <button type="button" className="button-link" onClick={onAgent} disabled={loading}>Чат ИИ</button>}
          <button type="button" className="button-link" onClick={onChangeCode} disabled={loading}>Сменить</button>
        </span>}
      </div>
      {code ? <CopyCode code={code} className="rp-code" /> : <p className="rp-field-error">Код не указан</p>}
      {code && codeProblem && <p className="rp-field-error">{codeProblem.text}</p>}
      {!code && <div className="rp-set">
        <button type="button" className="secondary" onClick={onChangeCode} disabled={loading}>Указать код</button>
        {onAgent && <button type="button" className="secondary" onClick={onAgent} disabled={loading} title="Подобрать код ТН ВЭД с ИИ: чат по строке с фото и наименованиями">Подобрать ИИ</button>}
      </div>}
      {description && <p className="rp-muted">{description}</p>}
      {(duty || vat) && <p className="rp-chips">{duty && <span>Пошлина {duty}</span>}{vat && <span>НДС {vat}</span>}</p>}
      {antiDumpingInfo && <div className="rp-dump">
        <span>Антидемпинг</span>
        <AntiDumpingChooser info={antiDumpingInfo} choice={source?.antiDumpingChoice} disabled={loading || item.excluded} onChoose={onAntiDumpingChoice} />
      </div>}
    </section>

    <section className="rp-block">
      <div className="rp-block-head"><h3>Цена за шт.</h3>{aiNote && <span className="badge warn">оценка ИИ</span>}</div>
      <div className={priceProblem ? 'rp-price todo' : 'rp-price'}>{priceField}</div>
      {priceProblem && <p className="rp-field-todo">Укажите цену</p>}
      {aiNote && <p className="rp-muted">{aiNote}</p>}
      {edited && <p className="rp-muted">В Excel: {formatMoney(source?.sourcePricePerPair, 'USD')}</p>}
      {sirMode && <p className="rp-muted">{item.sir ? `ТС по СУР: ${fixedNumber(item.sir.pricePerKgUsd, 2)} USD/кг × ${fixedNumber(item.sir.weightKg, 1)} кг. Цена инвойса на ТС не влияет` : 'Для кода нет индикатора СУР — строка считается по инвойсу'}</p>}
    </section>

    <div className="rp-block">
      <dl className="rp-grid">
        <div><dt>Мест</dt><dd>{displayNumber(item.places)}</dd></div>
        <div><dt>Количество</dt><dd>{displayNumber(item.pairs)} шт{item.pairsPerPlace && <small> · {displayNumber(item.pairsPerPlace)} в месте</small>}</dd></div>
        <div><dt>Брутто</dt><dd>{weight(item.grossKg)}</dd></div>
        <div><dt>Нетто</dt><dd>{weight(item.netKg)}</dd></div>
      </dl>
      {/* Прочие замечания (количество, вес, доставка) — рядом с количеством и весом. */}
      {otherProblems.map((problem) => <p key={problem.text} className={problem.tone === 'error' ? 'rp-field-error' : 'rp-field-todo'}>{problem.text}</p>)}
    </div>

    <section className="rp-block rp-calc">
      <h3>Расчёт по строке</h3>
      <dl>
        {lines.map((line) => <div key={line.label} title={line.title}><dt>{line.label}</dt><dd>{line.value}</dd></div>)}
      </dl>
    </section>

    {/* Итог и включение строки — внизу панели, видны всегда. */}
    <div className="rp-foot">
      <dl className="rp-total"><dt>К оплате</dt><dd>{formatMoney(rowPaymentKzt(item), 'KZT')}</dd></dl>
      <label className="rp-include">
        <input type="checkbox" checked={!item.excluded} disabled={loading} onChange={(event) => onToggleIncluded(event.target.checked)} />
        Учитывать строку в расчёте
      </label>
    </div>
  </aside>
}
