import { FormEvent, useEffect, useRef, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { ceramicsApi } from '../ceramicsApi'
import { CeramicSize, CeramicsType, Evidence, PriceResult, Resolution, SizeUnit } from '../types'
import { Draft, draftFromResolution, draftSize, MatchGroup } from '../lib/matchGroups'
import { baseName, formatSize, normalizeNumber, sizeKey, sizeUnits } from '../lib/sizes'
import { TypeCombo } from './TypeCombo'

type Props = {
  group: MatchGroup
  resolution: Resolution
  types: CeramicsType[]
  onDraft: (change: Partial<Draft>) => void
  onDraftPrice: (price: PriceResult | undefined) => void
  // Строка проверена — отметить и перейти к следующей.
  onDone: () => void
}

const fieldTitles: Record<Evidence['field'], string> = { type: 'Тип товара', size: 'Размер', price: 'Цена' }
const sourceLabels: Record<string, string> = {
  mark_exact: 'Марка', chinese_name: 'Китайское название', chinese_number: 'Число в названии', chinese_name_match: 'Совпадение с базой', mark_pattern: 'Серия марки',
  russian_name: 'Русское название', ai: 'ИИ', price_mode: 'База цен', manual: 'Оператор',
}
const typeStatusText: Record<string, string> = {
  NEED_REVIEW: 'Не удалось уверенно определить товар. Выберите один из возможных вариантов:',
  NOT_FOUND: 'Подходящий тип среди типов базы не найден. Выберите тип вручную:',
}

// Причины решения и правка типа, размера и цены. В базу сохраняется только по кнопке.
export function ResultDetails({ group, resolution, types, onDraft, onDraftPrice, onDone }: Props) {
  const { draft } = group
  const found = resolution.status === 'FOUND_BY_MARK'
  const [priceTouched, setPriceTouched] = useState(found)
  const [nameTouched, setNameTouched] = useState(found)
  const [loadingPrice, setLoadingPrice] = useState(false)
  const [error, setError] = useState('')
  const priceInfo = group.draftPrice ?? resolution.price
  const lastLookup = useRef(`${draft.productType}|${draft.sizeValue}|${draft.sizeUnit}`)

  // Тип или размер изменён оператором — цена подбирается заново по базе (мода).
  useEffect(() => {
    const lookup = `${draft.productType}|${draft.sizeValue}|${draft.sizeUnit}`
    if (lookup === lastLookup.current) return
    lastLookup.current = lookup
    if (!draft.productType) { onDraftPrice(undefined); return }
    const sizeValue = normalizeNumber(draft.sizeValue)
    if (draft.sizeValue && (!sizeValue || !draft.sizeUnit)) return
    let cancelled = false
    const timer = window.setTimeout(() => {
      setLoadingPrice(true)
      ceramicsApi.price(draft.productType, sizeValue, sizeValue ? draft.sizeUnit : '').then((price) => {
        if (cancelled) return
        onDraftPrice(price)
        const change: Partial<Draft> = {}
        if (!priceTouched) change.price = price.status === 'MATCHED' ? price.price ?? '' : ''
        if (!nameTouched) change.name = baseName(price.name, group.chineseName)
        onDraft(change)
      }).catch((reason) => { if (!cancelled) setError(errorMessage(reason)) })
        .finally(() => { if (!cancelled) setLoadingPrice(false) })
    }, 300)
    return () => { cancelled = true; window.clearTimeout(timer) }
    // onDraft/onDraftPrice меняются на каждом рендере таблицы; запрос зависит только от типа и размера.
  }, [draft.productType, draft.sizeValue, draft.sizeUnit])

  const size = draftSize(draft)
  const sizeValid = !draft.sizeValue || (Boolean(normalizeNumber(draft.sizeValue)) && Number(normalizeNumber(draft.sizeValue)) > 0 && Boolean(draft.sizeUnit))
  const priceValid = Boolean(normalizeNumber(draft.price)) && Number(normalizeNumber(draft.price)) > 0
  const typeNames = types.map((item) => item.name)
  if (draft.productType && !typeNames.includes(draft.productType)) typeNames.unshift(draft.productType)

  // Ничего не сохраняет: данные строки уходят в CSV, в базу их потом загружают сами.
  function submit(event: FormEvent) {
    event.preventDefault()
    onDone()
  }

  // Варианты размера без повторов: один размер из нескольких источников — один вариант.
  const sizeChoices = resolution.sizeOptions.reduce<{ size: CeramicSize; sources: string[] }[]>((list, option) => {
    const same = list.find((item) => sizeKey(item.size) === sizeKey(option.size))
    if (same) same.sources.push(sourceLabels[option.source] ?? option.source)
    else list.push({ size: option.size, sources: [sourceLabels[option.source] ?? option.source] })
    return list
  }, [])
  // Слова китайского названия — одной строкой разбора, а не отдельным пунктом на каждое.
  const isTermEvidence = (item: Evidence) => item.field === 'type' && item.source === 'chinese_name' && item.text.startsWith('Китайское название содержит')
  const evidenceByField = (['type', 'size', 'price'] as const).map((field) => ({ field, items: resolution.evidence.filter((item) => item.field === field && !isTermEvidence(item)) }))

  // Чего не хватает в строке — подсказка рядом с «Готово»; отметить строку можно и так.
  const missing = !draft.name.trim() ? 'нет наименования' : !priceValid ? 'нет цены' : ''

  return <div className="cer-details">
    <form className="cer-editor" onSubmit={submit}>
      {!found && typeStatusText[resolution.typeStatus] && <p className="cer-review-text">{typeStatusText[resolution.typeStatus]}</p>}
      {!found && resolution.typeCandidates.length > 0 && resolution.typeStatus !== 'MATCHED' && <div className="cer-chips">
        {resolution.typeCandidates.map((candidate) => <button key={candidate.productType} type="button" title={candidate.reason}
          className={draft.productType === candidate.productType ? 'cer-chip active' : 'cer-chip'} onClick={() => onDraft({ productType: candidate.productType })}>
          {candidate.productType}{candidate.confidence > 0 && <small>{Math.round(candidate.confidence * 100)}%</small>}
        </button>)}
      </div>}
      <div className="cer-edit-row">
        <div className="cer-field" title="Тип и размер нужны для подбора цены; в базу сохраняются наименование, марка и цена">Тип товара
          <TypeCombo label="Тип товара" value={draft.productType} names={typeNames} onChange={(productType) => onDraft({ productType })}
            hints={new Map(resolution.typeCandidates.filter((candidate) => candidate.confidence > 0).map((candidate) => [candidate.productType, candidate.confidence]))} />
        </div>
        <label className="cer-field">Размер
          <input value={draft.sizeValue} inputMode="decimal" placeholder="нет" onChange={(event) => onDraft({ sizeValue: event.target.value, sizeUnit: draft.sizeUnit || (event.target.value ? 'inch' : '') })} />
        </label>
        <label className="cer-field">Ед.
          <select value={draft.sizeValue ? draft.sizeUnit : ''} disabled={!draft.sizeValue} onChange={(event) => onDraft({ sizeUnit: event.target.value as SizeUnit })}>
            {!draft.sizeValue && <option value="">—</option>}
            {sizeUnits.map((unit) => <option key={unit.value} value={unit.value}>{unit.label}</option>)}
          </select>
        </label>
        <label className="cer-field">Цена
          <input value={draft.price} inputMode="decimal" placeholder={loadingPrice ? 'подбираем…' : 'введите'} className={priceValid ? undefined : 'cer-need'}
            onChange={(event) => { setPriceTouched(true); onDraft({ price: event.target.value }) }} />
        </label>
      </div>
      {!found && sizeChoices.length > 1 && <div className="cer-chips">
        {sizeChoices.map((choice) => <button key={sizeKey(choice.size)} type="button"
          className={sizeKey(size) === sizeKey(choice.size) ? 'cer-chip active' : 'cer-chip'} onClick={() => onDraft({ sizeValue: choice.size.value, sizeUnit: choice.size.unit })}>
          {formatSize(choice.size)}<small>{choice.sources.join(', ')}</small>
        </button>)}
      </div>}
      {!sizeValid && <p className="error">Размер — положительное число; единицу выберите из списка.</p>}

      {priceInfo.message && <p className={`cer-price-note ${priceInfo.status}`}>{loadingPrice ? 'Подбираем цену…' : priceInfo.message}</p>}
      {priceInfo.options.length > 1 && <div className="cer-chips">
        {priceInfo.options.map((option) => <button key={option.price} type="button" className={normalizeNumber(draft.price) === option.price ? 'cer-chip active' : 'cer-chip'}
          onClick={() => { setPriceTouched(true); onDraft({ price: option.price }) }}>
          {option.price}<small>{option.count} раз</small>
        </button>)}
      </div>}

      <label className="cer-field">Наименование
        <input value={draft.name} placeholder="Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘" onChange={(event) => { setNameTouched(true); onDraft({ name: event.target.value }) }} />
      </label>
      {error && <p className="error" role="alert">{error}</p>}
      <div className="cer-editor-actions">
        <details className="cer-why">
          <summary>Откуда результат</summary>
          {resolution.chineseTerms.length > 0 && <p className="cer-terms">
            {resolution.chineseTerms.map((term) => <span key={term.text}><b>{term.text}</b> {term.meaning}</span>)}
          </p>}
          {evidenceByField.map(({ field, items }) => items.length > 0 && <div key={field} className="cer-why-group">
            <h4>{fieldTitles[field]}</h4>
            <ul>{items.map((item, index) => <li key={index}><span className={`cer-source-tag ${item.source}`}>{sourceLabels[item.source] ?? item.source}</span>{item.text}</li>)}</ul>
          </div>)}
          {resolution.warnings.length > 0 && <ul className="cer-warnings">{resolution.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>}
          {!resolution.evidence.length && !resolution.warnings.length && <p className="muted">Нет данных.</p>}
        </details>
        {missing && <span className="cer-missing">В строке {missing}</span>}
        {!found && <button type="button" className="cer-secondary" onClick={() => { setPriceTouched(false); setNameTouched(false); onDraftPrice(undefined); onDraft(draftFromResolution(resolution, group.chineseName)) }}>Сбросить</button>}
        <button type="submit" title="Отметить строку проверенной и открыть следующую">Готово <span aria-hidden="true">→</span></button>
      </div>
    </form>
  </div>
}
