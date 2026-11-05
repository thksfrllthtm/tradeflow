import { ChangeEvent, ClipboardEvent, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { clipboardImages } from '@/shared/lib/clipboardImages'
import { resizePhoto } from '@/shared/lib/resizePhoto'
import { decisionsApi } from './decisionsApi'
import { Drawer } from '@/shared/components/Drawer/Drawer'

const maxPhotos = 3

// Ручное добавление решения администратором: фото, наименование и правильный код.
// Идёт тем же путём, что импорт старого инвойса, поэтому запись сразу «Проверено».
export function DecisionManual({ onClose, onAdded }: { onClose: () => void; onAdded: () => void }) {
  const [invoiceName, setInvoiceName] = useState('')
  const [finalName, setFinalName] = useState('')
  const [code, setCode] = useState('')
  const [photos, setPhotos] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function addFiles(files: File[]) {
    const images = files.filter((file) => file.type.startsWith('image/')).slice(0, maxPhotos - photos.length)
    if (images.length === 0) return
    const resized = await Promise.all(images.map(async (file) => {
      const url = URL.createObjectURL(file)
      try { return await resizePhoto(url) } finally { URL.revokeObjectURL(url) }
    }))
    setPhotos((current) => [...current, ...resized.filter((photo) => photo.startsWith('data:'))].slice(0, maxPhotos))
  }

  function pick(event: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.target.files ?? [])
    event.target.value = ''
    void addFiles(files)
  }

  // Скриншот из буфера (Ctrl+V) — как файл; ячейки Excel вставляются текстом, а не их картинкой.
  function paste(event: ClipboardEvent) {
    const files = clipboardImages(event.clipboardData)
    if (files.length) { event.preventDefault(); void addFiles(files) }
  }

  const normalized = code.replace(/\D/g, '')
  async function submit() {
    setBusy(true); setError('')
    try {
      const thumb = photos[0] ? await resizePhoto(photos[0], 160, 0.7) : ''
      const result = await decisionsApi.importRows([{ invoiceName: invoiceName.trim(), finalName: finalName.trim(), code: normalized, photos, thumb, note: 'Добавлено вручную' }])
      if (result.errors.length) setError(result.errors[0].message)
      else if (result.skipped) setError('Такой товар с этим кодом уже есть в базе')
      else { onAdded(); onClose() }
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  // Пока идёт сохранение, панель не закрывается.
  const close = () => { if (!busy) onClose() }
  return <Drawer title="Добавить вручную" subtitle="Запись попадёт в базу сразу как «Проверено»." onClose={close} onSubmit={() => void submit()}
    footer={<>
      <button type="button" className="secondary" onClick={close} disabled={busy}>Отмена</button>
      <button type="submit" disabled={busy || !invoiceName.trim() || normalized.length !== 10}>{busy ? 'Сохраняем…' : 'Добавить'}</button>
    </>}>
      <div className="drawer-fields" onPaste={paste}>
        <section className="drawer-section">
          <h3>Фото</h3>
          {photos.length > 0 && <div className="dc-photos">{photos.map((photo, index) => <button key={index} type="button" className="dc-photo" title="Убрать фото" onClick={() => setPhotos((current) => current.filter((_, other) => other !== index))}><img src={photo} alt={`Фото ${index + 1}`} /></button>)}</div>}
          {photos.length < maxPhotos && <label className="secondary dc-file">Выбрать фото<input type="file" accept="image/png,image/jpeg,image/webp" multiple onChange={pick} disabled={busy} /></label>}
          <p className="drawer-hint">До {maxPhotos} фото. Можно вставить скриншот через Ctrl+V. Нажмите на фото, чтобы убрать.</p>
        </section>
        <section className="drawer-section">
          <label className="drawer-field"><span>Наименование (как в инвойсе)<b className="req">*</b></span><input value={invoiceName} onChange={(event) => setInvoiceName(event.target.value)} disabled={busy} required autoFocus /></label>
          <label className="drawer-field"><span>Итоговое наименование для декларации («рус / кит»)</span><input value={finalName} onChange={(event) => setFinalName(event.target.value)} disabled={busy} /></label>
          <label className="drawer-field"><span>Код ТН ВЭД<b className="req">*</b></span><input value={code} onChange={(event) => setCode(event.target.value)} disabled={busy} required inputMode="numeric" placeholder="8201 10 000 0" /></label>
          {code && normalized.length !== 10 && <p className="drawer-hint">Код — 10 цифр, сейчас {normalized.length}.</p>}
        </section>
        {error && <p className="dc-error" role="alert">{error}</p>}
      </div>
  </Drawer>
}
