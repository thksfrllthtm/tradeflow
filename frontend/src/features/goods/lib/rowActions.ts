import { ChangeEvent, Dispatch, SetStateAction } from 'react'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { rowId } from './excelColumns'

type RowState = {
  setItems: Dispatch<SetStateAction<GoodsItemInput[]>>
  setResult: Dispatch<SetStateAction<GoodsCalculationResult | null>>
  setSelectedForDirectory: Dispatch<SetStateAction<string[]>>
  setError: (message: string) => void
}

// Правка строк на этапе 2 (сметы товаров). Любая правка сбрасывает
// результат расчёта, потому что он больше не соответствует строкам.
export function rowActions({ setItems, setResult, setSelectedForDirectory, setError }: RowState, itemCount: number, packageWeightKg: string) {
  return {
    setItem(id: string, field: keyof GoodsItemInput, value: string | boolean | undefined) {
      setItems((current) => current.map((item) => {
        if (item.id !== id) return item
        const updated = { ...item, [field]: value }
        if (field === 'name' && typeof value === 'string') {
          // The table edits the supplier-facing name. Keep the dedicated
          // supplier field in sync so the server receives the edited value,
          // while the selected directory name remains a separate snapshot.
          updated.supplierName = value
        }
        if (field === 'netKg' && typeof value === 'string') {
          updated.netSource = value.trim() ? 'manual' : ''
        }
        if (field === 'deliveryUsd' && typeof value === 'string') {
          updated.deliveryManual = value.trim() !== ''
        }
        return updated
      }))
      setResult(null)
    },

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
      const id = rowId(0, itemCount)
      setItems((current) => [...current, { id, sourceRow: 0, name: '', supplierName: '', standardName: '', code: '', pairs: '', places: '', pairsPerPlace: '', grossKg: '', grossWeightUnit: 'kg', netKg: '', netWeightUnit: 'kg', netSource: '', packageWeightKg, coefficient: '', pricePerPair: '', costInput: '', costCurrency: '', extraExpenses: '', customsValueDirectKzt: '', deliveryUsd: '', deliveryManual: false, deliveryAlreadyInCost: false, deliveryIncluded: false, excluded: false, exclusionReason: '', photoUrl: '', photoUrls: [] }])
      setResult(null)
    },

    removeRow(id: string) {
      setItems((current) => current.filter((item) => item.id !== id))
      setSelectedForDirectory((current) => current.filter((selected) => selected !== id))
      setResult(null)
    },

    resetAutomaticNet(id: string) {
      setItems((current) => current.map((item) => item.id === id ? { ...item, netKg: '', netSource: '', netWeightUnit: 'kg' } : item))
      setResult(null)
    },
  }
}
