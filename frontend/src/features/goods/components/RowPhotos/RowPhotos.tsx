import { useEffect, useMemo, useRef, useState } from 'react'
import { photosOf } from '@/shared/lib/itemPhotos'
import { GoodsItemInput } from '@/shared/types/goods'
import { MAX_ROW_PHOTOS } from '../../lib/rowPhotos'
import './RowPhotos.css'

type Props = {
  items: GoodsItemInput[]
  itemId: string
  startIndex: number
  onClose: () => void
  onRemove: (itemId: string, index: number) => void
  onMakeMain: (itemId: string, index: number) => void
  onMove: (itemId: string, index: number, toId: string) => void
}

const rowName = (item: GoodsItemInput) => item.name || item.supplierName || 'Без наименования'

// Фото строки сметы: слева фото крупно, справа — все фото строки и действия: сделать главным,
// перенести в другую строку (список строк с поиском), удалить. ← → листают, Esc закрывает.
// Строка без фото закрывает окно.
export function RowPhotos({ items, itemId, startIndex, onClose, onRemove, onMakeMain, onMove }: Props) {
  const rowIndex = items.findIndex((item) => item.id === itemId)
  const item = items[rowIndex]
  const photos = item ? photosOf(item) : []
  const [index, setIndex] = useState(startIndex)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [moving, setMoving] = useState(false)
  const [query, setQuery] = useState('')
  const [moved, setMoved] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)
  const current = Math.max(0, Math.min(index, photos.length - 1))
  const many = photos.length > 1

  function select(next: number) {
    setIndex(next); setConfirmDelete(false); setMoving(false)
  }
  const step = (delta: number) => select((current + delta + photos.length) % photos.length)

  useEffect(() => { if (photos.length === 0) onClose() }, [photos.length, onClose])
  useEffect(() => { if (moving) searchRef.current?.focus() }, [moving])
  useEffect(() => {
    if (!moved) return
    const timer = window.setTimeout(() => setMoved(''), 2600)
    return () => window.clearTimeout(timer)
  }, [moved])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        if (moving) setMoving(false)
        else if (confirmDelete) setConfirmDelete(false)
        else onClose()
        return
      }
      if ((event.target as HTMLElement).tagName === 'INPUT') return
      if (event.key === 'ArrowLeft' && many) step(-1)
      if (event.key === 'ArrowRight' && many) step(1)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  })

  const targets = useMemo(() => {
    const words = query.trim().toLowerCase()
    return items.map((other, at) => ({ other, at })).filter(({ other, at }) => other.id && other.id !== itemId
      && (!words || String(at + 1) === words || rowName(other).toLowerCase().includes(words)))
  }, [items, itemId, query])

  if (!item || photos.length === 0) return null

  function moveTo(target: GoodsItemInput, at: number) {
    onMove(itemId, current, target.id ?? '')
    setMoving(false); setQuery('')
    setMoved(`Фото перенесено в строку ${at + 1}`)
  }

  return <div className="rp-backdrop" role="dialog" aria-modal="true" aria-label={`Фото строки ${rowIndex + 1}`} onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <div className="rp-card">
      <div className="rp-stage">
        <img src={photos[current]} alt={`Фото ${current + 1} из ${photos.length}, строка ${rowIndex + 1}`} />
        {many && <>
          <button type="button" className="rp-nav prev" onClick={() => step(-1)} aria-label="Предыдущее фото">‹</button>
          <button type="button" className="rp-nav next" onClick={() => step(1)} aria-label="Следующее фото">›</button>
        </>}
        <span className="rp-counter">{current + 1} / {photos.length}{current === 0 && ' · главное'}</span>
      </div>

      <aside className="rp-side">
        <header className="rp-head">
          <div>
            <span className="rp-row">Строка {rowIndex + 1}</span>
            <h2 title={rowName(item)}>{rowName(item)}</h2>
          </div>
          <button type="button" className="rp-close" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)">×</button>
        </header>

        <section className="rp-section">
          <h3>Фото строки <small>{photos.length}</small></h3>
          <div className="rp-grid" role="listbox" aria-label="Все фото строки">
            {photos.map((photo, at) => <button key={at} type="button" role="option" aria-selected={at === current} className={at === current ? 'rp-thumb active' : 'rp-thumb'} onClick={() => select(at)} title={at === 0 ? 'Главное фото' : `Фото ${at + 1}`}>
              <img src={photo} alt="" />{at === 0 && <i>главное</i>}
            </button>)}
          </div>
        </section>

        <section className="rp-section rp-actions">
          {current > 0 && <button type="button" className="rp-action" onClick={() => { onMakeMain(itemId, current); select(0) }}>
            <b aria-hidden="true">★</b><span>Сделать главным<small>миниатюра строки и первое фото для подбора кода</small></span>
          </button>}

          <div className={moving ? 'rp-move open' : 'rp-move'}>
            <button type="button" className="rp-action" onClick={() => { setMoving((open) => !open); setConfirmDelete(false) }} aria-expanded={moving} disabled={items.length < 2}>
              <b aria-hidden="true">⇄</b><span>Перенести в другую строку<small>если фото попало не к тому товару</small></span>
            </button>
            {moving && <div className="rp-targets">
              <input ref={searchRef} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Номер строки или название" aria-label="Найти строку" />
              <ul>
                {targets.map(({ other, at }) => {
                  const count = photosOf(other).length
                  const full = count >= MAX_ROW_PHOTOS
                  return <li key={other.id}>
                    <button type="button" onClick={() => moveTo(other, at)} disabled={full} title={full ? `У строки уже ${MAX_ROW_PHOTOS} фото` : `Перенести фото в строку ${at + 1}`}>
                      <span className="rp-target-num">{at + 1}</span>
                      {count ? <img src={photosOf(other)[0]} alt="" /> : <span className="rp-target-empty" aria-hidden="true" />}
                      <span className="rp-target-name">{rowName(other)}</span>
                      <small>{full ? 'полно' : count ? `${count} фото` : 'без фото'}</small>
                    </button>
                  </li>
                })}
                {targets.length === 0 && <li className="rp-target-none">Строк не найдено</li>}
              </ul>
            </div>}
          </div>

          {confirmDelete
            ? <div className="rp-confirm">
                <span>Удалить это фото из строки?</span>
                <div>
                  <button type="button" className="rp-danger" onClick={() => { onRemove(itemId, current); setConfirmDelete(false) }}>Удалить</button>
                  <button type="button" className="rp-cancel" onClick={() => setConfirmDelete(false)}>Отмена</button>
                </div>
              </div>
            : <button type="button" className="rp-action delete" onClick={() => { setConfirmDelete(true); setMoving(false) }}>
                <b aria-hidden="true">🗑</b><span>Удалить фото</span>
              </button>}
        </section>

        <footer className="rp-foot">
          {moved ? <span className="rp-done" role="status">✓ {moved}</span> : <span>← → листать · Esc закрыть</span>}
        </footer>
      </aside>
    </div>
  </div>
}
