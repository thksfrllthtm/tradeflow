import { FormEvent, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { ceramicsApi } from '../ceramicsApi'
import { CeramicsProduct, CeramicsProductInput } from '../types'
import { normalizeNumber } from '../lib/sizes'

type Props = { product?: CeramicsProduct; onCancel: () => void; onSaved: (product: CeramicsProduct) => void }

// Добавление и правка марки: наименование, марка и цена. Тип и размер сервер выделяет из
// наименования («… 7 дюймов …/ 成年人陶瓷盘7高脚水果盘»).
export function ProductForm({ product, onCancel, onSaved }: Props) {
  const [form, setForm] = useState<CeramicsProductInput>({ mark: product?.mark ?? '', name: product?.name ?? '', price: product?.price ?? '' })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const set = (change: Partial<CeramicsProductInput>) => setForm((current) => ({ ...current, ...change }))

  async function submit(event: FormEvent) {
    event.preventDefault()
    setSaving(true); setError('')
    const input = { ...form, price: normalizeNumber(form.price) || form.price.trim() }
    try {
      onSaved(product ? await ceramicsApi.updateProduct(product.id, input) : await ceramicsApi.createProduct(input))
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setSaving(false)
    }
  }

  return <form className="cer-card cer-form" onSubmit={(event) => void submit(event)}>
    <h3>{product ? `Марка ${product.mark}` : 'Новая марка'}</h3>
    <div className="cer-form-grid">
      <label className="cer-field wide">Наименование *
        <input value={form.name} onChange={(event) => set({ name: event.target.value })} required maxLength={500} autoFocus={!product}
          placeholder="Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘" />
      </label>
      <label className="cer-field">Марка *<input value={form.mark} onChange={(event) => set({ mark: event.target.value })} required maxLength={128} /></label>
      <label className="cer-field">Цена *<input value={form.price} inputMode="decimal" onChange={(event) => set({ price: event.target.value })} required /></label>
    </div>
    {product && <p className="cer-display-name">Распознано из наименования: <strong>{product.productType}</strong>{product.sizeValue ? `, размер ${product.sizeValue}${product.sizeUnit === 'inch' ? '"' : ` ${product.sizeUnit}`}` : ', без размера'}</p>}
    {error && <p className="error" role="alert">{error}</p>}
    <div className="cer-editor-actions">
      <button type="button" className="cer-secondary" onClick={onCancel}>Отмена</button>
      <button type="submit" disabled={saving}>{saving ? 'Сохраняем…' : 'Сохранить'}</button>
    </div>
  </form>
}
