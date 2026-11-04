import { ChangeEvent, ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { mappedFieldForHeader, normalizeStoredMapping } from '../lib/excelColumns'
import { mergeStoredPhotos, photosOf } from '@/shared/lib/itemPhotos'
import { storageKey } from '../lib/localDraft'
import { GoodsCalculationInput, GoodsCalculationResult, GoodsEstimateSummary, GoodsItemInput, GoodsParams, GoodsParseRow, GoodsResultTools, GoodsSourceSnapshot, SheetInfo, SirParams } from '@/shared/types/goods'
import { goodsEstimatesApi } from '../goodsEstimatesApi'
import { CalculatorActivity, GoodsMapping } from '../types'
import { draftSnapshot, emptyParams, ensureItemIds, FILE_KEY, goodsPhotoStore, ITEMS_KEY, LOCAL_UPDATED_KEY, normalizeParams, PARAMS_KEY, RESULT_KEY, storedItems, storedParams, storedResult } from '../lib/draftStorage'
import { buildGloveItems, emptyMapping, mappingFieldOptions, matchImportedTnvedCodes } from '../lib/importMapping'
import { lookupNomenclature, withNomenclatureRates } from '../lib/nomenclatureRates'
import { requestErrorMessage } from '../lib/requestErrorMessage'
import { savedAtText } from '@/shared/lib/dates'
import { plural, rowProblems } from '../components/ResultStep/errorGroups'
import { reviewStatus, rowMissing } from '../components/ReviewRow/reviewStatus'
import { useDirectorySearch } from '../hooks/useDirectorySearch'
import { useDraftAutosave } from '../hooks/useDraftAutosave'
import { useServerAutosave } from '../hooks/useServerAutosave'
import '../goods.css'
import './GoodsCalculator.css'
import { ImportStep } from '../components/ImportStep/ImportStep'
import { ReviewStep } from '../components/ReviewStep/ReviewStep'
import { ResultStep } from '../components/ResultStep/ResultStep'
import { DirectoryPickerEntry, DirectoryPickerModal } from '../components/DirectoryPickerModal/DirectoryPickerModal'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { RowPhotos } from '../components/RowPhotos/RowPhotos'
import { makeMainRowPhoto, moveRowPhoto, removeRowPhoto } from '../lib/rowPhotos'
import { columnMappingActions } from '../lib/columnMappingActions'
import { resultPriceActions } from '../lib/resultPriceActions'
import { exportItems } from '../lib/exportItems'
import { useBudgetDistribution } from '../hooks/useBudgetDistribution'
import { rowActions } from '../lib/rowActions'
import { checkFromResult, loadReviewCheck, ReviewCheck, saveReviewCheck } from '../lib/reviewCheck'
import './calculatorPage.css'
import { AgentPanel, agentItemFrom, agentPhotosFrom } from '../agent/AgentPanel'
import { useRowDecisions } from '../hooks/useRowDecisions'
import { agentApi } from '../agent/agentApi'
import { useAgentRows } from '../agent/useAgentRows'

// exportName — название сметы и клиент для имени файла выгрузки.
type Props = { estimateId: number; onEstimateSaved?: (estimate: GoodsEstimateSummary) => void; headerSlot: ReactNode; exportName?: string }

export function GoodsCalculator({ estimateId, onEstimateSaved, headerSlot, exportName }: Props) {
  const paramsStorageKey = storageKey(PARAMS_KEY, estimateId)
  const itemsStorageKey = storageKey(ITEMS_KEY, estimateId)
  const fileStorageKey = storageKey(FILE_KEY, estimateId)
  const localUpdatedStorageKey = storageKey(LOCAL_UPDATED_KEY, estimateId)
  const [params, setParams] = useState<GoodsParams>(() => storedParams(estimateId))
  const [items, setItems] = useState<GoodsItemInput[]>(() => storedItems(estimateId))
  const [step, setStep] = useState<1 | 2 | 3>(() => storedResult(estimateId) ? 3 : storedItems(estimateId).length > 0 ? 2 : 1)
  const [result, setResult] = useState<GoodsCalculationResult | null>(() => storedResult(estimateId))
  const [previousBulkResult, setPreviousBulkResult] = useState<GoodsCalculationResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [activity, setActivity] = useState<CalculatorActivity>('')
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
  const [rawRows, setRawRows] = useState<GoodsParseRow[]>([])
  const [parsedImages, setParsedImages] = useState<{ sourceRow: number; data: string; mimeType: string }[]>([])
  const [mapping, setMapping] = useState<GoodsMapping>(emptyMapping)
  const [sourceSnapshot, setSourceSnapshot] = useState<GoodsSourceSnapshot | null>(null)
  const [fileSignature, setFileSignature] = useState(() => localStorage.getItem(fileStorageKey) ?? '')
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
  // Строка, для которой открыт чат подбора кода ИИ, и пакетный подбор для строк без кода.
  const [agentRowId, setAgentRowId] = useState<string | null>(null)
  const [agentBatch, setAgentBatch] = useState<{ done: number; total: number } | null>(null)
  // Уведомление внизу после смены кода на этапе результата: «Отменить» (previous — строки до смены) или ошибка.
  const [codeNotice, setCodeNotice] = useState<{ text: string; failed?: boolean; previous?: GoodsItemInput[] } | null>(null)

  // Удачное уведомление само скрывается через 8 секунд; ошибка — висит, пока не закроют.
  useEffect(() => {
    if (!codeNotice || codeNotice.failed) return
    const timer = window.setTimeout(() => setCodeNotice(null), 8000)
    return () => window.clearTimeout(timer)
  }, [codeNotice])
  const selectedFileRef = useRef<File | null>(null)
  const autoRateRequestedRef = useRef(false)
  // Дата, на которую курс взят из НБРК; пусто — курс введён вручную или его нет.
  const nbkRateDateRef = useRef('')
  const rateRequestRef = useRef(0)
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
  const directorySearch = useDirectorySearch(setError)
  const agentRows = useAgentRows(estimateId, step === 2)
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
        ? { ...draft.input.source, mapping: normalizeStoredMapping(emptyMapping(), draft.input.source.mapping as Partial<GoodsMapping> | undefined) }
        : null
      markSaved(serverParams, restoredItems, draft.result ?? null)
      autosave.setBaseline({ params: serverParams, items: restoredItems, result: draft.result ?? null, source: serverSource })
      // Исходная таблица Excel хранится только на сервере — берём её в любом случае, иначе
      // автосохранение стёрло бы её на сервере.
      if (serverSource) {
        setSourceSnapshot(serverSource)
        setSheet(serverSource.sheet || '')
        setHeaderRow(serverSource.headerRow || 1)
        setDataStartRow(serverSource.dataStartRow || 2)
        setHeaders(serverSource.headers ?? [])
        setRawRows(serverSource.rows ?? [])
        setMapping(serverSource.mapping)
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
  const { setMappingForHeader, setRawRowIncluded } = columnMappingActions(setMapping, setRawRows)
  const { recalculateResultPrice, restoreInitialResult, undoLastPriceChange, applyDistributedPrices } = resultPriceActions({ items, result, previousBulkResult, setResult, setPreviousBulkResult, calculate })
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
    if (field === 'assessmentDate' || field === 'usdKzt') setUsdRateNotice('')
    if (field === 'usdKzt') nbkRateDateRef.current = ''
    if (field === 'packageWeightKg') {
      // Общая упаковка меняет только строки, которые идут по общему значению.
      // Строка со своей упаковкой (значение отличается от общего) остаётся как есть.
      // Нетто из Excel или введённое вручную сервер не пересчитывает в любом случае.
      const previous = params.packageWeightKg
      setItems((current) => current.map((item) => item.packageWeightKg === previous ? { ...item, packageWeightKg: value } : item))
    }
    setResult(null)
  }

  // quiet — автоматическая загрузка: ошибку не показываем, курс можно взять кнопкой «НБРК» или ввести.
  async function loadUsdRate(quiet = false) {
    const date = params.assessmentDate
    if (!date) {
      if (!quiet) setError('Сначала укажите дату расчёта')
      return
    }
    // Ответ на устаревший запрос (дату успели сменить) не применяется.
    const request = ++rateRequestRef.current
    setUsdRateLoading(true)
    if (!quiet) setError('')
    try {
      const rate = await goodsEstimatesApi.usdRate(date)
      if (request !== rateRequestRef.current) return
      setParam('usdKzt', rate.rate)
      nbkRateDateRef.current = date
      setUsdRateNotice(`${rate.source} · ${rate.date}`)
    } catch (reason) {
      if (request !== rateRequestRef.current) return
      if (!quiet) setError(requestErrorMessage(reason))
      setUsdRateNotice('')
    } finally {
      if (request === rateRequestRef.current) setUsdRateLoading(false)
    }
  }

  // Курс НБРК подставляется сам: сразу при открытии сметы без курса и после смены даты, если
  // прежний курс тоже был из НБРК. Введённый вручную курс не меняется.
  useEffect(() => {
    if (!photoStorageReady || !params.assessmentDate) return
    const missing = params.usdKzt.trim() === '' && !autoRateRequestedRef.current
    const outdated = nbkRateDateRef.current !== '' && nbkRateDateRef.current !== params.assessmentDate
    if (!missing && !outdated) return
    const timer = window.setTimeout(() => {
      autoRateRequestedRef.current = true
      void loadUsdRate(true)
    }, 400)
    return () => window.clearTimeout(timer)
  }, [photoStorageReady, params.assessmentDate, params.usdKzt])

  async function parseFile(file: File, replaceExisting: boolean, requestedSheet = '') {
    const signature = `${file.name}:${file.size}:${file.lastModified}`
    setLoading(true); setError(''); setResult(null); setStep(1)
    try {
      const parsed = await goodsEstimatesApi.parseXlsx(file, { sheet: requestedSheet || undefined, headerRow, dataStartRow })
      setSheets(parsed.sheets)
      const selectedSheet = sheet || parsed.sheets[0]?.name || ''
      setSheet(selectedSheet)
      let newItems: GoodsItemInput[]
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
    const picked = event.target.files?.[0]
    event.target.value = ''
    if (!picked) return
    // Копия в памяти: при смене листа файл отправляется снова, а файл на диске к этому времени
    // могут открыть в Excel или изменить — тогда браузер обрывает отправку («Failed to fetch»).
    let file: File
    try {
      file = new File([await picked.arrayBuffer()], picked.name, { type: picked.type, lastModified: picked.lastModified })
    } catch {
      setError('Не удалось прочитать файл. Закройте его в Excel и выберите снова.')
      return
    }
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
      if (!matched.nomenclatureLoaded) {
        setStorageNotice('Коды ТН ВЭД из файла не проверены: номенклатура ТН ВЭД ЕАЭС не загружена — ставки для расчёта брать неоткуда.')
      } else if (matched.matched > 0 || matched.missing > 0) {
        setStorageNotice(`Коды ТН ВЭД сверены с номенклатурой: найдено ${matched.matched}${matched.missing > 0 ? `, нет в номенклатуре ${matched.missing}` : ''}. Ставки — пошлина ЕТТ и НДС из номенклатуры.`)
      }
      setStep(2)
    } catch (reason) {
      setError(requestErrorMessage(reason))
    } finally {
      setLoading(false)
    }
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

  // «Учитывать строку в расчёте» в панели строки результата; смета пересчитывается с текущими ценами.
  function setResultIncluded(index: number, included: boolean) {
    const nextItems = items.map((item, itemIndex) => itemIndex === index ? { ...item, excluded: !included } : item)
    setItems(nextItems)
    void calculate(withResultPrices(nextItems), false)
  }

  // Строки с ценами текущего результата (правка, подбор под бюджет) — для пересчёта без потери этих цен.
  function withResultPrices(list: GoodsItemInput[]) {
    return list.map((item, itemIndex) => ({ ...item, pricePerPair: result?.items[itemIndex]?.pricePerPair || item.pricePerPair }))
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

  // Подбор ИИ для всех включённых строк без кода, где подбора ещё не было: запуски идут на
  // сервере в фоне, значки в таблице обновляются сами.
  async function startAgentBatch() {
    // Строки, где уже был поиск в базе, но агента не запускали, тоже входят: агент запускается сразу.
    const rows = items.filter((item) => item.id && !item.excluded && !item.code && (!agentRows.statuses[item.id] || agentRows.statuses[item.id].status === 'ready'))
    if (rows.length === 0) return
    if (!window.confirm(`Подобрать коды для ${rows.length} строк без кода? Подбор идёт на сервере, результат — в чате каждой строки (кнопка «Подбор»).`)) return
    setAgentBatch({ done: 0, total: rows.length })
    setError('')
    try {
      for (const [index, item] of rows.entries()) {
        const ready = agentRows.statuses[item.id ?? '']
        if (ready?.status === 'ready') await agentApi.begin(ready.sessionId)
        else await agentApi.start({ estimateId, itemId: item.id ?? '', item: agentItemFrom(item), photos: await agentPhotosFrom(item), auto: true })
        setAgentBatch({ done: index + 1, total: rows.length })
        agentRows.refresh()
      }
    } catch (reason) {
      setError(requestErrorMessage(reason))
    } finally {
      setAgentBatch(null)
      agentRows.refresh()
    }
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

  // Вариант антидемпинга строки: сохраняется в строке сметы, расчёт повторяется с ценами текущего результата.
  function chooseAntiDumping(index: number, key: string) {
    const choice = key || undefined
    setItems((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, antiDumpingChoice: choice } : item))
    void calculate(withResultPrices(items.map((item, itemIndex) => itemIndex === index ? { ...item, antiDumpingChoice: choice } : item)), false)
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

  function changeResultPrice(index: number, value: string) {
    // Keep the edited value in the source rows as well as in the result. The
    // next row recalculation is built from `items`; without this mirror the
    // previous row still contains its old price and appears to disappear.
    setItems((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item))
    setResult((current) => current ? { ...current, items: current.items.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item) } : current)
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
    setParams(emptyParams()); autoRateRequestedRef.current = false; nbkRateDateRef.current = ''; setItems([]); setResult(null); setStep(1); setSourceSnapshot(null); setSelectedForDirectory([]); setDirectoryOpen(false); directorySearch.reset(); localStorage.removeItem(paramsStorageKey); localStorage.removeItem(itemsStorageKey); localStorage.removeItem(storageKey(RESULT_KEY, estimateId)); localStorage.removeItem(fileStorageKey); localStorage.removeItem(localUpdatedStorageKey)
    void goodsPhotoStore.remove(estimateId)
  }

  const includedCount = items.filter((item) => !item.excluded).length
  const hasSource = headers.length > 0 || items.length > 0
  const pickerRows = items.filter((item) => pickerTargets.includes(item.id ?? ''))
  // № — как в таблице; строка Excel — в скобках, если строка из файла.
  const pickerRowNumber = pickerRows.length === 1 ? items.indexOf(pickerRows[0]) + 1 : 0
  const pickerTitle = pickerRows.length === 1 ? `Код для строки ${pickerRowNumber}` : pickerRows.length > 1 ? `Код для ${pickerRows.length} выбранных строк` : 'Выбор кода ТН ВЭД'
  const pickerSubtitle = pickerRows.length === 1 ? pickerRows[0].name || pickerRows[0].supplierName || '' : ''
  const pickerTarget = pickerRows.length === 1 ? `К строке ${pickerRowNumber}` : `К ${pickerRows.length} выбранным строкам`
  // Общий код выбранных строк; у строк разные коды — блока «Сейчас в строке» нет.
  const pickerCodes = [...new Set(pickerRows.map((row) => row.code ?? ''))]
  const pickerCurrentCode = pickerCodes.length === 1 ? pickerCodes[0] : ''
  const pickerPhotos = pickerRows.flatMap((row) => photosOf(row).map((photo) => ({ photo, row })))
  // На этапе результата степпер компактный: «✓ Файл · ✓ Проверка · Результат».
  const resultStep = step === 3 && Boolean(result)
  // На этапе проверки строк — красный счётчик строк, требующих внимания (те же проверки, что в таблице).
  const reviewStep = step === 2
  const reviewAttention = reviewStep ? items.filter((item, index) => reviewStatus(item, result?.items[index], result ? undefined : reviewCheck?.rows[item.id ?? ''], rowMissing(item, params.packageWeightKg)).attention).length : 0
  const codeProblemRows = result ? result.items.filter((item, index) => !item.excluded && rowProblems(item, item.code || items[index]?.code || '').some((problem) => problem.kind === 'code')).length : 0
  const stepList: { value: 1 | 2 | 3; label: string; available: boolean }[] = [
    { value: 1, label: 'Файл', available: true },
    { value: 2, label: 'Проверка строк', available: hasSource },
    { value: 3, label: 'Результат', available: Boolean(result) },
  ]
  const calculatorStepper = <nav className="est-steps compact" aria-label="Этапы расчёта товаров">
    {stepList.map(({ value, label, available }) => {
      // «Проверка» завершена, только если у всех строк есть код и ставки; иначе — ⚠ и число таких строк.
      const unfinished = value === 2 && step === 3 && codeProblemRows > 0
      const done = value < step && available && !unfinished
      const title = unfinished ? `${label}: ${codeProblemRows} ${plural(codeProblemRows, ['строка', 'строки', 'строк'])} без кода ТН ВЭД или ставок` : label
      return <button type="button" key={value} className={step === value ? 'active' : done ? 'done' : unfinished ? 'unfinished' : undefined} aria-current={step === value ? 'step' : undefined} onClick={() => available && setStep(value)} disabled={!available} title={title}>
        <span className="est-step-num">{unfinished ? '⚠' : done ? '✓' : value}</span><span className="est-step-label">{label}</span>
        {unfinished && <b className="est-step-count" aria-label={title}>{codeProblemRows}</b>}
        {value === 2 && reviewStep && reviewAttention > 0 && <b className="est-step-count bad" title={`Требуют внимания: ${reviewAttention}`}>{reviewAttention}</b>}
      </button>
    })}
  </nav>
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
        sheets={sheets}
        sheet={sheet}
        headerRow={headerRow}
        dataStartRow={dataStartRow}
        headers={headers}
        rawRows={rawRows}
        mapping={mapping}
        // После перезагрузки файла в браузере нет — имя берём из сохранённой сметы.
        fileName={selectedFileRef.current?.name ?? sourceSnapshot?.filename ?? ''}
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
        loading={loading}
        storageNotice={storageNotice}
        unsaved={autosave.unsaved}
        header={<>{headerSlot}{calculatorStepper}</>}
         usdRateLoading={usdRateLoading}
         usdRateNotice={usdRateNotice}
        includedCount={includedCount}
        editItems={(next) => { setItems(next); setResult(null) }}
        setSelectedForDirectory={setSelectedForDirectory}
         setParam={setParam}
         loadUsdRate={loadUsdRate}
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
        saveDraft={saveDraft}
        clearAll={clearAll}
        agentStatuses={agentRows.statuses}
        rowDecisions={rowDecisions}
        agentBatch={agentBatch}
        openAgent={setAgentRowId}
        startAgentBatch={() => void startAgentBatch()}
      />}

      {step === 3 && result && <ResultStep result={result} items={items} loading={loading} activity={activity} storageNotice={storageNotice} header={<>{headerSlot}{calculatorStepper}</>} unsaved={autosave.unsaved} saveDraft={saveDraft} exportExcel={exportExcel} onBackToReview={returnToReview} onPriceChange={changeResultPrice} onPriceBlur={recalculateResultPrice} onAntiDumpingChoice={chooseAntiDumping}
        onChangeCode={(index) => { const id = items[index]?.id; if (id) openDirectoryPicker([id]) }} onAgent={(index) => { const id = items[index]?.id; if (id) setAgentRowId(id) }} onToggleIncluded={setResultIncluded} sir={params} onSirChange={changeSir} previewSir={previewSir} itp={params.resultTools?.itp} onItpChange={(value) => setResultTool('itp', value)} onRestoreInitial={() => { budget.reset(); restoreInitialResult() }} onUndoPrices={() => { budget.reset(); undoLastPriceChange() }} canUndoPrices={Boolean(previousBulkResult)} budget={budget} />}

      {directoryOpen && <DirectoryPickerModal
        title={pickerTitle}
        subtitle={pickerSubtitle}
        target={pickerTarget}
        currentCode={pickerCurrentCode}
        photos={pickerPhotos}
        multipleRows={pickerRows.length > 1}
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
      {codeNotice && <div className={codeNotice.failed ? 'code-notice failed' : 'code-notice'} role={codeNotice.failed ? 'alert' : 'status'}>
        <span>{codeNotice.text}</span>
        {codeNotice.previous && <button type="button" className="code-notice-undo" onClick={() => codeNotice.previous && undoCodeChange(codeNotice.previous)} disabled={loading}>Отменить</button>}
        <button type="button" className="code-notice-close" onClick={() => setCodeNotice(null)} aria-label="Закрыть уведомление">×</button>
      </div>}
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
