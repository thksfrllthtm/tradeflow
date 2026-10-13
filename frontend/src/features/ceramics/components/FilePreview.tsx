import { useMemo, useState } from 'react'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { GoodsParseResult } from '@/shared/types/goods'
import { MatchMapping, PHOTO_FROM_IMAGES } from '../lib/matchGroups'
import { toDataUrl } from '../lib/photos'

type Role = 'mark' | 'chineseName' | 'russianName'
const roles: { value: Role; label: string }[] = [
  { value: 'mark', label: 'Марка' },
  { value: 'chineseName', label: 'Китайское' },
  { value: 'russianName', label: 'Русское' },
]

type Props = {
  parsed: GoodsParseResult
  mapping: MatchMapping
  disabled: boolean
  onMapping: (mapping: MatchMapping) => void
  onRowsIncluded: (sourceRows: number[], included: boolean) => void
  onHeaderRow: (sourceRow: number) => void
}

// Строки файла клиента до подбора. По умолчанию видны только колонки, которые идут в подбор;
// «все колонки» показывают остальные, чтобы выбрать нужную по содержимому.
export function FilePreview({ parsed, mapping, disabled, onMapping, onRowsIncluded, onHeaderRow }: Props) {
  const [lightbox, setLightbox] = useState('')
  const [allColumns, setAllColumns] = useState(false)
  const photos = useMemo(() => {
    const byRow = new Map<number, string>()
    if (mapping.photo === PHOTO_FROM_IMAGES) for (const image of parsed.images) if (!byRow.has(image.sourceRow)) byRow.set(image.sourceRow, toDataUrl(image))
    return byRow
  }, [parsed.images, mapping.photo])
  // Пустые колонки (без значений во всех строках) не показываем никогда.
  const filled = useMemo(() => parsed.headers.filter((header) => parsed.rows.some((row) => (row.values[header] ?? '').trim())), [parsed])
  const mapped = roles.map((role) => mapping[role.value]).filter((header) => header && filled.includes(header))
  // Пока ничего не выбрано, показываем все колонки: иначе не из чего выбирать.
  const showAll = allColumns || mapped.length === 0
  const headers = showAll ? filled : mapped
  const hidden = filled.length - mapped.length
  const included = parsed.rows.filter((row) => row.included).length
  const roleOf = (header: string) => roles.find((role) => mapping[role.value] === header)

  function assign(header: string, role: Role | '') {
    const next = { ...mapping }
    for (const item of roles) if (next[item.value] === header) next[item.value] = ''
    if (role) next[role] = header
    onMapping(next)
  }

  return <section className="cer-file-preview">
    <div className="cer-file-preview-head">
      <strong>Строки файла</strong>
      <span>{included} из {parsed.rows.length} к подбору</span>
      <span className="cer-toolbar-spacer" />
      {mapped.length > 0 && hidden > 0 && <button type="button" className="cer-link" onClick={() => setAllColumns(!allColumns)}>
        {allColumns ? 'только нужные колонки' : `все колонки (+${hidden})`}
      </button>}
    </div>
    <div className="dir-table-wrap cer-preview">
      <table className={showAll ? 'sheet dir-table cer-table cer-raw-table all' : 'sheet dir-table cer-table cer-raw-table'}>
        <thead><tr>
          <th className="c-check"><input type="checkbox" aria-label="Все строки" disabled={disabled} checked={included === parsed.rows.length && included > 0}
            ref={(input) => { if (input) input.indeterminate = included > 0 && included < parsed.rows.length }}
            onChange={(event) => onRowsIncluded(parsed.rows.map((row) => row.sourceRow), event.target.checked)} /></th>
          <th className="c-row">№</th>
          {mapping.photo === PHOTO_FROM_IMAGES && <th className="c-photo">Фото</th>}
          {headers.map((header) => {
            const role = roleOf(header)
            return <th key={header} className={role ? 'mapped' : undefined} title={header}>
              {showAll
                ? <label className="cer-col-role">
                  <span className="cer-col-name">{header}</span>
                  <select value={role?.value ?? ''} disabled={disabled} onChange={(event) => assign(header, event.target.value as Role | '')} aria-label={`Что в колонке «${header}»`}>
                    <option value="">не нужна</option>
                    {roles.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
                  </select>
                </label>
                : <><span className="cer-col-label">{role?.label}</span><span className="cer-col-name muted">{header}</span></>}
            </th>
          })}
          <th className="c-actions" aria-label="Действия" />
        </tr></thead>
        <tbody>
          {parsed.rows.length === 0 && <tr><td colSpan={headers.length + 4} className="dir-empty">Под строкой заголовков нет данных.</td></tr>}
          {parsed.rows.map((row) => {
            const photo = photos.get(row.sourceRow)
            return <tr key={row.sourceRow} className={row.included ? undefined : 'cer-excluded'} title={row.warnings?.join('; ') || undefined}
              onClick={() => { if (!disabled) onRowsIncluded([row.sourceRow], !row.included) }}>
              <td className="c-check"><input type="checkbox" checked={row.included} disabled={disabled} aria-label={`Строка ${row.sourceRow}`}
                onClick={(event) => event.stopPropagation()} onChange={(event) => onRowsIncluded([row.sourceRow], event.target.checked)} /></td>
              <td className="c-row">{row.sourceRow}</td>
              {mapping.photo === PHOTO_FROM_IMAGES && <td className="c-photo">{photo
                ? <button type="button" className="cer-thumb small" onClick={(event) => { event.stopPropagation(); setLightbox(photo) }} aria-label="Открыть фото"><img src={photo} alt="" loading="lazy" /></button>
                : <span className="cer-thumb small empty" aria-hidden="true" />}</td>}
              {headers.map((header) => <td key={header} className={roleOf(header) ? 'mapped' : undefined}><span className="cer-cell">{row.values[header] || ''}</span></td>)}
              <td className="c-actions"><button type="button" className="cer-link" disabled={disabled}
                onClick={(event) => { event.stopPropagation(); onHeaderRow(row.sourceRow) }} title="Названия колонок — в этой строке">это заголовки</button></td>
            </tr>
          })}
        </tbody>
      </table>
    </div>
    {lightbox && <PhotoLightbox photos={[lightbox]} startIndex={0} onClose={() => setLightbox('')} />}
  </section>
}
