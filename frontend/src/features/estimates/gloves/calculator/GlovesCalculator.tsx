import { ChangeEvent, useEffect, useRef, useState } from 'react'
import { tnvedDirectoryApi } from '@/features/directories/tnved/tnvedDirectoryApi'
import { TNVEDDirectoryEntry } from '@/features/directories/tnved/types'
import { errorMessage } from '@/shared/lib/errorMessage'
import { calculatorApi } from '../../shared/api/calculatorApi'
import { useSyncedHorizontalScroll } from '../../shared/hooks/useSyncedHorizontalScroll'
import { mappedFieldForHeader, normalizeStoredMapping } from '../../shared/lib/excelColumns'
import { mergeStoredPhotos, photosOf } from '../../shared/lib/itemPhotos'
import { MAX_LOCAL_STORAGE_CHARS, storageKey } from '../../shared/lib/localDraft'
import { GlovesCalculationInput, GlovesCalculationResult, GlovesEstimate, GlovesItemInput, GlovesParams, GlovesParseRow, GlovesSourceSnapshot, SheetInfo } from '../../shared/types'
import { glovesEstimatesApi } from '../glovesEstimatesApi'
import { GlovesMapping } from '../types'
import { emptyParams, ensureItemIds, FILE_KEY, glovesPhotoStore, ITEMS_KEY, LOCAL_UPDATED_KEY, normalizeParams, PARAMS_KEY, storedItems, storedParams } from '../lib/draftStorage'
import { buildGloveItems, emptyMapping, mappingFieldOptions } from '../lib/importMapping'
import '../../shared/styles/calculatorCommon.css'
import '../styles/calculator.css'
import './GlovesCalculator.css'
import { ImportStep } from '../components/ImportStep/ImportStep'
import { ReviewStep } from '../components/ReviewStep/ReviewStep'
import { ResultStep } from '../components/ResultStep/ResultStep'
import { TnvedPickerModal } from '../components/TnvedPickerModal/TnvedPickerModal'
import { PhotoPreview } from '../components/PhotoPreview/PhotoPreview'
import { columnMappingActions } from '../../shared/lib/columnMappingActions'
import { resultPriceActions } from '../../shared/lib/resultPriceActions'
import { rowActions } from '../../shared/lib/rowActions'
import '../../shared/styles/calculatorPage.css'

type Props = { estimateId: number; onEstimateSaved?: (estimate: GlovesEstimate) => void }

export function GlovesCalculator({ estimateId, onEstimateSaved }: Props) {
  const paramsStorageKey = storageKey(PARAMS_KEY, estimateId)
  const itemsStorageKey = storageKey(ITEMS_KEY, estimateId)
  const fileStorageKey = storageKey(FILE_KEY, estimateId)
  const localUpdatedStorageKey = storageKey(LOCAL_UPDATED_KEY, estimateId)
  const [params, setParams] = useState<GlovesParams>(() => storedParams(estimateId))
  const [items, setItems] = useState<GlovesItemInput[]>(() => storedItems(estimateId))
  const [step, setStep] = useState<1 | 2 | 3>(() => storedItems(estimateId).length > 0 ? 2 : 1)
  const [result, setResult] = useState<GlovesCalculationResult | null>(null)
  const [previousBulkResult, setPreviousBulkResult] = useState<GlovesCalculationResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [storageNotice, setStorageNotice] = useState('')
  const [photoStorageReady, setPhotoStorageReady] = useState(false)
  const [sheets, setSheets] = useState<SheetInfo[]>([])
  const [sheet, setSheet] = useState('')
  const [headerRow, setHeaderRow] = useState(1)
  const [dataStartRow, setDataStartRow] = useState(2)
  const [headers, setHeaders] = useState<string[]>([])
  const [rawRows, setRawRows] = useState<GlovesParseRow[]>([])
  const [parsedImages, setParsedImages] = useState<{ sourceRow: number; data: string; mimeType: string }[]>([])
  const [mapping, setMapping] = useState<GlovesMapping>(emptyMapping)
  const [sourceSnapshot, setSourceSnapshot] = useState<GlovesSourceSnapshot | null>(null)
  const [fileSignature, setFileSignature] = useState(() => localStorage.getItem(fileStorageKey) ?? '')
  const [selectedForDirectory, setSelectedForDirectory] = useState<string[]>([])
  const [directoryQuery, setDirectoryQuery] = useState('')
  const [directoryItems, setDirectoryItems] = useState<TNVEDDirectoryEntry[]>([])
  const [directoryLoading, setDirectoryLoading] = useState(false)
  const [directoryOpen, setDirectoryOpen] = useState(false)
  const [lightbox, setLightbox] = useState<string | null>(null)
  const [paramsOpen, setParamsOpen] = useState<boolean>(() => !storedParams(estimateId).usdKzt)
  const selectedFileRef = useRef<File | null>(null)
  const searchTimer = useRef<ReturnType<typeof window.setTimeout> | null>(null)
  const hydrationRef = useRef(false)
  const { tableWrapRef, topScrollRef, syncHorizontalScroll } = useSyncedHorizontalScroll(step === 2)

  useEffect(() => {
    try {
      localStorage.setItem(paramsStorageKey, JSON.stringify(params))
      const serialized = JSON.stringify(items)
      const withoutPhotos = items.map((item) => ({ ...item, photoUrl: '', photoUrls: [] }))
      localStorage.setItem(itemsStorageKey, JSON.stringify(withoutPhotos))
      if (photoStorageReady) {
        void glovesPhotoStore.save(estimateId, items).catch(() => {
          if (serialized.length <= MAX_LOCAL_STORAGE_CHARS) localStorage.setItem(itemsStorageKey, serialized)
          else setStorageNotice('Не удалось сохранить фотографии в браузере. Сохраните черновик на сервере.')
        })
      }
      if (hydrationRef.current) localStorage.setItem(localUpdatedStorageKey, String(Date.now()))
    } catch {
      setStorageNotice('Не удалось сохранить черновик в браузере. Экспортируйте результат перед закрытием страницы.')
    }
  }, [params, items, estimateId, paramsStorageKey, itemsStorageKey, localUpdatedStorageKey, photoStorageReady])

  useEffect(() => {
    let active = true
    void glovesPhotoStore.load(estimateId).catch(() => ({} as Record<string, string[]>)).then((storedPhotos) => glovesEstimatesApi.get(estimateId).then((estimate) => {
      if (!active) return
      const localUpdatedAt = Number(localStorage.getItem(localUpdatedStorageKey) || 0)
      const serverUpdatedAt = Date.parse(estimate.updatedAt) || 0
      const localDraftExists = localStorage.getItem(localUpdatedStorageKey) !== null
      const draft = { input: estimate.input, result: estimate.result, updatedAt: estimate.updatedAt }
      if (!localDraftExists || localUpdatedAt <= serverUpdatedAt) {
        const { items: _items, ...savedParams } = draft.input ?? ({} as GlovesCalculationInput)
        setParams(normalizeParams(savedParams))
        const restoredItems = Array.isArray(draft.input?.items) ? ensureItemIds(draft.input.items) : []
        setItems(mergeStoredPhotos(restoredItems, storedPhotos))
        if (draft.input?.source) {
          const savedMapping = normalizeStoredMapping(emptyMapping(), draft.input.source.mapping as Partial<GlovesMapping> | undefined)
          setSourceSnapshot({ ...draft.input.source, mapping: savedMapping })
          setSheet(draft.input.source.sheet || '')
          setHeaderRow(draft.input.source.headerRow || 1)
          setDataStartRow(draft.input.source.dataStartRow || 2)
          setHeaders(draft.input.source.headers ?? [])
          setRawRows(draft.input.source.rows ?? [])
          setMapping(savedMapping)
        }
        setResult(draft.result ?? null)
        setStep(draft.result ? 3 : Array.isArray(draft.input?.items) && draft.input.items.length > 0 ? 2 : 1)
        setStorageNotice(`Смета загружена: ${new Date(draft.updatedAt).toLocaleString('ru-RU')}`)
      } else {
        setItems((current) => mergeStoredPhotos(current, storedPhotos))
        setStorageNotice('Оставлены более свежие несохранённые изменения этого браузера')
      }
      hydrationRef.current = true
    }).catch((reason) => {
      if (!active) return
      setStorageNotice(`Серверный черновик недоступен: ${errorMessage(reason)}`)
      hydrationRef.current = true
    })).finally(() => {
      if (active) setPhotoStorageReady(true)
    })
    return () => { active = false }
  }, [estimateId, localUpdatedStorageKey])

  useEffect(() => {
    void searchDirectory('')
    return () => { if (searchTimer.current) window.clearTimeout(searchTimer.current) }
  }, [])

  const { setItem, addManualPhoto, addEmptyRow, removeRow, resetAutomaticNet } = rowActions({ setItems, setResult, setSelectedForDirectory, setError }, items.length, params.packageWeightKg)
  const { setMappingForHeader, setRawRowIncluded } = columnMappingActions(setMapping, setRawRows)
  const { recalculateResultPrice, decreaseResultPrices, restoreInitialResult, undoLastBulkPriceDecrease } = resultPriceActions({ items, result, previousBulkResult, setResult, setPreviousBulkResult, calculate })

  function setParam(field: keyof GlovesParams, value: string) {
    setParams((current) => ({ ...current, [field]: value }))
    if (field === 'packageWeightKg') {
      // Keep each row's fallback in sync with the global setting. Explicit
      // Excel/manual net values are still preserved by the backend and are
      // never overwritten by this change.
      setItems((current) => current.map((item) => ({ ...item, packageWeightKg: value })))
    }
    setResult(null)
  }

  async function parseFile(file: File, replaceExisting: boolean, requestedSheet = '') {
    const signature = `${file.name}:${file.size}:${file.lastModified}`
    setLoading(true); setError(''); setResult(null); setStep(1)
    try {
      const parsed = await calculatorApi.parseXlsx(file, { sheet: requestedSheet || undefined, headerRow, dataStartRow })
      setSheets(parsed.sheets)
      const selectedSheet = sheet || parsed.sheets[0]?.name || ''
      setSheet(selectedSheet)
      let newItems: GlovesItemInput[]
      setHeaders(parsed.headers ?? [])
      setRawRows(parsed.rows)
      setParsedImages(parsed.images)
      const nextMapping = emptyMapping()
      setMapping(nextMapping)
      newItems = buildGloveItems(parsed.rows, parsed.images, nextMapping, params.packageWeightKg, items.length)
      setSourceSnapshot({ filename: file.name, signature, sheet: selectedSheet, headerRow, dataStartRow, headers: parsed.headers ?? [], rows: parsed.rows, mapping: nextMapping })
      setItems((current) => replaceExisting ? newItems : [...current, ...newItems])
      setFileSignature(signature)
      localStorage.setItem(fileStorageKey, signature)
      if (parsed.warnings?.length) setStorageNotice(parsed.warnings.join(' '))
      else setStorageNotice('Файл обработан. Проверьте исключённые строки и сопоставление колонок.')
      if (newItems.length === 0) setError('В выбранном диапазоне нет заполненных строк товаров')
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setLoading(false)
    }
  }

  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    const signature = `${file.name}:${file.size}:${file.lastModified}`
    let replaceExisting = false
    if (items.length > 0 && fileSignature === signature) {
      if (!window.confirm('Этот файл уже загружен в раздел перчаток. Заменить текущие строки?')) return
      replaceExisting = true
    }
    selectedFileRef.current = file
    await parseFile(file, replaceExisting)
  }

  async function reparseSelectedSheet() {
    const file = selectedFileRef.current
    if (!file) {
      setError('Сначала загрузите XLSX-файл')
      return
    }
    if (items.length > 0 && !window.confirm('Перечитать выбранный лист и заменить текущие строки? Ручные исправления строк будут заменены.')) return
    await parseFile(file, true, sheet)
  }

  function applyMapping() {
    if (rawRows.length === 0) return
    if (items.length > 0 && !window.confirm('Пересобрать строки по выбранным колонкам? Ручные исправления текущих строк будут заменены.')) return
    setItems(buildGloveItems(rawRows, parsedImages, mapping, params.packageWeightKg, 0))
    setSourceSnapshot((current) => current ? { ...current, sheet, headerRow, dataStartRow, headers, rows: rawRows, mapping } : current)
    setSelectedForDirectory([])
    setResult(null)
    setError('')
    setStep(2)
  }

  async function searchDirectory(query: string) {
    setDirectoryQuery(query)
    setDirectoryLoading(true)
    try {
      setDirectoryItems(await tnvedDirectoryApi.list({ q: query, category: 'Перчатки' }))
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setDirectoryLoading(false)
    }
  }

  function changeDirectoryQuery(value: string) {
    setDirectoryQuery(value)
    if (searchTimer.current) window.clearTimeout(searchTimer.current)
    searchTimer.current = window.setTimeout(() => { void searchDirectory(value) }, 300)
  }

  function applyDirectoryEntry(entry: TNVEDDirectoryEntry) {
    if (selectedForDirectory.length === 0) return
    const snapshot = { entryId: entry.id, category: entry.category, name: entry.name, nameZh: entry.nameZh, code: entry.code, companyCoefficient: entry.companyCoefficient, duty: entry.duty, vat: entry.vat, verificationStatus: entry.verificationStatus, sourceKind: 'operator' }
    setItems((current) => current.map((item) => selectedForDirectory.includes(item.id ?? '') ? { ...item, standardName: entry.name, code: entry.code, coefficient: entry.companyCoefficient, directoryEntryId: entry.id, directorySnapshot: snapshot } : item))
    setSelectedForDirectory([])
    setDirectoryOpen(false)
    setResult(null)
    setError('')
  }

  function openDirectoryPicker(ids: string[]) {
    if (ids.length === 0) return
    setSelectedForDirectory(ids)
    const singleRow = ids.length === 1 ? items.find((item) => item.id === ids[0]) : undefined
    const prefill = singleRow?.supplierName?.trim() || ''
    setDirectoryOpen(true)
    if (searchTimer.current) window.clearTimeout(searchTimer.current)
    void searchDirectory(prefill)
  }

  function clearRowDirectory(id: string) {
    setItems((current) => current.map((item) => item.id === id ? { ...item, standardName: '', code: '', directoryEntryId: undefined, directorySnapshot: undefined } : item))
    setSelectedForDirectory((current) => current.filter((selected) => selected !== id))
    setResult(null)
  }

  async function calculate(itemsOverride?: GlovesItemInput[], saveServerDraft = true, updateInputItems = true) {
    const itemsToCalculate = itemsOverride ?? items
    if (itemsToCalculate.filter((item) => !item.excluded).length === 0) {
      setError('Добавьте хотя бы одну включённую строку')
      return
    }
    setLoading(true); setError('')
    if (itemsOverride && updateInputItems) setItems(itemsOverride)
    try {
      const itemsForRequest = itemsToCalculate.map(({ photoUrl: _photoUrl, photoUrls: _photoUrls, ...item }) => ({ ...item, photoUrl: '' }))
      const calculated = await calculatorApi.calculate({ ...params, items: itemsForRequest, source: sourceSnapshot ?? undefined })
      setResult(calculated)
      setStep(3)
      // The calculation response does not contain photos.  Saving a draft may
      // take longer because it validates and stores Base64 image data, so it
      // must not keep the calculator in the blocking "Считаем…" state.
      setLoading(false)
      setStorageNotice('Расчёт готов. Сохраняем черновик…')
      if (saveServerDraft) void glovesEstimatesApi.saveDraft(estimateId, { input: { ...params, items: itemsToCalculate, source: sourceSnapshot ?? undefined }, result: calculated })
        .then((saved) => { onEstimateSaved?.(saved); setStorageNotice('Черновик сохранён на сервере и в этом браузере') })
        .catch((reason) => setStorageNotice(`Расчёт выполнен, но серверный черновик не сохранён: ${errorMessage(reason)}`))
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setLoading(false)
    }
  }

  function changeResultPrice(index: number, value: string) {
    setResult((current) => current ? { ...current, items: current.items.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item) } : current)
  }

  async function saveDraft() {
    if (loading) return
    setLoading(true); setError('')
    try {
      const saved = await glovesEstimatesApi.saveDraft(estimateId, { input: { ...params, items, source: sourceSnapshot ?? undefined }, result: result ?? undefined })
      onEstimateSaved?.(saved)
      setStorageNotice('Черновик сохранён на сервере и в этом браузере')
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setLoading(false)
    }
  }

  async function exportExcel() {
    setLoading(true); setError('')
    try {
      const itemsForRequest = items.map(({ photoUrl: _photoUrl, photoUrls: _photoUrls, ...item }) => ({ ...item, photoUrl: '' }))
      const blob = await calculatorApi.exportXlsx({ ...params, items: itemsForRequest })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url; link.download = `Расчёт_перчатки_${new Date().toISOString().slice(0, 10)}.xlsx`; link.click(); URL.revokeObjectURL(url)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setLoading(false)
    }
  }

  function clearAll() {
    if (!window.confirm('Очистить параметры и все строки калькулятора перчаток?')) return
    setParams(emptyParams()); setItems([]); setResult(null); setStep(1); setSourceSnapshot(null); setSelectedForDirectory([]); setDirectoryOpen(false); setDirectoryQuery(''); setDirectoryItems([]); localStorage.removeItem(paramsStorageKey); localStorage.removeItem(itemsStorageKey); localStorage.removeItem(fileStorageKey); localStorage.removeItem(localUpdatedStorageKey)
    void glovesPhotoStore.remove(estimateId)
  }

  const includedCount = items.filter((item) => !item.excluded).length
  const allDirectorySelected = items.length > 0 && selectedForDirectory.length === items.length
  const hasSource = headers.length > 0 || items.length > 0
  const pickerRows = items.filter((item) => selectedForDirectory.includes(item.id ?? ''))
  const pickerTitle = pickerRows.length === 1 ? `Карточка для строки ${pickerRows[0].sourceRow || 'новой'}${pickerRows[0].supplierName ? ` · ${pickerRows[0].supplierName}` : ''}` : pickerRows.length > 1 ? `Карточка для ${pickerRows.length} выбранных строк` : 'Выберите карточку перчаток'
  const pickerPhotos = pickerRows.flatMap((row) => photosOf(row).map((photo) => ({ photo, row })))
  const deliverySummary = params.deliveryMode === 'wagons' ? (params.wagons ? `${params.wagons} ваг. × ${params.wagonTariffUsd} USD` : 'по вагонам') : params.deliveryMode === 'total' ? (params.deliveryTotalUsd ? `всего ${params.deliveryTotalUsd} USD` : 'общая сумма') : 'не задана'
  const feeSummary = params.customsFeeKzt ? `${params.customsFeeKzt} ₸` : params.mrp && params.feeMultiplier ? `${params.mrp} × ${params.feeMultiplier}` : '—'
  return <div className="gloves-calculator-embedded">
    <div className="gloves-page">
      <div className="gloves-heading"><p className="eyebrow">ОТДЕЛЬНЫЙ РАСЧЁТ</p><h1>Перчатки</h1><p>Таможенная стоимость = (стоимость + доп. расходы) × курс. Растаможка = таможенный сбор + пошлины + НДС. Количество в месте означает количество пар; деления на 2 нет.</p></div>
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
      <nav className="gloves-stepper" aria-label="Этапы расчёта перчаток">
        <button type="button" className={step === 1 ? 'active' : ''} onClick={() => setStep(1)}>1. Файл и колонки</button>
        <button type="button" className={step === 2 ? 'active' : ''} onClick={() => hasSource && setStep(2)} disabled={!hasSource}>2. Проверка строк</button>
        <button type="button" className={step === 3 ? 'active' : ''} onClick={() => result && setStep(3)} disabled={!result}>3. Результат</button>
      </nav>

      {step === 1 && <ImportStep
        loading={loading}
        sheets={sheets}
        sheet={sheet}
        headerRow={headerRow}
        dataStartRow={dataStartRow}
        headers={headers}
        rawRows={rawRows}
        mapping={mapping}
        selectedFileAvailable={Boolean(selectedFileRef.current)}
        mappingFieldOptions={mappingFieldOptions}
        upload={upload}
        addEmptyRow={addEmptyRow}
        setSheet={setSheet}
        setHeaderRow={setHeaderRow}
        setDataStartRow={setDataStartRow}
        reparseSelectedSheet={reparseSelectedSheet}
        mappedFieldForHeader={mappedFieldForHeader}
        setMappingForHeader={setMappingForHeader}
        setRawRowIncluded={setRawRowIncluded}
        applyMapping={applyMapping}
      />}

      {step === 2 && <ReviewStep
        params={params}
        items={items}
        result={result}
        selectedForDirectory={selectedForDirectory}
        paramsOpen={paramsOpen}
        loading={loading}
        storageNotice={storageNotice}
        deliverySummary={deliverySummary}
        feeSummary={feeSummary}
        tableWrapRef={tableWrapRef}
        topScrollRef={topScrollRef}
        includedCount={includedCount}
        allDirectorySelected={allDirectorySelected}
        setItems={setItems}
        setSelectedForDirectory={setSelectedForDirectory}
        setParamsOpen={setParamsOpen}
        setParam={setParam}
        syncHorizontalScroll={syncHorizontalScroll}
        openDirectoryPicker={openDirectoryPicker}
        clearRowDirectory={clearRowDirectory}
        setItem={setItem}
        removeRow={removeRow}
        setLightbox={setLightbox}
        addManualPhoto={addManualPhoto}
        resetAutomaticNet={resetAutomaticNet}
        calculate={calculate}
        saveDraft={saveDraft}
        clearAll={clearAll}
        onBackToImport={() => setStep(1)}
      />}

      {step === 3 && result && <ResultStep result={result} items={items} loading={loading} saveDraft={saveDraft} exportExcel={exportExcel} onBackToReview={() => { setPreviousBulkResult(null); setStep(2) }} onPriceChange={changeResultPrice} onPriceBlur={recalculateResultPrice} onBulkPriceDecrease={decreaseResultPrices} onRestoreInitial={restoreInitialResult} onUndoBulkPriceDecrease={undoLastBulkPriceDecrease} canUndoBulkPriceDecrease={Boolean(previousBulkResult)} />}

      {directoryOpen && <TnvedPickerModal
        title={pickerTitle}
        photos={pickerPhotos}
        multipleRows={pickerRows.length > 1}
        query={directoryQuery}
        entries={directoryItems}
        loading={directoryLoading}
        onClose={() => setDirectoryOpen(false)}
        onQueryChange={changeDirectoryQuery}
        onSearchNow={() => { if (searchTimer.current) window.clearTimeout(searchTimer.current); void searchDirectory(directoryQuery) }}
        onShowAll={() => void searchDirectory('')}
        onPick={applyDirectoryEntry}
        onPreview={setLightbox}
      />}
      <p className="gloves-disclaimer">Это внутренний предварительный расчёт. ИТС, код и ставки берутся из выбранной операторской карточки и не являются официальным подтверждением.</p>
    </div>
    {lightbox && <PhotoPreview photo={lightbox} onClose={() => setLightbox(null)} />}
  </div>
}

