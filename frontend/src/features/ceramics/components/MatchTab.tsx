import { ChangeEvent, useMemo, useRef, useState } from 'react'
import { goodsApi } from '@/shared/api/goodsApi'
import { GoodsParseResult } from '@/shared/types/goods'
import { errorMessage } from '@/shared/lib/errorMessage'
import { ceramicsApi } from '../ceramicsApi'
import { AIStatus, CeramicsType, Resolution, ResolutionStatus } from '../types'
import { buildGroups, chunk, Draft, draftFromResolution, findHeaderRow, guessMapping, MatchGroup, MatchMapping, PHOTO_FROM_IMAGES, resultCsv } from '../lib/matchGroups'
import { AI_PHOTO_SIDE, resizePhoto } from '../lib/photos'
import { ResultsTable, statusTone } from './ResultsTable'
import { SourceSettings } from '@/features/goods/components/ImportStep/ImportStep'
import { ExcelPreview } from '@/features/goods/components/ImportStep/ExcelPreview'

// Что можно назначить колонке файла — щелчком по её заголовку.
type ColumnRole = 'mark' | 'chineseName' | 'russianName'
const columnFields: { value: ColumnRole; label: string }[] = [
  { value: 'mark', label: 'Марка' },
  { value: 'chineseName', label: 'Китайское наименование' },
  { value: 'russianName', label: 'Русское (подсказка)' },
]

// Сколько неизвестных марок уходит на сервер за один запрос (там — до двух обращений к ИИ
// на марку). Небольшие порции: прогресс виден, отмена срабатывает быстро.
const ANALYZE_BATCH = 4
const LOOKUP_BATCH = 1000

type Filter = 'all' | ResolutionStatus
const filters: { value: Filter; label: string }[] = [
  { value: 'all', label: 'Все' },
  { value: 'NEED_REVIEW', label: 'Проверить' },
  { value: 'NOT_DETERMINED', label: 'Не найдено' },
  { value: 'AUTO', label: 'Авто' },
  { value: 'FOUND_BY_MARK', label: 'По марке' },
]

type Props = { types: CeramicsType[]; ai: AIStatus | null }

export function MatchTab({ types, ai }: Props) {
  const [file, setFile] = useState<File | null>(null)
  const [parsed, setParsed] = useState<GoodsParseResult | null>(null)
  const [sheet, setSheet] = useState('')
  const [headerRow, setHeaderRow] = useState(1)
  const [mapping, setMapping] = useState<MatchMapping>({ mark: '', chineseName: '', russianName: '', photo: '' })
  const [groups, setGroups] = useState<MatchGroup[]>([])
  const [loading, setLoading] = useState(false)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  // Настройки колонок и строки файла открыты до подбора; после — сворачиваются в одну строку.
  const [setupOpen, setSetupOpen] = useState(true)
  const runId = useRef(0)

  // findHeader: при первом открытии файла сама ищет строку заголовков, если в строке 1 их нет.
  async function parse(nextFile: File, nextSheet: string, nextHeaderRow: number, findHeader = false) {
    setLoading(true); setError('')
    try {
      const read = (headerRow: number) => goodsApi.parseXlsx(nextFile, { sheet: nextSheet || undefined, headerRow, dataStartRow: headerRow + 1, shareImages: true })
      let result = await read(nextHeaderRow)
      const foundHeader = findHeader && !guessMapping(result.headers, false).mark ? findHeaderRow(result) : null
      if (foundHeader) {
        result = await read(foundHeader)
        setHeaderRow(foundHeader)
      }
      setParsed(result)
      setSheet(nextSheet || result.sheets[0]?.name || '')
      setMapping(guessMapping(result.headers, result.images.length > 0, result.rows))
      setGroups([])
      setSetupOpen(true)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setLoading(false)
    }
  }

  function upload(event: ChangeEvent<HTMLInputElement>) {
    const selected = event.target.files?.[0]
    event.target.value = ''
    if (!selected) return
    runId.current++
    setRunning(false)
    setFile(selected)
    setSheet('')
    setHeaderRow(1)
    void parse(selected, '', 1, true)
  }

  const updateGroup = (key: string, change: (group: MatchGroup) => MatchGroup) =>
    setGroups((current) => current.map((group) => group.key === key ? change(group) : group))

  function applyResolutions(resolutions: Resolution[]) {
    const byKey = new Map(resolutions.map((resolution) => [resolution.key, resolution]))
    setGroups((current) => current.map((group) => {
      const resolution = byKey.get(group.key)
      return resolution ? { ...group, state: 'done', error: undefined, resolution, draft: draftFromResolution(resolution, group.chineseName), draftPrice: undefined } : group
    }))
  }

  // Сначала все марки ищутся в базе одним-двумя запросами; ИИ — только для неизвестных,
  // каждая марка один раз, небольшими порциями.
  async function run(onlyKeys?: string[]) {
    if (!parsed) return
    const id = ++runId.current
    const all = onlyKeys ? groups.filter((group) => onlyKeys.includes(group.key)) : buildGroups(parsed, mapping)
    if (!onlyKeys) {
      if (all.length === 0) { setError('В выбранных колонках нет ни марок, ни китайских наименований'); return }
      setGroups(all)
      setFilter('all')
      setSetupOpen(false)
    }
    setRunning(true); setError('')
    try {
      const found = new Set<string>()
      for (const part of chunk(all.filter((group) => group.markKey), LOOKUP_BATCH)) {
        const resolutions = await ceramicsApi.lookup(part.map((group) => group.mark))
        if (id !== runId.current) return
        const byMark = new Map(resolutions.map((resolution) => [resolution.markKey, resolution]))
        const matched = part.flatMap((group) => {
          const resolution = byMark.get(group.markKey)
          return resolution ? [{ ...resolution, key: group.key }] : []
        })
        matched.forEach((resolution) => found.add(resolution.key))
        applyResolutions(matched)
      }
      const unknown = all.filter((group) => !found.has(group.key))
      for (const batch of chunk(unknown, ANALYZE_BATCH)) {
        if (id !== runId.current) return
        const keys = new Set(batch.map((group) => group.key))
        setGroups((current) => current.map((group) => keys.has(group.key) ? { ...group, state: 'running' } : group))
        const items = await Promise.all(batch.map(async (group) => ({
          key: group.key, mark: group.mark, chineseName: group.chineseName, russianName: group.russianName,
          photo: group.photo && ai?.vision ? await resizePhoto(group.photo, AI_PHOTO_SIDE) : '',
        })))
        try {
          const resolutions = await ceramicsApi.analyze(items)
          if (id !== runId.current) return
          applyResolutions(resolutions)
        } catch (reason) {
          if (id !== runId.current) return
          const message = errorMessage(reason)
          setGroups((current) => current.map((group) => keys.has(group.key) ? { ...group, state: 'error', error: message } : group))
        }
      }
    } catch (reason) {
      if (id === runId.current) setError(errorMessage(reason))
    } finally {
      if (id === runId.current) setRunning(false)
    }
  }

  function stop() {
    runId.current++
    setRunning(false)
    setGroups((current) => current.map((group) => group.state === 'running' ? { ...group, state: 'pending' } : group))
  }

  function setDraft(key: string, change: Partial<Draft>) {
    updateGroup(key, (group) => ({ ...group, draft: { ...group.draft, ...change } }))
  }



  function downloadCsv() {
    const blob = new Blob([resultCsv(groups)], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `${(file?.name ?? 'керамика').replace(/\.xlsx$/i, '')} — подбор.csv`
    link.click()
    URL.revokeObjectURL(url)
  }

  const counts = useMemo(() => {
    const result: Record<string, number> = { all: groups.length }
    for (const group of groups) if (group.resolution) result[group.resolution.status] = (result[group.resolution.status] ?? 0) + 1
    return result
  }, [groups])
  const processed = groups.filter((group) => group.state === 'done' || group.state === 'error').length
  const visible = filter === 'all' ? groups : groups.filter((group) => group.resolution?.status === filter)
  const rowsIncluded = parsed?.rows.filter((row) => row.included).length ?? 0
  const photoCount = parsed ? new Set(parsed.images.map((image) => image.sourceRow)).size : 0
  const fileInput = <input type="file" accept=".xlsx" onChange={upload} disabled={loading || running} />

  return <div className="cer-match">
    {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
    {ai && !ai.configured && <p className="cer-note warn">ИИ не настроен на сервере: неизвестные марки определяются только по совпадениям с базой.</p>}

    {!parsed
      ? <label className={loading ? 'cer-drop busy' : 'cer-drop'}>
        {fileInput}
        <span className="cer-drop-icon" aria-hidden="true">XLSX</span>
        <strong>{loading ? 'Читаем файл…' : 'Загрузите Excel клиента'}</strong>
        <span>Марка, китайское наименование и фото — колонки определятся сами</span>
      </label>
      // Как этап 1 сметы: панель «откуда читаем» и лист «как в Excel»; колонку назначают щелчком по её заголовку.
      : <div className="rv-step cer-import">
        <div className="rv-toolbar im-toolbar">
          <SourceSettings loading={loading || running} fileName={file?.name ?? ''} fileInput={fileInput} sheets={parsed.sheets} sheet={sheet} headerRow={headerRow}
            selectedFileAvailable={!!file} setSheet={setSheet} setHeaderRow={setHeaderRow} reparse={() => { if (file) void parse(file, sheet, headerRow) }} />
          <div className="rv-title-actions">
            <span className="rv-save-state im-count">К подбору <b>{rowsIncluded}</b> из {parsed.rows.length} строк</span>
            {!mapping.mark && <span className="rv-save-state unsaved" title="Нажмите на заголовок колонки с марками и выберите «Марка»">Выберите колонку «Марка»</span>}
            {parsed.images.length > 0 && <label className="cer-check" title="Фото из Excel помогают ИИ определить тип товара">
              <input type="checkbox" checked={mapping.photo === PHOTO_FROM_IMAGES} disabled={running}
                onChange={(event) => setMapping({ ...mapping, photo: event.target.checked ? PHOTO_FROM_IMAGES : '' })} />Фото ({photoCount})
            </label>}
            {groups.length > 0 && <button type="button" className="secondary" onClick={() => setSetupOpen(!setupOpen)} aria-expanded={setupOpen}>{setupOpen ? 'Скрыть строки' : 'Строки файла'}</button>}
            <label className="secondary cer-file-button">{loading ? 'Читаем…' : 'Заменить файл'}{fileInput}</label>
            {running
              ? <button type="button" className="secondary" onClick={stop}>Остановить</button>
              : <button type="button" onClick={() => void run()} disabled={loading || rowsIncluded === 0 || (!mapping.mark && !mapping.chineseName)}>{groups.length ? 'Подобрать заново' : 'Подобрать'} <span aria-hidden="true">→</span></button>}
          </div>
        </div>
        {running && <div className="cer-progress" role="status">
          <span className="cer-progress-bar"><span style={{ transform: `scaleX(${groups.length ? processed / groups.length : 0})` }} /></span>
          <span>{processed} из {groups.length}</span>
        </div>}
        {setupOpen && <section className="rv-card" aria-label="Строки файла">
          <ExcelPreview headers={parsed.headers} rows={parsed.rows} fields={columnFields}
            fieldForHeader={(header) => columnFields.find((field) => mapping[field.value] === header)?.value ?? ''}
            onFieldChange={(header, field) => {
              const next = { ...mapping }
              for (const item of columnFields) if (next[item.value] === header) next[item.value] = ''
              if (field) next[field as ColumnRole] = header
              setMapping(next)
            }}
            onIncludedChange={(sourceRow, included) => setParsed((current) => current && { ...current, rows: current.rows.map((row) => row.sourceRow === sourceRow ? { ...row, included } : row) })} />
        </section>}
      </div>}

    {groups.length > 0 && <>
      <section className="cer-sheet-card" aria-label="Результат подбора">
      <div className="cer-filterbar">
        <div className="dir-seg cer-filters" role="tablist" aria-label="Статус">
          {filters.filter((item) => item.value === 'all' || counts[item.value]).map((item) => <button key={item.value} type="button" role="tab" aria-selected={filter === item.value}
            className={filter === item.value ? 'active' : undefined} onClick={() => setFilter(item.value)}>
            {item.value !== 'all' && <span className={`status-dot ${statusTone[item.value]}`} aria-hidden="true" />}{item.label}<span className="cer-seg-count">{counts[item.value] ?? 0}</span>
          </button>)}
        </div>
        <span className="cer-toolbar-spacer" />
        <button type="button" className="secondary" onClick={downloadCsv} title="Таблица по строкам файла">Скачать CSV</button>
      </div>
      <ResultsTable groups={visible} types={types} running={running}
        onDraft={setDraft}
        onDraftPrice={(key, draftPrice) => updateGroup(key, (group) => ({ ...group, draftPrice }))}
        onCheck={(key) => updateGroup(key, (group) => ({ ...group, checked: true }))}
        onRetry={(key) => void run([key])} />
      </section>
      <p className="dir-foot">Нажмите на строку, чтобы проверить и поправить тип, размер, наименование и цену. В базу здесь ничего не сохраняется: результат — в «Скачать CSV».</p>
    </>}
  </div>
}
