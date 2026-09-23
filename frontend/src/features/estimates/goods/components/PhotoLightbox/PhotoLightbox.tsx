import { useEffect, useState } from 'react'
import '../../../shared/styles/photoLightbox.css'

type Props = { photos: string[]; startIndex: number; onClose: () => void }

// Просмотр фотографий товара: стрелки ‹ › и клавиши ← → листают, Esc закрывает.
export function PhotoLightbox({ photos, startIndex, onClose }: Props) {
  const [index, setIndex] = useState(startIndex)
  const many = photos.length > 1
  const move = (step: number) => setIndex((current) => (current + step + photos.length) % photos.length)

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
      if (event.key === 'ArrowLeft' && many) move(-1)
      if (event.key === 'ArrowRight' && many) move(1)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  })

  return <div className="image-lightbox product-lightbox" role="dialog" aria-modal="true" aria-label="Фотографии товара" onClick={onClose}>
    <div className="image-lightbox-card image-gallery-card" onClick={(event) => event.stopPropagation()}>
      <button type="button" className="image-lightbox-close" onClick={onClose} aria-label="Закрыть">×</button>
      {many && <button type="button" className="image-gallery-arrow image-gallery-prev" onClick={() => move(-1)} aria-label="Предыдущая фотография">‹</button>}
      <img src={photos[index]} alt={`Фото товара ${index + 1} из ${photos.length}`} />
      {many && <button type="button" className="image-gallery-arrow image-gallery-next" onClick={() => move(1)} aria-label="Следующая фотография">›</button>}
      {many && <small className="image-gallery-counter">{index + 1} / {photos.length}</small>}
    </div>
  </div>
}
