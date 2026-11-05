import { Fragment, useState } from 'react'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { CeramicsType, PriceResult } from '../types'
import { Draft, draftSize, MatchGroup } from '../lib/matchGroups'
import { formatSize } from '../lib/sizes'
import { ResultDetails } from './ResultDetails'

type Props = {
  groups: MatchGroup[]
  types: CeramicsType[]
  running: boolean
  onDraft: (key: string, change: Partial<Draft>) => void
  onDraftPrice: (key: string, price: PriceResult | undefined) => void
  onCheck: (key: string) => void
  onRetry: (key: string) => void
}

const shortStatus: Record<string, string> = {
  FOUND_BY_MARK: 'По марке', AUTO: 'Авто', NEED_REVIEW: 'Проверить', NOT_DETERMINED: 'Не найдено',
  pending: 'Ожидает', running: 'Анализ…', error: 'Ошибка',
}
// Тон общей точки-статуса (.status-dot) для статуса строки.
export const statusTone: Record<string, string> = {
  FOUND_BY_MARK: 'ok', AUTO: 'info', NEED_REVIEW: 'warn', NOT_DETERMINED: 'error', error: 'error', running: 'info cer-running',
}

// Результат подбора: одна строка на марку. Строка раскрывается — там причины и правка.
export function ResultsTable({ groups, types, running, onDraft, onDraftPrice, onCheck, onRetry }: Props) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [lightbox, setLightbox] = useState('')
  const toggle = (key: string) => setExpanded((current) => {
    const next = new Set(current)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    return next
  })
  // После «Готово» — сразу следующая марка, которую нужно проверить: не искать её глазами.
  const openNext = (key: string) => setExpanded((current) => {
    const next = new Set(current)
    next.delete(key)
    const index = groups.findIndex((group) => group.key === key)
    const following = groups.slice(index + 1).find((group) => group.resolution && group.resolution.status !== 'FOUND_BY_MARK' && !group.checked)
    if (following) next.add(following.key)
    return next
  })

  return <div className="dir-table-wrap cer-results">
    <table className="sheet sheet-rownum dir-table cer-table">
      <thead><tr>
        <th className="c-photo" aria-label="Фото" />
        <th className="c-mark">Марка</th>
        <th className="c-zh">Китайское</th>
        <th className="c-name">Наименование</th>
        <th className="c-size">Размер</th>
        <th className="c-price">Цена</th>
        <th className="c-status">Статус</th>
        <th className="c-chevron" aria-label="Подробнее" />
      </tr></thead>
      <tbody>
        {groups.length === 0 && <tr><td colSpan={8} className="dir-empty">Нет строк с таким статусом.</td></tr>}
        {groups.map((group) => {
          const resolution = group.resolution
          const open = expanded.has(group.key)
          const status = resolution?.status ?? group.state
          const size = formatSize(draftSize(group.draft))
          const sizeFlag = resolution && resolution.status !== 'FOUND_BY_MARK' && (resolution.sizeStatus === 'ASSUMED' || resolution.sizeStatus === 'NEED_REVIEW')
          return <Fragment key={group.key}>
            <tr className={`cer-row${open ? ' open' : ''}${resolution ? '' : ' idle'}`} onClick={() => resolution && toggle(group.key)}>
              <td className="c-photo">{group.photo
                ? <button type="button" className="cer-thumb" onClick={(event) => { event.stopPropagation(); setLightbox(group.photo) }} aria-label="Открыть фото"><img src={group.photo} alt="" loading="lazy" /></button>
                : <span className="cer-thumb empty" aria-hidden="true" />}</td>
              <td className="c-mark">{group.mark || <span className="dir-none">—</span>}
                {group.sourceRows.length > 1 && <span className="cer-repeat" title={`Марка встречается в ${group.sourceRows.length} строках файла`}> ×{group.sourceRows.length}</span>}
              </td>
              <td className="c-zh" title={group.chineseName}>{group.chineseName || <span className="dir-none">—</span>}</td>
              <td className="c-name">
                {group.draft.name
                  ? <span className="cer-name">{group.draft.name}</span>
                  : <span className="dir-none">{resolution?.typeCandidates.length ? `выберите тип: ${resolution.typeCandidates.length} вар.` : '—'}</span>}
              </td>
              <td className="c-size">{size || <span className="dir-none">—</span>}{sizeFlag && <span className="cer-flag" title={resolution.sizeStatus === 'ASSUMED' ? 'Размер предположен — проверьте' : 'Размер неоднозначен'}>?</span>}</td>
              <td className="c-price">{group.draft.price || <span className="dir-none">—</span>}</td>
              <td className="c-status">
                <span className={`status-dot ${statusTone[status] ?? ''}`} title={group.error}>{shortStatus[status] ?? status}</span>
                {group.checked && <small>проверено</small>}
              </td>
              <td className="c-chevron" onClick={(event) => event.stopPropagation()}>
                {group.state === 'error' && !running
                  ? <button type="button" className="button-link" onClick={() => onRetry(group.key)}>Повторить</button>
                  : resolution && <button type="button" className="icon-button ghost sm cer-chevron" onClick={() => toggle(group.key)} aria-expanded={open} aria-label={open ? 'Свернуть' : 'Подробнее'}>›</button>}
              </td>
            </tr>
            {open && resolution && <tr className="cer-details-row sheet-section"><td colSpan={8}>
              <ResultDetails group={group} resolution={resolution} types={types}
                onDraft={(change) => onDraft(group.key, change)}
                onDraftPrice={(price) => onDraftPrice(group.key, price)}
                onDone={() => { onCheck(group.key); openNext(group.key) }} />
            </td></tr>}
          </Fragment>
        })}
      </tbody>
    </table>
    {lightbox && <PhotoLightbox photos={[lightbox]} startIndex={0} onClose={() => setLightbox('')} />}
  </div>
}
