import { ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { mappedFieldForHeader, normalizeStoredMapping } from '../lib/excelColumns'
import { mergeStoredPhotos } from '@/shared/lib/itemPhotos'
import { GoodsCalculationInput, GoodsCalculationResult, GoodsEstimateSummary, GoodsItemInput, GoodsParams, GoodsResultTools, GoodsSourceSnapshot, SirParams } from '@/shared/types/goods'
import { goodsEstimatesApi } from '../goodsEstimatesApi'
import { CalculatorActivity, GoodsMapping } from '../types'
import { draftSnapshot, emptyParams, ensureItemIds, FILE_KEY, goodsPhotoStore, ITEMS_KEY, LOCAL_UPDATED_KEY, normalizeParams, PARAMS_KEY, RESULT_KEY, storageKey, storedItems, storedParams, storedResult } from '../lib/draftStorage'
import { emptyMapping, mappingFieldOptions, upgradeLegacyMapping } from '../lib/importMapping'
import { lookupNomenclature, withNomenclatureRates } from '../lib/nomenclatureRates'
import { requestErrorMessage } from '../lib/requestErrorMessage'
import { savedAtText } from '@/shared/lib/dates'
import { useDirectorySearch } from '../hooks/useDirectorySearch'
import { useDraftAutosave } from '../hooks/useDraftAutosave'
import { useServerAutosave } from '../hooks/useServerAutosave'
import { useUsdRate } from '../hooks/useUsdRate'
import { useExcelImport } from '../hooks/useExcelImport'
import { CalculatorStepper } from './CalculatorStepper'
import { CodeNotice, CodeNoticeState } from './CodeNotice'
import '../goods.css'
import './GoodsCalculator.css'
import { ImportStep } from '../components/ImportStep/ImportStep'
import { ReviewStep } from '../components/ReviewStep/ReviewStep'
import { ResultStep } from '../components/ResultStep/ResultStep'
import { DirectoryPickerEntry, DirectoryPickerModal } from '../components/DirectoryPickerModal/DirectoryPickerModal'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { RowPhotos } from '../components/RowPhotos/RowPhotos'
import { makeMainRowPhoto, moveRowPhoto, removeRowPhoto } from '../lib/rowPhotos'
import { resultPriceActions } from '../lib/resultPriceActions'
import { exportItems } from '../lib/exportItems'
import { useBudgetDistribution } from '../hooks/useBudgetDistribution'
import { rowActions } from '../lib/rowActions'
import { checkFromResult, loadReviewCheck, ReviewCheck, saveReviewCheck } from '../lib/reviewCheck'
import './calculatorPage.css'
import { AgentPanel } from '../agent/AgentPanel'
import { useRowDecisions } from '../hooks/useRowDecisions'
import { useAgentRows } from '../agent/useAgentRows'
import { useAgentBatch } from '../agent/useAgentBatch'

// exportName — название сметы и клиент для имени файла выгрузки.
type Props = { estimateId: number; onEstimateSaved?: (estimate: GoodsEstimateSummary) => void; headerSlot: ReactNode; exportName?: string }

export function GoodsCalculator({ estimateId, onEstimateSaved, headerSlot, exportName }: Props) {
  const paramsStorageKey = storageKey(PARAMS_KEY, estimateId)
  const itemsStorageKey = storageKey(ITEMS_KEY, estimateId)
  const localUpdatedStorageKey = storageKey(LOCAL_UPDATED_KEY, estimateId)
  const [params, setParams] = useState<GoodsParams>(() => storedParams(estimateId))
  const [items, setItems] = useState<GoodsItemInput[]>(() => storedItems(estimateId))
  const [step, setStep] = useState<1 | 2 | 3>(() => storedResult(estimateId) ? 3 : storedItems(estimateId).length > 0 ? 2 : 1)
  const [result, setResult] = useState<GoodsCalculationResult | null>(() => storedResult(estimateId))
  const [previousBulkResult, setPreviousBulkResult] = useState<GoodsCalculationResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [activity, setActivity] = useState<CalculatorActivity>('')
  const [error, setError] = useState('')
  const [storageNotice, setStorageNotice] = useState('')
  const [photoStorageReady, setPhotoStorageReady] = useState(false)
  const [sourceSnapshot, setSourceSnapshot] = useState<GoodsSourceSnapshot | null>(null)
  const [selectedForDirectory, setSelectedForDirectory] = useState<string[]>([])
  const [pickerTargets, setPickerTargets] = useState<string[]>([])
  // Ошибки последнего расчёта по id строк (этап 2 показывает их и после правок) и удалённая
  // строка, которую можно вернуть.
  const [reviewCheck, setReviewCheck] = useState<ReviewCheck | null>(() => {
    const stored = storedResult(estimateId)
    return stored ? checkFromResult(estimateId, storedItems(estimateId), stored) : loadReviewCheck(estimateId)
  })
  const [removedRow, setRemovedRow] = useState<{ item: GoodsItemInput; index: number } | null>(null)
  const [directoryOpen, setDirectoryOpen] = useState(false)
  const [lightbox, setLightbox] = useState<{ photos: string[]; index: number } | null>(null)
  // Фото строки на этапе проверки: просмотр с удалением и переносом в другую строку.
  const [rowPhotos, setRowPhotos] = useState<{ itemId: string; index: number } | null>(null)
  // Строка, для которой открыт чат подбора кода ИИ.
  const [agentRowId, setAgentRowId] = useState<string | null>(null)
  // Уведомление внизу после смены кода на этапе результата.
  const [codeNotice, setCodeNotice] = useState<CodeNoticeState | null>(null)
  const hydrationRef = useRef(false)
  // Черновик, который сейчас лежит на сервере (draftSnapshot): отличается от него только то,
  // что изменено в этом браузере и ещё не сохранено.
  const savedSnapshotRef = useRef<string | null>(null)
  const [savedVersion, setSavedVersion] = useState(0)
  function markSaved(savedParams: GoodsParams, savedItems: GoodsItemInput[], savedResult: GoodsCalculationResult | null) {
    savedSnapshotRef.current = draftSnapshot(savedParams, savedItems, savedResult)
    setSavedVersion((version) => version + 1)
  }
  const calculationRequestRef = useRef(0)

  const closeRowPhotos = useCallback(() => setRowPhotos(null), [])
  function moveRowPhotoTo(fromId: string, index: number, toId: string) {
    const moved = moveRowPhoto(items, fromId, index, toId)
    if (moved) setItems(moved)
    else setError('Фото не перенесено: у строки уже 20 фото')
  }

  function openPhotoGallery(photos: string[], index = 0) {
    if (photos.length === 0) return
    setLightbox({ photos, index: Math.max(0, Math.min(index, photos.length - 1)) })
  }

  useDraftAutosave({ estimateId, params, items, result, photoStorageReady, hydrationRef, savedSnapshotRef, savedVersion, onNotice: setStorageNotice })
  // Смета загружена с сервера: с этого момента правки сами сохраняются на сервер.
  const [serverReady, setServerReady] = useState(false)
  const autosave = useServerAutosave({
    estimateId,
    state: { params, items, result, source: sourceSnapshot },
    enabled: serverReady,
    onSaving: () => setStorageNotice('Сохраняем на сервере…'),
    onSaved: (summary, saved) => {
      markSaved(saved.params, saved.items, saved.result)
      onEstimateSaved?.(summary)
      setStorageNotice(savedAtText())
    },
    onError: (message) => setStorageNotice(`Не сохранено на сервере: ${message}. Повторим автоматически`),
  })
  const usdRate = useUsdRate({ params, ready: photoStorageReady, setParam, setError })
  const excel = useExcelImport({ estimateId, items, packageWeightKg: params.packageWeightKg, setItems, setResult, setStep, setLoading, setError, setStorageNotice, setSourceSnapshot, setSelectedForDirectory })
  const directorySearch = useDirectorySearch(setError)
  const agentRows = useAgentRows(estimateId, step === 2)
  const agentBatch = useAgentBatch(estimateId, items, agentRows, setError)
  // Кнопка «В базу» в строках: что уже в базе решений и сохранение строк.
  const rowDecisions = useRowDecisions(estimateId, items, serverReady && step === 2)

  useEffect(() => {
    let active = true
    void goodsPhotoStore.load(estimateId).catch(() => ({} as Record<string, string[]>)).then((storedPhotos) => goodsEstimatesApi.get(estimateId).then((estimate) => {
      if (!active) return
      const localUpdatedAt = Number(localStorage.getItem(localUpdatedStorageKey) || 0)
      const serverUpdatedAt = Date.parse(estimate.updatedAt) || 0
      const localDraftExists = localStorage.getItem(localUpdatedStorageKey) !== null
      const draft = { input: estimate.input, result: estimate.result, updatedAt: estimate.updatedAt }
      const { items: _items, ...savedParams } = draft.input ?? ({} as GoodsCalculationInput)
      const serverParams = normalizeParams(savedParams)
      const restoredItems = Array.isArray(draft.input?.items) ? ensureItemIds(draft.input.items) : []
      const serverSource = draft.input?.source
        ? { ...draft.input.source, mapping: upgradeLegacyMapping(normalizeStoredMapping(emptyMapping(), draft.input.source.mapping as Partial<GoodsMapping> | undefined)) }
        : null
      markSaved(serverParams, restoredItems, draft.result ?? null)
      autosave.setBaseline({ params: serverParams, items: restoredItems, result: draft.result ?? null, source: serverSource })
      // Исходная таблица Excel хранится только на сервере — берём её в любом случае, иначе
      // автосохранение стёрло бы её на сервере.
      if (serverSource) {
        setSourceSnapshot(serverSource)
        excel.restore(serverSource)
      }
      // Прежняя версия сайта ставила отметку «в браузере новее» при простом открытии сметы. Если
      // строки и параметры совпадают с сервером, а своего результата в браузере нет, отметка
      // ложная: берём серверную версию вместе с результатом расчёта.
      const spuriousLocalMark = !storedResult(estimateId) && draftSnapshot(params, items, null) === draftSnapshot(serverParams, restoredItems, null)
      if (!localDraftExists || localUpdatedAt <= serverUpdatedAt || spuriousLocalMark) {
        setParams(serverParams)
        setItems(mergeStoredPhotos(restoredItems, storedPhotos))
        setResult(draft.result ?? null)
        if (draft.result) setReviewCheck(checkFromResult(estimateId, restoredItems, draft.result))
        setStep(draft.result ? 3 : Array.isArray(draft.input?.items) && draft.input.items.length > 0 ? 2 : 1)
        setStorageNotice(savedAtText(draft.updatedAt))
      } else {
        setItems((current) => mergeStoredPhotos(current, storedPhotos))
        setStorageNotice('Показаны изменения из этого браузера — сохраняем их на сервер')
      }
      hydrationRef.current = true
      setServerReady(true)
    }).catch((reason) => {
      if (!active) return
      setStorageNotice(`Серверный черновик недоступен: ${requestErrorMessage(reason)}`)
      hydrationRef.current = true
    })).finally(() => {
      if (active) setPhotoStorageReady(true)
    })
    return () => { active = false }
  }, [estimateId, localUpdatedStorageKey])

  const { addManualPhoto, addEmptyRow, removeRow } = rowActions({ setItems, setResult, setSelectedForDirectory, setError }, items.length, params.packageWeightKg)
  const { withResultPrices, changeResultPrice, recalculateResultPrice, restoreInitialResult, undoLastPriceChange, applyDistributedPrices, setResultIncluded, chooseAntiDumping } = resultPriceActions({ items, result, previousBulkResult, setItems, setResult, setPreviousBulkResult, calculate })
  // Подбор под бюджет и сверка по ИТП хранятся в смете (параметр resultTools) и сохраняются автосохранением.
  const setResultTool = <K extends keyof GoodsResultTools>(key: K, value: GoodsResultTools[K]) =>
    setParams((current) => ({ ...current, resultTools: { ...current.resultTools, [key]: value } }))
  const budget = useBudgetDistribution({ estimateId, unit: 'шт', result, items, preview: previewCalculation, apply: applyDistributedPrices, saved: params.resultTools?.budget, onSave: (value) => setResultTool('budget', value) })

  useEffect(() => { saveReviewCheck(reviewCheck, estimateId) }, [reviewCheck, estimateId])

  // Удалённую строку можно вернуть на её место: ошибки расчёта вернутся вместе с ней (они по id строки).
  function removeRowWithUndo(id: string) {
    const index = items.findIndex((item) => item.id === id)
    if (index >= 0) setRemovedRow({ item: items[index], index })
    removeRow(id)
  }

  function restoreRemovedRow() {
    if (!removedRow) return
    const { item, index } = removedRow
    setItems((current) => current.some((row) => row.id === item.id) ? current : [...current.slice(0, index), item, ...current.slice(index)])
    setRemovedRow(null)
    setResult(null)
  }

  function setParam(field: keyof GoodsParams, value: string) {
    setParams((current) => ({ ...current, [field]: value }))
    usdRate.paramChanged(field)
    if (field === 'packageWeightKg') {
      // Общая упаковка меняет только строки, которые идут по общему значению.
      // Строка со своей упаковкой (значение отличается от общего) остаётся как есть.
      // Нетто из Excel или введённое вручную сервер не пересчитывает в любом случае.
      const previous = params.packageWeightKg
      setItems((current) => current.map((item) => item.packageWeightKg === previous ? { ...item, packageWeightKg: value } : item))
    }
    setResult(null)
  }

  // Товар даёт строке название, код и ИТС; код номенклатуры — только код. Ставки в обоих случаях —
  // пошлина ЕТТ и НДС из номенклатуры (окончательно их подставит сервер при расчёте).
  function applyDirectoryEntry(entry: DirectoryPickerEntry) {
    if (pickerTargets.length === 0) return
    const product = entry.kind === 'product' ? entry.product : null
    const fullDisplayName = product ? [product.name, product.nameZh].filter((value) => value?.trim()).join(' / ') : ''
    const companyCoefficient = product?.companyCoefficient || ''
    const card: Record<string, unknown> = product ? { category: product.category, name: product.name, nameZh: product.nameZh, code: entry.code, companyCoefficient } : { code: entry.code }
    const snapshot = entry.nomenclature ? withNomenclatureRates(entry.nomenclature, directorySearch.source, card) : card
    const standardName = product ? fullDisplayName : String(snapshot.name ?? '')
    // Название строки меняется только при выборе товара из справочника товаров. Код ТН ВЭД
    // сам по себе название не меняет — остаётся название из Excel.
    const names = (item: GoodsItemInput) => product ? { name: fullDisplayName, supplierName: fullDisplayName } : { name: item.name, supplierName: item.supplierName }
    const nextItems = items.map((item) => pickerTargets.includes(item.id ?? '') ? { ...item, ...names(item), standardName, code: entry.code, coefficient: companyCoefficient, productDirectoryId: product?.id, directorySnapshot: snapshot } : item)
    // Выбор для выбранных галочкой строк снимает галочки; выбор для одной строки их не трогает.
    if (pickerTargets.length > 1 || pickerTargets.every((id) => selectedForDirectory.includes(id))) setSelectedForDirectory((current) => current.filter((id) => !pickerTargets.includes(id)))
    const targets = pickerTargets
    setPickerTargets([])
    setDirectoryOpen(false)
    setError('')
    // На этапе результата («Сменить» в панели строки) — сразу пересчёт с текущими ценами, без возврата к проверке.
    if (step === 3 && result) {
      void recalculateCode(nextItems, items.filter((item) => targets.includes(item.id ?? '')))
      return
    }
    setItems(nextItems)
    setResult(null)
  }

  // Код строки сменили на этапе результата: смета пересчитывается с текущими ценами. Получилось — новые строки
  // и уведомление с «Отменить» (previous — строки до смены); не получилось — прежние код и результат остаются.
  async function recalculateCode(nextItems: GoodsItemInput[], previous?: GoodsItemInput[]) {
    const failure = await calculate(withResultPrices(nextItems), false, undefined, false)
    if (failure) {
      setCodeNotice({ text: `${previous ? 'Код не изменён' : 'Прежний код не вернулся'}: ${failure}`, failed: true })
      return
    }
    setItems(nextItems)
    setCodeNotice(previous ? { text: 'Код изменён, смета пересчитана', previous } : { text: 'Прежний код возвращён, смета пересчитана' })
  }

  // «Отменить» в уведомлении: вернуть строкам прежний код и товар; цену оставить текущей.
  function undoCodeChange(previous: GoodsItemInput[]) {
    const nextItems = items.map((item) => {
      const before = previous.find((row) => row.id === item.id)
      return before ? { ...before, pricePerPair: item.pricePerPair, antiDumpingChoice: item.antiDumpingChoice, excluded: item.excluded } : item
    })
    setCodeNotice(null)
    void recalculateCode(nextItems)
  }

  // Код из подбора ИИ: ставки — пошлина ЕТТ и НДС из номенклатуры, как при выборе кода вручную.
  async function applyAgentCode(id: string, code: string) {
    const found = await lookupNomenclature([code]).catch(() => null)
    const nomenclature = found?.byCode.get(code)
    const snapshot = nomenclature ? withNomenclatureRates(nomenclature, found?.source ?? null, { code }) : { code }
    setItems((current) => current.map((item) => item.id === id ? { ...item, code, productDirectoryId: undefined, directorySnapshot: snapshot } : item))
    setResult(null)
  }

  function applyAgentName(id: string, name: string) {
    setItems((current) => current.map((item) => item.id === id ? { ...item, name, supplierName: item.supplierName || item.name, standardName: name } : item))
    setResult(null)
  }

  // Строки, для которых открыт выбор товара или кода: отдельно от галочек, чтобы выбор для одной
  // строки не сбрасывал массовое выделение.
  // Поиск открывается пустым: текущий код строки виден в блоке «Сейчас в строке».
  function openDirectoryPicker(ids: string[]) {
    if (ids.length === 0) return
    setPickerTargets(ids)
    setDirectoryOpen(true)
    directorySearch.restart()
  }

  // Результат сохраняется на сервер автосохранением, как и любая другая правка.
  // Возвращает текст ошибки или null; showError — показать ошибку плашкой (иначе её покажет вызвавший).
  async function calculate(itemsOverride?: GoodsItemInput[], updateInputItems = true, paramsOverride?: GoodsParams, showError = true): Promise<string | null> {
    const itemsToCalculate = itemsOverride ?? items
    if (itemsToCalculate.filter((item) => !item.excluded).length === 0) {
      const message = 'Добавьте хотя бы одну включённую строку'
      if (showError) setError(message)
      return message
    }
    const requestId = calculationRequestRef.current + 1
    calculationRequestRef.current = requestId
    setLoading(true); setActivity('calculate'); setError('')
    if (itemsOverride && updateInputItems) setItems(itemsOverride)
    try {
      const itemsForRequest = itemsToCalculate.map(({ photoUrl: _photoUrl, photoUrls: _photoUrls, ...item }) => ({ ...item, photoUrl: '' }))
      const calculated = await goodsEstimatesApi.calculate({ ...(paramsOverride ?? params), items: itemsForRequest, source: sourceSnapshot ?? undefined })
      if (requestId !== calculationRequestRef.current) return 'расчёт прерван более новым'
      setResult(calculated)
      setReviewCheck(checkFromResult(estimateId, itemsToCalculate, calculated))
      setStep(3)
      return null
    } catch (reason) {
      const message = requestErrorMessage(reason)
      if (requestId === calculationRequestRef.current && showError) setError(message)
      return message
    } finally {
      if (requestId === calculationRequestRef.current) { setLoading(false); setActivity('') }
    }
  }

  // Пробный расчёт для подбора цен под бюджет: без показа результата и без сохранения черновика.
  function previewCalculation(nextItems: GoodsItemInput[], paramsOverride?: GoodsParams) {
    const itemsForRequest = nextItems.map(({ photoUrl: _photoUrl, photoUrls: _photoUrls, ...item }) => ({ ...item, photoUrl: '' }))
    return goodsEstimatesApi.calculate({ ...(paramsOverride ?? params), items: itemsForRequest, source: sourceSnapshot ?? undefined })
  }

  function returnToReview() {
    budget.reset()
    // Цена, изменённая на этапе результата, является временной. При возврате
    // к проверке нужно начать новый расчёт с исходных данных строк (цена из Excel остаётся).
    calculationRequestRef.current += 1
    setLoading(false)
    setActivity('')
    setPreviousBulkResult(null)
    setResult(null)
    setItems((current) => current.map((item) => ({ ...item, pricePerPair: item.sourcePricePerPair ?? '' })))
    setStep(2)
  }

  // ТС по инвойсу или по СУР, вес, снижение и страна СУР: хранятся в параметрах сметы, расчёт
  // повторяется с ценами текущего результата. По СУР цены инвойса на ТС не влияют — подбор под бюджет сбрасывается.
  function changeSir(patch: SirParams) {
    const nextParams = { ...params, ...patch }
    setParams(nextParams)
    if (patch.customsValueMode === 'sir') budget.reset()
    void calculate(withResultPrices(items), false, nextParams)
  }

  // Окно сравнения: та же смета с ценами текущего результата, но с другими параметрами СУР.
  function previewSir(patch: SirParams) {
    return previewCalculation(withResultPrices(items), { ...params, ...patch })
  }

  // Кнопка «Сохранить»: не ждать паузы автосохранения. Уходят только изменения.
  async function saveDraft() {
    if (loading) return
    setLoading(true); setActivity('save'); setError('')
    try {
      await autosave.saveNow()
      setStorageNotice(savedAtText())
    } catch (reason) {
      setError(requestErrorMessage(reason))
    } finally {
      setLoading(false); setActivity('')
    }
  }

  async function exportExcel() {
    setLoading(true); setActivity('export'); setError('')
    try {
      const blob = await goodsEstimatesApi.exportXlsx({ ...params, items: exportItems(items, result), source: sourceSnapshot ?? undefined })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      // Имя файла — по смете и клиенту: в переписке с клиентом видно, к какой смете он относится.
      const base = (exportName ?? '').replace(/[\\/:*?"<>|\u0000-\u001f]+/g, ' ').replace(/\s+/g, ' ').trim().slice(0, 120) || 'Расчёт товаров'
      link.href = url; link.download = `${base} — ${new Date().toISOString().slice(0, 10)}.xlsx`; link.click(); URL.revokeObjectURL(url)
    } catch (reason) {
      setError(requestErrorMessage(reason))
    } finally {
      setLoading(false); setActivity('')
    }
  }

  function clearAll() {
    if (!window.confirm('Очистить параметры и все строки этой сметы? Смета очистится и на сервере.')) return
    setReviewCheck(null); setRemovedRow(null)
    setParams(emptyParams()); usdRate.reset(); setItems([]); setResult(null); setStep(1); setSourceSnapshot(null); setSelectedForDirectory([]); setDirectoryOpen(false); directorySearch.reset(); localStorage.removeItem(paramsStorageKey); localStorage.removeItem(itemsStorageKey); localStorage.removeItem(storageKey(RESULT_KEY, estimateId)); localStorage.removeItem(storageKey(FILE_KEY, estimateId)); localStorage.removeItem(localUpdatedStorageKey)
    void goodsPhotoStore.remove(estimateId)
  }

  const includedCount = items.filter((item) => !item.excluded).length
  const hasSource = excel.headers.length > 0 || items.length > 0
  const resultStep = step === 3 && Boolean(result)
  const calculatorStepper = <CalculatorStepper step={step} setStep={setStep} items={items} result={result} params={params} reviewCheck={reviewCheck} hasSource={hasSource} />
  // Пока смета не загружена с сервера, правки некуда положить: ответ сервера заменил бы строки,
  // добавленные до него (на Render после простоя ответ идёт около минуты). Шапку сметы не
  // показываем: после загрузки её рисует этап, и открытая форма названия закрылась бы.
  if (!photoStorageReady) return <div className="goods-calculator-embedded is-import">
    <div className="goods-page"><p className="muted" role="status">Загружаем смету…</p></div>
  </div>

  // Строку заголовка (название сметы и этапы) рисует сам этап — у каждого рядом свои кнопки.
  return <div className={`goods-calculator-embedded ${resultStep ? 'is-result' : step === 2 ? 'is-review' : 'is-import'}`}>
    <div className="goods-page">
      {error && <div className="banner error goods-request-error" role="alert"><div><strong>Не удалось выполнить действие</strong><p>{error}</p></div><button type="button" onClick={() => setError('')} aria-label="Закрыть сообщение">×</button></div>}

      {step === 1 && <ImportStep
        header={<>{headerSlot}{calculatorStepper}</>}
        loading={loading}
        sheets={excel.sheets}
        sheet={excel.sheet}
        headerRow={excel.headerRow}
        dataStartRow={excel.dataStartRow}
        headers={excel.headers}
        rawRows={excel.rawRows}
        mapping={excel.mapping}
        // После перезагрузки файла в браузере нет — имя берём из сохранённой сметы.
        fileName={excel.selectedFile?.name ?? sourceSnapshot?.filename ?? ''}
        selectedFileAvailable={Boolean(excel.selectedFile)}
        mappingFieldOptions={mappingFieldOptions}
        upload={excel.upload}
        addEmptyRow={() => { addEmptyRow(); setStep(2) }}
        setSheet={excel.setSheet}
        setHeaderRow={excel.setHeaderRow}
        setDataStartRow={excel.setDataStartRow}
        reparseSelectedSheet={excel.reparseSelectedSheet}
        mappedFieldForHeader={mappedFieldForHeader}
        setMappingForHeader={excel.setMappingForHeader}
        setRawRowIncluded={excel.setRawRowIncluded}
        applyMapping={excel.applyMapping}
      />}

      {step === 2 && <ReviewStep
        params={params}
        items={items}
        result={result}
        selectedForDirectory={selectedForDirectory}
        loading={loading}
        storageNotice={storageNotice}
        unsaved={autosave.unsaved}
        header={<>{headerSlot}{calculatorStepper}</>}
        usdRateLoading={usdRate.loading}
        usdRateNotice={usdRate.notice}
        includedCount={includedCount}
        editItems={(next) => { setItems(next); setResult(null) }}
        setSelectedForDirectory={setSelectedForDirectory}
        setParam={setParam}
        loadUsdRate={usdRate.load}
        openDirectoryPicker={openDirectoryPicker}
        removeRow={removeRowWithUndo}
        removedRow={removedRow ? { rowNumber: removedRow.index + 1, name: removedRow.item.name } : null}
        restoreRemovedRow={restoreRemovedRow}
        dismissRemovedRow={() => setRemovedRow(null)}
        reviewCheck={reviewCheck}
        openRowPhotos={(itemId, index) => setRowPhotos({ itemId, index })}
        addManualPhoto={addManualPhoto}
        addEmptyRow={addEmptyRow}
        calculate={calculate}
        agentRowId={agentRowId}
        saveDraft={saveDraft}
        clearAll={clearAll}
        agentStatuses={agentRows.statuses}
        rowDecisions={rowDecisions}
        agentBatch={agentBatch.progress}
        openAgent={setAgentRowId}
        startAgentBatch={() => void agentBatch.start()}
      />}

      {step === 3 && result && <ResultStep agentOpen={Boolean(agentRowId)} onCloseAgent={() => setAgentRowId(null)} result={result} items={items} loading={loading} activity={activity} storageNotice={storageNotice} header={<>{headerSlot}{calculatorStepper}</>} unsaved={autosave.unsaved} saveDraft={saveDraft} exportExcel={exportExcel} onBackToReview={returnToReview} onPriceChange={changeResultPrice} onPriceBlur={recalculateResultPrice} onAntiDumpingChoice={chooseAntiDumping}
        onChangeCode={(index) => { const id = items[index]?.id; if (id) openDirectoryPicker([id]) }} onAgent={(index) => { const id = items[index]?.id; if (id) setAgentRowId(id) }} onToggleIncluded={setResultIncluded} sir={params} onSirChange={changeSir} previewSir={previewSir} itp={params.resultTools?.itp} onItpChange={(value) => setResultTool('itp', value)} onRestoreInitial={() => { budget.reset(); restoreInitialResult() }} onUndoPrices={() => { budget.reset(); undoLastPriceChange() }} canUndoPrices={Boolean(previousBulkResult)} budget={budget} />}

      {directoryOpen && <DirectoryPickerModal
        items={items}
        targetIds={pickerTargets}
        mode={directorySearch.mode}
        query={directorySearch.query}
        entries={directorySearch.entries}
        counts={directorySearch.counts}
        moreCodes={directorySearch.moreCodes}
        loading={directorySearch.loading}
        onClose={() => { setDirectoryOpen(false); setPickerTargets([]) }}
        onModeChange={directorySearch.changeMode}
        onQueryChange={directorySearch.changeQuery}
        onSearchNow={directorySearch.searchNow}
        onShowAll={directorySearch.restart}
        onPick={applyDirectoryEntry}
        onPreview={openPhotoGallery}
      />}
      <p className="goods-disclaimer" title="Это внутренний предварительный расчёт. Пошлина (ЕТТ) и НДС берутся из номенклатуры ТН ВЭД ЕАЭС по коду строки, ИТС — из карточки товара; это не официальное подтверждение.">
        <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"><circle cx="8" cy="8" r="6.3" /><path d="M8 7.3v3.6" /><circle cx="8" cy="5" r=".4" fill="currentColor" /></svg>
        Предварительный расчёт, не официальный документ. Пошлина и НДС — из номенклатуры ТН ВЭД ЕАЭС.
      </p>
      {codeNotice && <CodeNotice notice={codeNotice} loading={loading} onUndo={undoCodeChange} onClose={() => setCodeNotice(null)} />}
    </div>
    {agentRowId && (() => {
      const index = items.findIndex((item) => item.id === agentRowId)
      if (index < 0) return null
      // Ключ — строка: при переходе ↑ ↓ к соседней строке панель начинает с чистого состояния.
      const neighbour = (offset: number) => { const id = items[index + offset]?.id; return id ? () => setAgentRowId(id) : undefined }
      return <AgentPanel key={agentRowId} estimateId={estimateId} item={items[index]} rowNumber={index + 1} rowCount={items.length} onPrev={neighbour(-1)} onNext={neighbour(1)} onClose={() => setAgentRowId(null)}
        onApplyCode={(code) => applyAgentCode(agentRowId, code)} onApplyName={(name) => applyAgentName(agentRowId, name)} onAddPhoto={(event) => addManualPhoto(agentRowId, event)}
        onChanged={() => { agentRows.refresh(); void rowDecisions.refresh() }} />
    })()}
    {rowPhotos && <RowPhotos items={items} itemId={rowPhotos.itemId} startIndex={rowPhotos.index} onClose={closeRowPhotos}
      onRemove={(itemId, index) => setItems((current) => removeRowPhoto(current, itemId, index))}
      onMakeMain={(itemId, index) => setItems((current) => makeMainRowPhoto(current, itemId, index))}
      onMove={moveRowPhotoTo} />}
    {lightbox && <PhotoLightbox photos={lightbox.photos} startIndex={lightbox.index} onClose={() => setLightbox(null)} />}
  </div>
}
