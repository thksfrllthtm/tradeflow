import { ChangeEvent, ReactNode, useEffect, useRef, useState } from 'react'
import { mappedFieldForHeader, normalizeStoredMapping } from '../../shared/lib/excelColumns'
import { mergeStoredPhotos, photosOf } from '../../shared/lib/itemPhotos'
import { storageKey } from '../../shared/lib/localDraft'
import { useSyncedHorizontalScroll } from '../../shared/hooks/useSyncedHorizontalScroll'
import { GlovesCalculationInput, GlovesCalculationResult, GlovesEstimate, GlovesItemInput, GlovesParams, GlovesParseRow, GlovesSourceSnapshot, SheetInfo } from '../../shared/types'
import { goodsEstimatesApi } from '../goodsEstimatesApi'
import { GlovesMapping } from '../types'
import { emptyParams, ensureItemIds, FILE_KEY, goodsPhotoStore, ITEMS_KEY, LOCAL_UPDATED_KEY, normalizeParams, PARAMS_KEY, storedItems, storedParams } from '../lib/draftStorage'
import { buildGloveItems, emptyMapping, mappingFieldOptions, matchImportedTnvedCodes } from '../lib/importMapping'
import { requestErrorMessage } from '../lib/requestErrorMessage'
import { useDirectorySearch } from '../hooks/useDirectorySearch'
import { useDraftAutosave } from '../hooks/useDraftAutosave'
import '../../shared/styles/calculatorCommon.css'
import '../styles/calculator.css'
import './GoodsCalculator.css'
import { ImportStep } from '../components/ImportStep/ImportStep'
import { ReviewStep } from '../components/ReviewStep/ReviewStep'
import { ResultStep } from '../components/ResultStep/ResultStep'
import { DirectoryPickerEntry, DirectoryPickerModal } from '../components/DirectoryPickerModal/DirectoryPickerModal'
import { PhotoLightbox } from '../components/PhotoLightbox/PhotoLightbox'
import { columnMappingActions } from '../../shared/lib/columnMappingActions'
import { resultPriceActions } from '../../shared/lib/resultPriceActions'
import { exportItems } from '../../shared/lib/exportItems'
import { useBudgetDistribution } from '../../shared/hooks/useBudgetDistribution'
import { rowActions } from '../../shared/lib/rowActions'
import '../../shared/styles/calculatorPage.css'

type Props = { estimateId: number; onEstimateSaved?: (estimate: GlovesEstimate) => void; headerSlot: ReactNode }

export function GoodsCalculator({ estimateId, onEstimateSaved, headerSlot }: Props) {
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
  const [usdRateLoading, setUsdRateLoading] = useState(false)
  const [usdRateNotice, setUsdRateNotice] = useState('')
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
  const [directoryOpen, setDirectoryOpen] = useState(false)
  const [lightbox, setLightbox] = useState<{ photos: string[]; index: number } | null>(null)
  const [paramsOpen, setParamsOpen] = useState<boolean>(() => !storedParams(estimateId).usdKzt)
  const selectedFileRef = useRef<File | null>(null)
  const autoRateRequestedRef = useRef(false)
  const hydrationRef = useRef(false)
  const { tableWrapRef, topScrollRef, syncHorizontalScroll } = useSyncedHorizontalScroll(step === 2)
  const calculationRequestRef = useRef(0)

  function openPhotoGallery(photos: string[], index = 0) {
    if (photos.length === 0) return
    setLightbox({ photos, index: Math.max(0, Math.min(index, photos.length - 1)) })
  }

  useDraftAutosave({ estimateId, params, items, photoStorageReady, hydrationRef, onNotice: setStorageNotice })
  const directorySearch = useDirectorySearch(setError)

  useEffect(() => {
    let active = true
    void goodsPhotoStore.load(estimateId).catch(() => ({} as Record<string, string[]>)).then((storedPhotos) => goodsEstimatesApi.get(estimateId).then((estimate) => {
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
      setStorageNotice(`Серверный черновик недоступен: ${requestErrorMessage(reason)}`)
      hydrationRef.current = true
    })).finally(() => {
      if (active) setPhotoStorageReady(true)
    })
    return () => { active = false }
  }, [estimateId, localUpdatedStorageKey])

  const { setItem, addManualPhoto, addEmptyRow, removeRow, resetAutomaticNet } = rowActions({ setItems, setResult, setSelectedForDirectory, setError }, items.length, params.packageWeightKg)
  const { setMappingForHeader, setRawRowIncluded } = columnMappingActions(setMapping, setRawRows)
  const { recalculateResultPrice, decreaseResultPrices, restoreInitialResult, undoLastBulkPriceDecrease, applyDistributedPrices } = resultPriceActions({ items, result, previousBulkResult, setResult, setPreviousBulkResult, calculate })
  const budget = useBudgetDistribution({ kind: 'goods', estimateId, unit: 'шт', result, items, preview: previewCalculation, apply: applyDistributedPrices })

  function setParam(field: keyof GlovesParams, value: string) {
    setParams((current) => ({ ...current, [field]: value }))
    if (field === 'assessmentDate' || field === 'usdKzt') setUsdRateNotice('')
    if (field === 'packageWeightKg') {
      // Общая упаковка меняет только строки, которые идут по общему значению.
      // Строка со своей упаковкой (значение отличается от общего) остаётся как есть.
      // Нетто из Excel или введённое вручную сервер не пересчитывает в любом случае.
      const previous = params.packageWeightKg
      setItems((current) => current.map((item) => item.packageWeightKg === previous ? { ...item, packageWeightKg: value } : item))
    }
    setResult(null)
  }

  async function loadUsdRate() {
    if (!params.assessmentDate) {
      setError('Сначала укажите дату расчёта')
      return
    }
    setUsdRateLoading(true)
    setError('')
    try {
      const rate = await goodsEstimatesApi.usdRate(params.assessmentDate)
      setParam('usdKzt', rate.rate)
      setUsdRateNotice(`${rate.source} · ${rate.date}`)
    } catch (reason) {
      setError(requestErrorMessage(reason))
      setUsdRateNotice('')
    } finally {
      setUsdRateLoading(false)
    }
  }

  useEffect(() => {
    if (step !== 2 || !hydrationRef.current || autoRateRequestedRef.current || params.usdKzt.trim() !== '') return
    autoRateRequestedRef.current = true
    void loadUsdRate()
  }, [step, params.usdKzt])

  async function parseFile(file: File, replaceExisting: boolean, requestedSheet = '') {
    const signature = `${file.name}:${file.size}:${file.lastModified}`
    setLoading(true); setError(''); setResult(null); setStep(1)
    try {
      const parsed = await goodsEstimatesApi.parseXlsx(file, { sheet: requestedSheet || undefined, headerRow, dataStartRow })
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
      setError(requestErrorMessage(reason))
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
      if (!window.confirm('Этот файл уже загружен в эту смету. Заменить текущие строки?')) return
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

  async function applyMapping() {
    if (rawRows.length === 0) return
    if (items.length > 0 && !window.confirm('Пересобрать строки по выбранным колонкам? Ручные исправления текущих строк будут заменены.')) return
    setLoading(true)
    setError('')
    try {
      const importedItems = buildGloveItems(rawRows, parsedImages, mapping, params.packageWeightKg, 0)
      const matched = await matchImportedTnvedCodes(importedItems)
      setItems(matched.items)
      setSourceSnapshot((current) => current ? { ...current, sheet, headerRow, dataStartRow, headers, rows: rawRows, mapping } : current)
      setSelectedForDirectory([])
      setResult(null)
      if (matched.matched > 0 || matched.missing > 0) {
        setStorageNotice(`Коды ТН ВЭД: автоматически сопоставлено ${matched.matched}${matched.missing > 0 ? `, не найдено ${matched.missing}` : ''}.`)
      }
      setStep(2)
    } catch (reason) {
      setError(requestErrorMessage(reason))
    } finally {
      setLoading(false)
    }
  }

  function applyDirectoryEntry(entry: DirectoryPickerEntry) {
    if (selectedForDirectory.length === 0) return
    const linked = entry.productDirectory?.tnved ?? entry
    const displayName = entry.productDirectory?.name || linked.name
    const displayNameZh = entry.productDirectory?.nameZh || entry.productDirectory?.tnvedNameZh || linked.nameZh
    const fullDisplayName = [displayName, displayNameZh].filter((value) => value?.trim()).join(' / ')
    const selectedProduct = entry.productDirectory
    const companyCoefficient = selectedProduct?.companyCoefficient || ''
    const snapshot = { entryId: linked.id, category: linked.category, name: displayName, nameZh: displayNameZh, code: linked.code, ...(selectedProduct ? { companyCoefficient } : {}), antiDumpingPercent: linked.antiDumpingPercent, duty: linked.duty, vat: linked.vat, verificationStatus: linked.verificationStatus, sourceKind: 'operator' }
    // Название строки меняется только при выборе товара из справочника товаров. Код ТН ВЭД
    // сам по себе название не меняет — остаётся название из Excel.
    const names = (item: GlovesItemInput) => selectedProduct ? { name: fullDisplayName, supplierName: fullDisplayName } : { name: item.name, supplierName: item.supplierName }
    setItems((current) => current.map((item) => selectedForDirectory.includes(item.id ?? '') ? { ...item, ...names(item), standardName: fullDisplayName, code: linked.code, coefficient: selectedProduct ? companyCoefficient : '', directoryEntryId: linked.id, productDirectoryId: selectedProduct?.id, directorySnapshot: snapshot } : item))
    setSelectedForDirectory([])
    setDirectoryOpen(false)
    setResult(null)
    setError('')
  }

  function openDirectoryPicker(ids: string[]) {
    if (ids.length === 0) return
    setSelectedForDirectory(ids)
    setDirectoryOpen(true)
    directorySearch.restart()
  }

  function clearRowDirectory(id: string) {
    setItems((current) => current.map((item) => item.id === id ? { ...item, standardName: '', code: '', coefficient: '', directoryEntryId: undefined, productDirectoryId: undefined, directorySnapshot: undefined } : item))
    setSelectedForDirectory((current) => current.filter((selected) => selected !== id))
    setResult(null)
  }

  async function calculate(itemsOverride?: GlovesItemInput[], saveServerDraft = true, updateInputItems = true) {
    const itemsToCalculate = itemsOverride ?? items
    if (itemsToCalculate.filter((item) => !item.excluded).length === 0) {
      setError('Добавьте хотя бы одну включённую строку')
      return
    }
    const requestId = calculationRequestRef.current + 1
    calculationRequestRef.current = requestId
    setLoading(true); setError('')
    if (itemsOverride && updateInputItems) setItems(itemsOverride)
    try {
      const itemsForRequest = itemsToCalculate.map(({ photoUrl: _photoUrl, photoUrls: _photoUrls, ...item }) => ({ ...item, photoUrl: '' }))
      const calculated = await goodsEstimatesApi.calculate({ ...params, items: itemsForRequest, source: sourceSnapshot ?? undefined })
      if (requestId !== calculationRequestRef.current) return
      setResult(calculated)
      setStep(3)
      // The calculation response does not contain photos.  Saving a draft may
      // take longer because it validates and stores Base64 image data, so it
      // must not keep the calculator in the blocking "Считаем…" state.
      setLoading(false)
      setStorageNotice('Расчёт готов. Сохраняем черновик…')
      if (saveServerDraft) void goodsEstimatesApi.saveDraft(estimateId, { input: { ...params, items: itemsToCalculate, source: sourceSnapshot ?? undefined }, result: calculated })
        .then((saved) => { onEstimateSaved?.(saved); setStorageNotice('Черновик сохранён на сервере и в этом браузере') })
        .catch((reason) => setStorageNotice(`Расчёт выполнен, но серверный черновик не сохранён: ${requestErrorMessage(reason)}`))
    } catch (reason) {
      if (requestId === calculationRequestRef.current) setError(requestErrorMessage(reason))
    } finally {
      if (requestId === calculationRequestRef.current) setLoading(false)
    }
  }

  // Пробный расчёт для подбора цен под бюджет: без показа результата и без сохранения черновика.
  function previewCalculation(nextItems: GlovesItemInput[]) {
    const itemsForRequest = nextItems.map(({ photoUrl: _photoUrl, photoUrls: _photoUrls, ...item }) => ({ ...item, photoUrl: '' }))
    return goodsEstimatesApi.calculate({ ...params, items: itemsForRequest, source: sourceSnapshot ?? undefined })
  }

  function returnToReview() {
    budget.reset()
    // Цена, изменённая на этапе результата, является временной. При возврате
    // к проверке нужно начать новый расчёт с исходных данных строк (цена из Excel остаётся).
    calculationRequestRef.current += 1
    setLoading(false)
    setPreviousBulkResult(null)
    setResult(null)
    setItems((current) => current.map((item) => ({ ...item, pricePerPair: item.sourcePricePerPair ?? '' })))
    setStep(2)
  }

  function changeResultPrice(index: number, value: string) {
    // Keep the edited value in the source rows as well as in the result. The
    // next row recalculation is built from `items`; without this mirror the
    // previous row still contains its old price and appears to disappear.
    setItems((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item))
    setResult((current) => current ? { ...current, items: current.items.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item) } : current)
  }

  async function saveDraft() {
    if (loading) return
    setLoading(true); setError('')
    try {
      const saved = await goodsEstimatesApi.saveDraft(estimateId, { input: { ...params, items, source: sourceSnapshot ?? undefined }, result: result ?? undefined })
      onEstimateSaved?.(saved)
      setStorageNotice('Черновик сохранён на сервере и в этом браузере')
    } catch (reason) {
      setError(requestErrorMessage(reason))
    } finally {
      setLoading(false)
    }
  }

  async function exportExcel() {
    setLoading(true); setError('')
    try {
      const blob = await goodsEstimatesApi.exportXlsx({ ...params, items: exportItems(items, result), source: sourceSnapshot ?? undefined })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url; link.download = `Расчёт_товаров_${new Date().toISOString().slice(0, 10)}.xlsx`; link.click(); URL.revokeObjectURL(url)
    } catch (reason) {
      setError(requestErrorMessage(reason))
    } finally {
      setLoading(false)
    }
  }

  function clearAll() {
    if (!window.confirm('Очистить параметры и все строки этой сметы?')) return
    setParams(emptyParams()); setItems([]); setResult(null); setStep(1); setSourceSnapshot(null); setSelectedForDirectory([]); setDirectoryOpen(false); directorySearch.reset(); localStorage.removeItem(paramsStorageKey); localStorage.removeItem(itemsStorageKey); localStorage.removeItem(fileStorageKey); localStorage.removeItem(localUpdatedStorageKey)
    void goodsPhotoStore.remove(estimateId)
  }

  const includedCount = items.filter((item) => !item.excluded).length
  const allDirectorySelected = items.length > 0 && selectedForDirectory.length === items.length
  const hasSource = headers.length > 0 || items.length > 0
  const pickerRows = items.filter((item) => selectedForDirectory.includes(item.id ?? ''))
  const pickerTitle = pickerRows.length === 1 ? `Выбор товара или кода для строки ${pickerRows[0].sourceRow || 'новой'}${pickerRows[0].supplierName ? ` · ${pickerRows[0].supplierName}` : ''}` : pickerRows.length > 1 ? `Выбор товара или кода для ${pickerRows.length} выбранных строк` : 'Выберите товар или код ТН ВЭД'
  const pickerPhotos = pickerRows.flatMap((row) => photosOf(row).map((photo) => ({ photo, row })))
  const deliverySummary = params.deliveryMode === 'wagons' ? (params.wagons ? `${params.wagons} ваг. × ${params.wagonTariffUsd} USD` : 'по вагонам') : params.deliveryMode === 'total' ? (params.deliveryTotalUsd ? `всего ${params.deliveryTotalUsd} USD` : 'общая сумма') : params.deliveryMode === 'none' ? 'без доставки' : 'не задана'
  const feeSummary = params.customsFeeKzt ? `${params.customsFeeKzt} ₸` : params.mrp && params.feeMultiplier ? `${params.mrp} × ${params.feeMultiplier}` : '—'
  const stepList: { value: 1 | 2 | 3; label: string; available: boolean }[] = [
    { value: 1, label: 'Файл и колонки', available: true },
    { value: 2, label: 'Проверка строк', available: hasSource },
    { value: 3, label: 'Результат', available: Boolean(result) },
  ]
  const calculatorStepper = <nav className="est-steps" aria-label="Этапы расчёта товаров">
    {stepList.map(({ value, label, available }) => {
      const done = value < step && available
      return <button type="button" key={value} className={step === value ? 'active' : done ? 'done' : undefined} aria-current={step === value ? 'step' : undefined} onClick={() => available && setStep(value)} disabled={!available} title={label}>
        <span className="est-step-num">{done ? '✓' : value}</span><span className="est-step-label">{label}</span>
      </button>
    })}
  </nav>
  return <div className="gloves-calculator-embedded">
    <div className="gloves-page">
      <div className="estimate-bar">{headerSlot}{calculatorStepper}</div>
      {error && <div className="banner error gloves-request-error" role="alert"><div><strong>Не удалось выполнить действие</strong><p>{error}</p></div><button type="button" onClick={() => setError('')} aria-label="Закрыть сообщение">×</button></div>}

      {step === 1 && <ImportStep
        loading={loading}
        sheets={sheets}
        sheet={sheet}
        headerRow={headerRow}
        dataStartRow={dataStartRow}
        headers={headers}
        rawRows={rawRows}
        mapping={mapping}
        fileName={selectedFileRef.current?.name ?? ''}
        selectedFileAvailable={Boolean(selectedFileRef.current)}
        mappingFieldOptions={mappingFieldOptions}
        upload={upload}
        addEmptyRow={() => { addEmptyRow(); setStep(2) }}
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
         usdRateLoading={usdRateLoading}
         usdRateNotice={usdRateNotice}
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
         loadUsdRate={loadUsdRate}
        syncHorizontalScroll={syncHorizontalScroll}
        openDirectoryPicker={openDirectoryPicker}
        clearRowDirectory={clearRowDirectory}
        setItem={setItem}
        removeRow={removeRow}
        setLightbox={openPhotoGallery}
        addManualPhoto={addManualPhoto}
        resetAutomaticNet={resetAutomaticNet}
        addEmptyRow={addEmptyRow}
        calculate={calculate}
        saveDraft={saveDraft}
        clearAll={clearAll}
      />}

      {step === 3 && result && <ResultStep result={result} items={items} loading={loading} saveDraft={saveDraft} exportExcel={exportExcel} onBackToReview={returnToReview} onPriceChange={changeResultPrice} onPriceBlur={recalculateResultPrice} onBulkPriceDecrease={decreaseResultPrices} onRestoreInitial={() => { budget.reset(); restoreInitialResult() }} onUndoBulkPriceDecrease={() => { budget.reset(); undoLastBulkPriceDecrease() }} canUndoBulkPriceDecrease={Boolean(previousBulkResult)} budget={budget} />}

      {directoryOpen && <DirectoryPickerModal
        title={pickerTitle}
        photos={pickerPhotos}
        multipleRows={pickerRows.length > 1}
        mode={directorySearch.mode}
        query={directorySearch.query}
        entries={directorySearch.entries}
        counts={directorySearch.counts}
        loading={directorySearch.loading}
        onClose={() => setDirectoryOpen(false)}
        onModeChange={directorySearch.changeMode}
        onQueryChange={directorySearch.changeQuery}
        onSearchNow={directorySearch.searchNow}
        onShowAll={directorySearch.restart}
        onPick={applyDirectoryEntry}
        onPreview={openPhotoGallery}
      />}
      <p className="gloves-disclaimer">Это внутренний предварительный расчёт. ИТС, код и ставки берутся из выбранной операторской карточки и не являются официальным подтверждением.</p>
    </div>
    {lightbox && <PhotoLightbox photos={lightbox.photos} startIndex={lightbox.index} onClose={() => setLightbox(null)} />}
  </div>
}
