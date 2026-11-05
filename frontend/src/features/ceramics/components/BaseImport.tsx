import { ChangeEvent, useMemo, useState } from 'react'
import { goodsApi } from '@/shared/api/goodsApi'
import { GoodsParseResult } from '@/shared/types/goods'
import { errorMessage } from '@/shared/lib/errorMessage'
import { ceramicsApi } from '../ceramicsApi'
import { CeramicsProductInput } from '../types'
import { normalizeMark } from '../lib/marks'
import { parsePriceCell, priceNumber } from '../lib/sizes'

type Mapping = { name: string; mark: string; price: string }
// duplicateOf — строка полностью повторяет более раннюю (та же марка, наименование и цена): не ошибка,
// загружается один раз.
type Row = { sourceRow: number; input: CeramicsProductInput; rawPrice: string; errors: string[]; duplicateOf?: number }

const guesses: Record<keyof Mapping, RegExp> = {
  name: /наимен|назван|品名|名称|товар/i,
  mark: /марк|артикул|型号|货号|款号|item\s*no|model/i,
  price: /цена|price|单价|价格|стоим/i,
}

function guess(headers: string[]): Mapping {
  const used = new Set<string>()
  const pick = (pattern: RegExp) => {
    const header = headers.find((value) => !used.has(value) && pattern.test(value)) ?? ''
    if (header) used.add(header)
    return header
  }
  return { name: pick(guesses.name), mark: pick(guesses.mark), price: pick(guesses.price) }
}

// Загрузка имеющегося прайса в базу: наименование, марка и цена. Строки с ошибками
// показываются и не загружаются.
export function BaseImport({ onCancel, onImported }: { onCancel: () => void; onImported: (message: string) => void }) {
  const [parsed, setParsed] = useState<GoodsParseResult | null>(null)
  const [fileName, setFileName] = useState('')
  const [headerRow, setHeaderRow] = useState(1)
  const [file, setFile] = useState<File | null>(null)
  const [mapping, setMapping] = useState<Mapping>({ name: '', mark: '', price: '' })
  const [overwrite, setOverwrite] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [onlyProblems, setOnlyProblems] = useState(false)

  async function parse(nextFile: File, nextHeaderRow: number) {
    setBusy(true); setError('')
    try {
      const result = await goodsApi.parseXlsx(nextFile, { headerRow: nextHeaderRow, dataStartRow: nextHeaderRow + 1 })
      setParsed(result)
      setFileName(nextFile.name)
      setMapping(guess(result.headers))
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  function upload(event: ChangeEvent<HTMLInputElement>) {
    const selected = event.target.files?.[0]
    event.target.value = ''
    if (!selected) return
    setFile(selected)
    void parse(selected, headerRow)
  }

  const rows = useMemo<Row[]>(() => {
    if (!parsed) return []
    const seen = new Map<string, { sourceRow: number; name: string; price: string }>()
    return parsed.rows.filter((row) => row.included).flatMap((row) => {
      const value = (header: string) => (header ? row.values[header] ?? '' : '').trim()
      const name = value(mapping.name).replace(/\s+/g, ' ')
      const mark = value(mapping.mark)
      if (!mark && !name) return []
      const errors: string[] = []
      if (!name) errors.push('нет наименования')
      if (!mark) errors.push('нет марки')
      const price = parsePriceCell(value(mapping.price))
      if (!price) errors.push(!value(mapping.price) ? 'нет цены' : priceNumber(value(mapping.price)) ? 'цена равна нулю' : `цена «${value(mapping.price)}» не распознана — нужно число`)
      const key = normalizeMark(mark)
      const earlier = key ? seen.get(key) : undefined
      if (earlier && earlier.name === name && earlier.price === price) {
        return [{ sourceRow: row.sourceRow, rawPrice: value(mapping.price), errors, input: { name, mark, price }, duplicateOf: earlier.sourceRow }]
      }
      if (earlier) {
        const differs = [earlier.name !== name && 'наименованием', earlier.price !== price && 'ценой'].filter(Boolean).join(' и ')
        errors.push(`марка уже есть в строке ${earlier.sourceRow} с другим ${differs}`)
      } else if (key) seen.set(key, { sourceRow: row.sourceRow, name, price })
      return [{ sourceRow: row.sourceRow, rawPrice: value(mapping.price), errors, input: { name, mark, price } }]
    })
  }, [parsed, mapping])

  const valid = rows.filter((row) => row.errors.length === 0 && !row.duplicateOf)
  const invalid = rows.filter((row) => row.errors.length > 0).length
  const duplicates = rows.filter((row) => row.duplicateOf).length
  // Проблемные строки — первыми: в большом прайсе их иначе не найти.
  const shown = [...rows.filter((row) => row.errors.length || row.duplicateOf), ...(onlyProblems ? [] : rows.filter((row) => !row.errors.length && !row.duplicateOf))]

  async function submit() {
    setBusy(true); setError('')
    try {
      const result = await ceramicsApi.importProducts(valid.map((row) => row.input), overwrite)
      onImported(`Загружено: новых ${result.created}, обновлено ${result.updated}, пропущено существующих ${result.skipped}`)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  const select = (field: keyof Mapping, label: string) => <label className="cer-field"><span>{label}<span className="cer-required"> *</span></span>
    <select value={mapping[field]} onChange={(event) => setMapping({ ...mapping, [field]: event.target.value })}>
      <option value="">— не выбрано —</option>
      {parsed?.headers.map((header) => <option key={header} value={header}>{header}</option>)}
    </select>
  </label>

  return <section className="cer-card cer-import">
    <div className="cer-card-row">
      <h3>Загрузка базы из Excel</h3>
      <label className="secondary cer-file-button">{busy && !parsed ? 'Читаем…' : parsed ? 'Другой файл' : 'Выбрать .xlsx'}<input type="file" accept=".xlsx" onChange={upload} disabled={busy} /></label>
      {fileName && <span className="cer-file-name">{fileName}</span>}
      {parsed && <label className="cer-field narrow" title="Номер строки Excel с названиями колонок">Заголовки в строке
        <input type="number" min={1} value={headerRow} disabled={busy} onChange={(event) => setHeaderRow(Math.max(1, Number(event.target.value) || 1))}
          onBlur={() => { if (file) void parse(file, headerRow) }} />
      </label>}
      <span className="cer-toolbar-spacer" />
      <button type="button" className="secondary" onClick={onCancel}>Закрыть</button>
    </div>
    <p className="muted">Три колонки: наименование («Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘»), марка и цена. Тип и размер система выделит из наименования.</p>
    {error && <p className="error" role="alert">{error}</p>}
    {parsed && <>
      <div className="cer-mapping">
        {select('name', 'Наименование')}
        {select('mark', 'Марка')}
        {select('price', 'Цена')}
      </div>
      {(invalid > 0 || duplicates > 0) && <label className="cer-check"><input type="checkbox" checked={onlyProblems} onChange={(event) => setOnlyProblems(event.target.checked)} />Показать только строки с ошибками и повторы ({invalid + duplicates}) — они всегда вверху таблицы</label>}
      <div className="dir-table-wrap cer-preview">
        <table className="sheet dir-table cer-table">
          <thead><tr><th>Строка</th><th>Наименование</th><th className="c-mark">Марка</th><th className="c-price">Цена</th><th>Проверка</th></tr></thead>
          <tbody>
            {rows.length === 0 && <tr><td colSpan={5} className="dir-empty">Выберите колонки наименования, марки и цены.</td></tr>}
            {shown.slice(0, 300).map((row) => <tr key={row.sourceRow} className={row.errors.length ? 'cer-invalid' : undefined}>
              <td>{row.sourceRow}</td><td>{row.input.name}</td><td className="c-mark"><code>{row.input.mark}</code></td>
              <td className="c-price">{row.input.price || <span className="dir-warn">{row.rawPrice || '—'}</span>}</td>
              <td>{row.errors.length ? <span className="dir-warn">{row.errors.join('; ')}</span> : row.duplicateOf ? <span className="muted">повтор строки {row.duplicateOf} — загрузится один раз</span> : 'ок'}</td>
            </tr>)}
          </tbody>
        </table>
      </div>
      <div className="cer-card-row end">
        <span className="muted">Готово к загрузке: {valid.length}{invalid > 0 && `, с ошибками (не загрузятся): ${invalid}`}{duplicates > 0 && `, полных повторов: ${duplicates}`}{shown.length > 300 && `. Показаны первые 300 строк`}</span>
        <label className="cer-check"><input type="checkbox" checked={overwrite} onChange={(event) => setOverwrite(event.target.checked)} />Обновлять марки, которые уже есть в базе</label>
        <button type="button" disabled={busy || valid.length === 0} onClick={() => void submit()}>{busy ? 'Загрузка…' : `Загрузить ${valid.length}`}</button>
      </div>
    </>}
  </section>
}
