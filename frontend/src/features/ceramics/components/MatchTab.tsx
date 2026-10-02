import { ChangeEvent, useMemo, useRef, useState } from 'react'
import { goodsApi } from '@/shared/api/goodsApi'
import { GoodsParseResult } from '@/shared/types/goods'
import { errorMessage } from '@/shared/lib/errorMessage'
import { ceramicsApi } from '../ceramicsApi'
import { AIStatus, CeramicsType, Resolution, ResolutionStatus } from '../types'
import { buildGroups, chunk, Draft, draftFromResolution, findHeaderRow, guessMapping, MatchGroup, MatchMapping, PHOTO_FROM_IMAGES, resultCsv, savedResolution } from '../lib/matchGroups'
import { AI_PHOTO_SIDE, resizePhoto } from '../lib/photos'
import { ResultsTable } from './ResultsTable'
import { FilePreview } from './FilePreview'

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

type Props = { types: CeramicsType[]; ai: AIStatus | null; onBaseChanged: () => void }

export function MatchTab({ types, ai, onBaseChanged }: Props) {
  const [file, setFile] = useState<File | null>(null)
  const [parsed, setParsed] = useState<GoodsParseResult | null>(null)
  const [sheet, setSheet] = useState('')
  const [headerRow, setHeaderRow] = useState(1)
  const [mapping, setMapping] = useState<MatchMapping>({ mark: '', chineseName: '', russianName: '', photo: '' })
  const [groups, setGroups] = useState<MatchGroup[]>([])
  const [loading, setLoading] = useState(false)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  // Настройки колонок и строки файла открыты до подбора; после — сворачиваются в одну строку.
  const [setupOpen, setSetupOpen] = useState(true)
  const runId = useRef(0)

  // findHeader: при первом открытии файла сама ищет строку заголовков, если в строке 1 их нет.
  async function parse(nextFile: File, nextSheet: string, nextHeaderRow: number, findHeader = false) {
    setLoading(true); setError(''); setNotice('')
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
    setRunning(true); setError(''); setNotice('')
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

  async function save(group: MatchGroup) {
    setError(''); setNotice('')
    // В базу — только наименование, марка и цена.
    const input = { mark: group.resolution?.product?.mark ?? group.mark, name: group.draft.name, price: group.draft.price }
    const existing = group.resolution?.product
    const product = existing ? await ceramicsApi.updateProduct(existing.id, input) : await ceramicsApi.createProduct(input)
    updateGroup(group.key, (current) => {
      const resolution = savedResolution(current, product)
      return { ...current, state: 'done', saved: true, resolution, draft: draftFromResolution(resolution, current.chineseName), draftPrice: undefined }
    })
    onBaseChanged()
    return product
  }

  async function saveAllAuto() {
    const ready = groups.filter((group) => group.resolution?.status === 'AUTO' && group.markKey)
    if (ready.length === 0 || !window.confirm(`Сохранить в базу ${ready.length} марок, определённых автоматически? Проверьте их перед сохранением.`)) return
    let saved = 0
    const failed: string[] = []
    for (const group of ready) {
      try { await save(group); saved++ } catch (reason) { failed.push(`${group.mark}: ${errorMessage(reason)}`) }
    }
    setNotice(`Сохранено марок: ${saved}`)
    if (failed.length) setError(`Не сохранены: ${failed.slice(0, 5).join('; ')}${failed.length > 5 ? '…' : ''}`)
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
  const autoCount = groups.filter((group) => group.resolution?.status === 'AUTO' && group.markKey).length
  const rowsIncluded = parsed?.rows.filter((row) => row.included).length ?? 0
  const photoCount = parsed ? new Set(parsed.images.map((image) => image.sourceRow)).size : 0
  const fileInput = <input type="file" accept=".xlsx" onChange={upload} disabled={loading || running} />
  const columns = [
    mapping.mark && `марка — ${mapping.mark}`,
    mapping.chineseName && `китайское — ${mapping.chineseName}`,
    mapping.russianName && `русское — ${mapping.russianName}`,
    mapping.photo && 'фото из Excel',
  ].filter(Boolean).join(' · ')

  return <div className="cer-match">
    {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
    {notice && <p className="banner ok" role="status">{notice}<button type="button" onClick={() => setNotice('')}>×</button></p>}
    {ai && !ai.configured && <p className="cer-note warn">ИИ не настроен на сервере: неизвестные марки определяются только по совпадениям с базой.</p>}

    {!parsed
      ? <label className={loading ? 'cer-drop busy' : 'cer-drop'}>
        {fileInput}
        <span className="cer-drop-icon" aria-hidden="true">XLSX</span>
        <strong>{loading ? 'Читаем файл…' : 'Загрузите Excel клиента'}</strong>
        <span>Марка, китайское наименование и фото — колонки определятся сами</span>
      </label>
      : <section className="cer-file">
        <div className="cer-file-bar">
          <span className="cer-file-icon" aria-hidden="true">XLS</span>
          <div className="cer-file-info">
            <strong title={file?.name}>{file?.name}</strong>
            <span>
              {rowsIncluded === parsed.rows.length ? `${rowsIncluded} строк` : `${rowsIncluded} из ${parsed.rows.length} строк`}
              {photoCount > 0 && ` · ${photoCount} фото`}
              {headerRow > 1 && <span className="cer-hint"> · заголовки в строке {headerRow}</span>}
              {' · '}{columns || <span className="dir-warn">колонки не выбраны</span>}
              {' · '}<button type="button" className="cer-link" onClick={() => setSetupOpen(!setupOpen)} aria-expanded={setupOpen}>{setupOpen ? 'скрыть настройки' : 'настроить'}</button>
            </span>
          </div>
          <label className="cer-file-button">{loading ? 'Читаем…' : 'Другой файл'}{fileInput}</label>
          {running
            ? <button type="button" className="cer-secondary" onClick={stop}>Остановить</button>
            : <button type="button" onClick={() => void run()} disabled={loading || (!mapping.mark && !mapping.chineseName)}>{groups.length ? 'Подобрать заново' : 'Подобрать'}</button>}
        </div>
        {running && <div className="cer-progress" role="status">
          <span className="cer-progress-bar"><span style={{ width: `${groups.length ? Math.round(processed / groups.length * 100) : 0}%` }} /></span>
          <span>{processed} из {groups.length}</span>
        </div>}
        {setupOpen && <div className="cer-setup">
          <div className="cer-setup-fields">
            <label className="cer-field">Лист
              <select value={sheet} disabled={loading || running} onChange={(event) => { setSheet(event.target.value); if (file) void parse(file, event.target.value, headerRow) }}>
                {parsed.sheets.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
              </select>
            </label>
            <label className="cer-field narrow" title="Номер строки Excel с названиями колонок">Заголовки
              <input type="number" min={1} value={headerRow} disabled={loading || running} onChange={(event) => setHeaderRow(Math.max(1, Number(event.target.value) || 1))}
                onBlur={() => { if (file) void parse(file, sheet, headerRow) }} />
            </label>
            <MappingSelect label="Марка" required value={mapping.mark} headers={parsed.headers} onChange={(mark) => setMapping({ ...mapping, mark })} />
            <MappingSelect label="Китайское наименование" value={mapping.chineseName} headers={parsed.headers} onChange={(chineseName) => setMapping({ ...mapping, chineseName })} />
            <MappingSelect label="Русское (подсказка)" value={mapping.russianName} headers={parsed.headers} onChange={(russianName) => setMapping({ ...mapping, russianName })}
              hint="Перевод клиента не считается правильным — только подсказка" />
            <label className="cer-field">Фото
              <select value={mapping.photo} onChange={(event) => setMapping({ ...mapping, photo: event.target.value })}>
                <option value="">Не использовать</option>
                <option value={PHOTO_FROM_IMAGES} disabled={parsed.images.length === 0}>Из Excel{parsed.images.length === 0 ? ' (нет)' : ''}</option>
              </select>
            </label>
          </div>
          <FilePreview parsed={parsed} mapping={mapping} disabled={loading || running}
            onMapping={setMapping}
            onRowsIncluded={(sourceRows, included) => {
              const selected = new Set(sourceRows)
              setParsed((current) => current && { ...current, rows: current.rows.map((row) => selected.has(row.sourceRow) ? { ...row, included } : row) })
            }}
            onHeaderRow={(sourceRow) => { setHeaderRow(sourceRow); if (file) void parse(file, sheet, sourceRow) }} />
        </div>}
      </section>}

    {groups.length > 0 && <>
      <div className="cer-results-bar">
        <div className="cer-filters" role="tablist" aria-label="Статус">
          {filters.filter((item) => item.value === 'all' || counts[item.value]).map((item) => <button key={item.value} type="button" role="tab" aria-selected={filter === item.value}
            className={filter === item.value ? 'active' : undefined} onClick={() => setFilter(item.value)}>
            {item.value !== 'all' && <span className={`cer-dot ${item.value}`} aria-hidden="true" />}{item.label}<span className="cer-seg-count">{counts[item.value] ?? 0}</span>
          </button>)}
        </div>
        <span className="cer-toolbar-spacer" />
        {autoCount > 0 && !running && <button type="button" className="cer-secondary" onClick={() => void saveAllAuto()}>Сохранить все авто ({autoCount})</button>}
        <button type="button" className="cer-secondary" onClick={downloadCsv} title="Таблица по строкам файла">Скачать CSV</button>
      </div>
      <ResultsTable groups={visible} types={types} running={running}
        onDraft={setDraft}
        onDraftPrice={(key, draftPrice) => updateGroup(key, (group) => ({ ...group, draftPrice }))}
        onSave={save}
        onRetry={(key) => void run([key])} />
      <p className="dir-foot">Нажмите на строку, чтобы увидеть, откуда взяты наименование и цена, и поправить их. Новые марки попадают в базу только после подтверждения.</p>
    </>}
  </div>
}

function MappingSelect({ label, value, headers, onChange, required, hint }: { label: string; value: string; headers: string[]; onChange: (value: string) => void; required?: boolean; hint?: string }) {
  return <label className="cer-field" title={hint}><span>{label}{required && <span className="cer-required"> *</span>}</span>
    <select value={value} onChange={(event) => onChange(event.target.value)}>
      <option value="">—</option>
      {headers.map((header) => <option key={header} value={header}>{header}</option>)}
    </select>
  </label>
}
