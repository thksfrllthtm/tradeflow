import { ChangeEvent, Dispatch, SetStateAction } from 'react'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { rowId } from './excelColumns'

type RowState = {
  setItems: Dispatch<SetStateAction<GoodsItemInput[]>>
  setResult: Dispatch<SetStateAction<GoodsCalculationResult | null>>
  setSelectedForDirectory: Dispatch<SetStateAction<string[]>>
  setError: (message: string) => void
}

// Правка одного поля строки. Таблица правит название поставщика: supplierName держим в том же
// значении, чтобы сервер получил исправленное (название из справочника — отдельный снимок).
export function editItem(item: GoodsItemInput, field: keyof GoodsItemInput, value: string | boolean | undefined): GoodsItemInput {
  const updated = { ...item, [field]: value }
  if (field === 'name' && typeof value === 'string') updated.supplierName = value
  if (field === 'netKg' && typeof value === 'string') updated.netSource = value.trim() ? 'manual' : ''
  if (field === 'deliveryUsd' && typeof value === 'string') updated.deliveryManual = value.trim() !== ''
  return updated
}

export function emptyItem(id: string, packageWeightKg: string): GoodsItemInput {
  return { id, sourceRow: 0, name: '', supplierName: '', standardName: '', code: '', pairs: '', places: '', pairsPerPlace: '', grossKg: '', grossWeightUnit: 'kg', netKg: '', netWeightUnit: 'kg', netSource: '', packageWeightKg, coefficient: '', pricePerPair: '', costInput: '', costCurrency: '', extraExpenses: '', customsValueDirectKzt: '', deliveryUsd: '', deliveryManual: false, deliveryAlreadyInCost: false, deliveryIncluded: false, excluded: false, photoUrl: '', photoUrls: [] }
}

// Правка строк на этапе 2 (сметы товаров). Любая правка сбрасывает
// результат расчёта, потому что он больше не соответствует строкам.
export function rowActions({ setItems, setResult, setSelectedForDirectory, setError }: RowState, itemCount: number, packageWeightKg: string) {
  return {
    addManualPhoto(id: string, event: ChangeEvent<HTMLInputElement>) {
      const file = event.target.files?.[0]
      event.target.value = ''
      if (!file) return
      if (!['image/jpeg', 'image/png'].includes(file.type)) {
        setError('Фото должно быть в формате JPG или PNG')
        return
      }
      if (file.size > 5 * 1024 * 1024) {
        setError('Размер фотографии не должен превышать 5 МБ')
        return
      }
      const reader = new FileReader()
      reader.onload = () => {
        const dataURL = typeof reader.result === 'string' ? reader.result : ''
        if (!dataURL) return
        setItems((current) => current.map((item) => {
          if (item.id !== id) return item
          const photos = item.photoUrls?.length ? item.photoUrls : item.photoUrl ? [item.photoUrl] : []
          return { ...item, photoUrl: photos[0] || dataURL, photoUrls: [...photos, dataURL] }
        }))
        setResult(null)
        setError('')
      }
      reader.onerror = () => setError('Не удалось прочитать фотографию')
      reader.readAsDataURL(file)
    },

    addEmptyRow() {
      setItems((current) => [...current, emptyItem(rowId(0, itemCount), packageWeightKg)])
      setResult(null)
    },

    removeRow(id: string) {
      setItems((current) => current.filter((item) => item.id !== id))
      setSelectedForDirectory((current) => current.filter((selected) => selected !== id))
      setResult(null)
    },
  }
}
