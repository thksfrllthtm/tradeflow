import { Dispatch, RefObject, SetStateAction } from 'react'
import { GlovesCalculationResult, GlovesItemInput, GlovesParams } from '../../../shared/types'
import { ReviewRow } from '../ReviewRow/ReviewRow'
import '../../../shared/styles/calculatorCommon.css'
import '../../styles/calculator.css'
import './ReviewStep.css'

type Props = {
  params: GlovesParams
  items: GlovesItemInput[]
  result: GlovesCalculationResult | null
  selectedForDirectory: string[]
  paramsOpen: boolean
  loading: boolean
  storageNotice: string
  deliverySummary: string
  feeSummary: string
  tableWrapRef: RefObject<HTMLDivElement | null>
  topScrollRef: RefObject<HTMLDivElement | null>
  includedCount: number
  allDirectorySelected: boolean
  setItems: Dispatch<SetStateAction<GlovesItemInput[]>>
  setSelectedForDirectory: Dispatch<SetStateAction<string[]>>
  setParamsOpen: (value: boolean) => void
  setParam: (field: keyof GlovesParams, value: string) => void
  syncHorizontalScroll: (source: 'top' | 'body') => void
  openDirectoryPicker: (ids: string[]) => void
  clearRowDirectory: (id: string) => void
  setItem: (id: string, field: keyof GlovesItemInput, value: string | boolean | undefined) => void
  removeRow: (id: string) => void
  setLightbox: (value: string) => void
  addManualPhoto: (id: string, event: React.ChangeEvent<HTMLInputElement>) => void
  resetAutomaticNet: (id: string) => void
  calculate: () => void
  saveDraft: () => void
  clearAll: () => void
  onBackToImport: () => void
}

export function ReviewStep({ params, items, result, selectedForDirectory, paramsOpen, loading, storageNotice, deliverySummary, feeSummary, tableWrapRef, topScrollRef, includedCount, allDirectorySelected, setItems, setSelectedForDirectory, setParamsOpen, setParam, syncHorizontalScroll, openDirectoryPicker, clearRowDirectory, setItem, removeRow, setLightbox, addManualPhoto, resetAutomaticNet, calculate, saveDraft, clearAll, onBackToImport }: Props) {
  return <div className="gloves-step-page">
    <div className="gloves-step-title"><div><p className="eyebrow">ЭТАП 2</p><h2>Проверка строк и подбор кода</h2><p className="muted">Все данные строки видны прямо в таблице. Измените значения, выберите карточку ТН ВЭД и проверьте статус перед расчётом.</p></div><button type="button" className="button-link" onClick={onBackToImport}>← К файлу и колонкам</button></div>
    <div className="gloves-row-toolbar"><span>{selectedForDirectory.length > 0 ? `Выбрано строк: ${selectedForDirectory.length}` : 'Отметьте строки, чтобы применить одну карточку сразу к нескольким'}</span><button type="button" className="secondary" disabled={selectedForDirectory.length === 0} onClick={() => openDirectoryPicker(selectedForDirectory)}>Подобрать код для выбранных</button><button type="button" className="button-link" disabled={selectedForDirectory.length === 0} onClick={() => setSelectedForDirectory([])}>Снять выбор</button></div>
    <div className="gloves-table-top-scroll" ref={topScrollRef} onScroll={() => syncHorizontalScroll('top')} aria-hidden="true"><div /></div>
    <section className="gloves-table-wrap" ref={tableWrapRef} onScroll={() => syncHorizontalScroll('body')}><table className="gloves-table gloves-review-table"><thead><tr><th className="col-check"><input type="checkbox" checked={includedCount > 0 && includedCount === items.length} onChange={(event) => setItems((current) => current.map((item) => ({ ...item, excluded: !event.target.checked })))} aria-label="Включить все строки" title="Включить или исключить все строки" /><small>Вкл.</small></th><th className="col-check"><input type="checkbox" checked={allDirectorySelected} onChange={(event) => setSelectedForDirectory(event.target.checked ? items.map((item) => item.id ?? '') : [])} aria-label="Выбрать все строки для карточки" title="Выбрать все строки для карточки" /><small>Выбрать</small></th><th>Строка</th><th>Товар, код и фото</th><th>Места</th><th>Пар в месте</th><th>Пары</th><th>Брутто, кг</th><th>Нетто, кг</th><th>ИТС</th><th>Статус</th><th></th></tr></thead><tbody>{items.map((item, index) => <ReviewRow key={item.id ?? index} item={item} rowNumber={index + 1} resultItem={result?.items[index]} selected={selectedForDirectory.includes(item.id ?? '')} onSelected={(checked) => setSelectedForDirectory((current) => checked ? [...current, item.id ?? ''] : current.filter((id) => id !== item.id))} onOpenPicker={() => openDirectoryPicker([item.id ?? ''])} onClearDirectory={() => clearRowDirectory(item.id ?? '')} onChange={setItem} onRemove={removeRow} onPreview={setLightbox} onUpload={addManualPhoto} onResetNet={() => resetAutomaticNet(item.id ?? '')} />)}{items.length === 0 && <tr><td colSpan={12} className="gloves-empty"><h3>Строк пока нет</h3><p>Загрузите XLSX или добавьте строку вручную.</p></td></tr>}</tbody></table></section>
    <details className="gloves-params" open={paramsOpen} onToggle={(event) => setParamsOpen(event.currentTarget.open)}>
      <summary>Параметры расчёта <span className="gloves-params-summary">курс {params.usdKzt || '—'} · доставка: {deliverySummary} · сбор: {feeSummary}</span></summary>
      <div className="gloves-params-grid">
        <label>Дата расчёта<input type="date" value={params.assessmentDate} onChange={(event) => setParam('assessmentDate', event.target.value)} required /></label>
        <label>Страна происхождения<input value={params.origin} onChange={(event) => setParam('origin', event.target.value)} placeholder="Например, Китай" /></label>
        <label>Курс USD → KZT<input inputMode="decimal" value={params.usdKzt} onChange={(event) => setParam('usdKzt', event.target.value.replace(',', '.'))} placeholder="Обязательно" required /></label>
        <label>Режим доставки<select value={params.deliveryMode} onChange={(event) => setParam('deliveryMode', event.target.value)}><option value="wagons">По вагонам</option><option value="total">Общая сумма</option><option value="none">Без доставки</option></select></label>
        {params.deliveryMode === 'none' ? null : params.deliveryMode === 'wagons' ? <><label>Вагоны<input inputMode="decimal" value={params.wagons} onChange={(event) => setParam('wagons', event.target.value.replace(',', '.'))} placeholder="Например, 2" /></label><label>Тариф за вагон, USD<input inputMode="decimal" value={params.wagonTariffUsd} onChange={(event) => setParam('wagonTariffUsd', event.target.value.replace(',', '.'))} /></label></> : <label>Общая доставка, USD<input inputMode="decimal" value={params.deliveryTotalUsd} onChange={(event) => setParam('deliveryTotalUsd', event.target.value.replace(',', '.'))} placeholder="Например, 840" /><small>Сумма за всю партию.</small></label>}
        <label>Вес упаковки, кг<input inputMode="decimal" value={params.packageWeightKg} onChange={(event) => setParam('packageWeightKg', event.target.value.replace(',', '.'))} /><small>Для автоматического нетто.</small></label>
        <label>МРП, KZT<input inputMode="decimal" value={params.mrp} onChange={(event) => setParam('mrp', event.target.value.replace(',', '.'))} /></label>
        <label>Множитель сбора<input inputMode="decimal" value={params.feeMultiplier} onChange={(event) => setParam('feeMultiplier', event.target.value.replace(',', '.'))} placeholder="Например, 6" /><small>Сбор = МРП × множитель. Делится поровну.</small></label>
      </div>
    </details>
    <div className="gloves-actions"><button type="button" className="btn-calculate" onClick={() => void calculate()} disabled={loading || includedCount === 0}>{loading ? 'Считаем…' : 'Рассчитать и открыть результат →'}</button><button type="button" className="btn-save-draft" onClick={() => void saveDraft()} disabled={loading || items.length === 0}>Сохранить черновик</button><button type="button" className="btn-clear" onClick={clearAll} disabled={loading || items.length === 0}>Очистить</button><span className="muted">Изменения сохраняются в браузере; для серверного черновика нажмите «Сохранить черновик».</span>{storageNotice && <span className="muted">{storageNotice}</span>}</div>
  </div>
}
