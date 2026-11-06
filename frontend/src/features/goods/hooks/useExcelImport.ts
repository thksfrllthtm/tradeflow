import { ChangeEvent, Dispatch, SetStateAction, useRef, useState } from 'react'
import { GoodsCalculationResult, GoodsItemInput, GoodsParseRow, GoodsSourceSnapshot, SheetInfo } from '@/shared/types/goods'
import { goodsEstimatesApi } from '../goodsEstimatesApi'
import { GoodsMapping } from '../types'
import { columnMappingActions } from '../lib/columnMappingActions'
import { FILE_KEY, storageKey } from '../lib/draftStorage'
import { buildGloveItems, emptyMapping, matchImportedTnvedCodes } from '../lib/importMapping'
import { requestErrorMessage } from '../lib/requestErrorMessage'

type Options = {
  estimateId: number
  items: GoodsItemInput[]
  packageWeightKg: string
  setItems: Dispatch<SetStateAction<GoodsItemInput[]>>
  setResult: Dispatch<SetStateAction<GoodsCalculationResult | null>>
  setStep: (step: 1 | 2 | 3) => void
  setLoading: (loading: boolean) => void
  setError: (message: string) => void
  setStorageNotice: (notice: string) => void
  setSourceSnapshot: Dispatch<SetStateAction<GoodsSourceSnapshot | null>>
  setSelectedForDirectory: Dispatch<SetStateAction<string[]>>
}

// Этап 1: загрузка Excel, выбор листа и строк, сопоставление колонок и сборка строк сметы.
export function useExcelImport({ estimateId, items, packageWeightKg, setItems, setResult, setStep, setLoading, setError, setStorageNotice, setSourceSnapshot, setSelectedForDirectory }: Options) {
  const fileStorageKey = storageKey(FILE_KEY, estimateId)
  const [sheets, setSheets] = useState<SheetInfo[]>([])
  const [sheet, setSheet] = useState('')
  const [headerRow, setHeaderRow] = useState(1)
  const [dataStartRow, setDataStartRow] = useState(2)
  const [headers, setHeaders] = useState<string[]>([])
  const [rawRows, setRawRows] = useState<GoodsParseRow[]>([])
  const [parsedImages, setParsedImages] = useState<{ sourceRow: number; data: string; mimeType: string }[]>([])
  const [mapping, setMapping] = useState<GoodsMapping>(emptyMapping)
  const [fileSignature, setFileSignature] = useState(() => localStorage.getItem(fileStorageKey) ?? '')
  const selectedFileRef = useRef<File | null>(null)
  const { setMappingForHeader, setRawRowIncluded } = columnMappingActions(setMapping, setRawRows)

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
      newItems = buildGloveItems(parsed.rows, parsed.images, nextMapping, packageWeightKg, items.length)
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
      const importedItems = buildGloveItems(rawRows, parsedImages, mapping, packageWeightKg, 0)
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

  // Исходная таблица Excel из сметы на сервере.
  function restore(source: GoodsSourceSnapshot & { mapping: GoodsMapping }) {
    setSheet(source.sheet || '')
    setHeaderRow(source.headerRow || 1)
    setDataStartRow(source.dataStartRow || 2)
    setHeaders(source.headers ?? [])
    setRawRows(source.rows ?? [])
    setMapping(source.mapping)
  }

  return {
    sheets, sheet, setSheet, headerRow, setHeaderRow, dataStartRow, setDataStartRow, headers, rawRows, mapping,
    selectedFile: selectedFileRef.current,
    upload, reparseSelectedSheet, applyMapping, setMappingForHeader, setRawRowIncluded, restore,
  }
}
