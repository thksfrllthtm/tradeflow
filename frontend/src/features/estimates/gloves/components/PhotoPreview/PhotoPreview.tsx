import '../../../shared/styles/photoLightbox.css'

// Одно фото строки крупно; клик по фону закрывает.
export function PhotoPreview({ photo, onClose }: { photo: string; onClose: () => void }) {
  return <div className="image-lightbox product-lightbox" role="dialog" aria-modal="true" onClick={onClose}>
    <div className="image-lightbox-card" onClick={(event) => event.stopPropagation()}>
      <button type="button" className="image-lightbox-close" onClick={onClose} aria-label="Закрыть">×</button>
      <img src={photo} alt="Фото товара" />
    </div>
  </div>
}
