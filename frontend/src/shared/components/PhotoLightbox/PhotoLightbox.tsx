import { MouseEvent, useEffect, useState } from 'react'
import { useDialogFocus } from '@/shared/hooks/useDialogFocus'
import { useEscape } from '@/shared/hooks/useEscape'
import './PhotoLightbox.css'

type Props = { photos: string[]; startIndex: number; onClose: () => void }

const ZOOM = 2.5

// Просмотр фотографий товара: стрелки ‹ › и клавиши ← → листают, Esc закрывает.
// Щелчок по фото — увеличение в 2,5 раза в месте щелчка, движение мыши двигает увеличенную область,
// повторный щелчок — обратно.
export function PhotoLightbox({ photos, startIndex, onClose }: Props) {
  const [index, setIndex] = useState(startIndex)
  // Точка увеличения в процентах от фото; null — фото целиком.
  const [zoom, setZoom] = useState<{ x: number; y: number } | null>(null)
  const many = photos.length > 1
  const move = (step: number) => { setZoom(null); setIndex((current) => (current + step + photos.length) % photos.length) }
  const point = (event: MouseEvent<HTMLImageElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    return { x: ((event.clientX - rect.left) / rect.width) * 100, y: ((event.clientY - rect.top) / rect.height) * 100 }
  }

  useEscape(onClose)
  const dialogRef = useDialogFocus<HTMLDivElement>()
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'ArrowLeft' && many) move(-1)
      if (event.key === 'ArrowRight' && many) move(1)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  })

  return <div ref={dialogRef} tabIndex={-1} className="image-lightbox product-lightbox" role="dialog" aria-modal="true" aria-label="Фотографии товара" onClick={onClose}>
    <div className="image-lightbox-card image-gallery-card" onClick={(event) => event.stopPropagation()}>
      <button type="button" className="image-lightbox-close" onClick={onClose} aria-label="Закрыть">×</button>
      {many && <button type="button" className="image-gallery-arrow image-gallery-prev" onClick={() => move(-1)} aria-label="Предыдущая фотография">‹</button>}
      <div className="image-zoom">
        <img src={photos[index]} alt={`Фото товара ${index + 1} из ${photos.length}`} className={zoom ? 'zoomed' : undefined}
          title={zoom ? 'Щёлкните, чтобы уменьшить' : 'Щёлкните, чтобы увеличить'}
          style={zoom ? { transform: `scale(${ZOOM})`, transformOrigin: `${zoom.x}% ${zoom.y}%` } : undefined}
          onClick={(event) => setZoom(zoom ? null : point(event))}
          onMouseMove={(event) => { if (zoom) setZoom(point(event)) }} />
      </div>
      {many && <button type="button" className="image-gallery-arrow image-gallery-next" onClick={() => move(1)} aria-label="Следующая фотография">›</button>}
      {many && <small className="image-gallery-counter">{index + 1} / {photos.length}</small>}
    </div>
  </div>
}
