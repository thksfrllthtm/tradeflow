import { useEffect, useRef, useState } from 'react'
import { productDirectoryApi } from '@/features/directories/products/productDirectoryApi'
import { tnvedDirectoryApi } from '@/features/directories/tnved/tnvedDirectoryApi'
import { DirectoryPickerEntry, DirectoryPickerMode } from '../components/DirectoryPickerModal/DirectoryPickerModal'
import { requestErrorMessage } from '../lib/requestErrorMessage'

type Results = Record<DirectoryPickerMode, DirectoryPickerEntry[]>

// Поиск для окна выбора: один запрос возвращает и карточки товаров (со связанным кодом),
// и коды ТН ВЭД, поэтому переключение вкладок мгновенное. Ввод ищет с задержкой 300 мс;
// устаревшие ответы (пользователь уже ввёл другое) отбрасываются.
export function useDirectorySearch(onError: (message: string) => void) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Results>({ products: [], tnved: [] })
  const [loading, setLoading] = useState(false)
  const [mode, setMode] = useState<DirectoryPickerMode>('products')
  const timer = useRef<ReturnType<typeof window.setTimeout> | null>(null)
  const requestRef = useRef(0)
  const cancelPending = () => { if (timer.current) window.clearTimeout(timer.current) }

  async function search(nextQuery: string) {
    const requestId = ++requestRef.current
    setQuery(nextQuery)
    setLoading(true)
    try {
      const [codes, products] = await Promise.all([tnvedDirectoryApi.list({ q: nextQuery }), productDirectoryApi.list({ q: nextQuery })])
      if (requestId !== requestRef.current) return
      const productCards: DirectoryPickerEntry[] = products.filter((product) => product.tnved && !product.tnved.archivedAt).map((product) => ({ ...product.tnved!, id: -product.id, name: product.name, nameZh: product.nameZh || product.tnved!.nameZh, code: product.tnvedCode, companyCoefficient: product.companyCoefficient, antiDumpingPercent: product.antiDumpingPercent, productDirectory: product }))
      setResults({ products: productCards, tnved: codes })
    } catch (reason) {
      if (requestId === requestRef.current) onError(requestErrorMessage(reason))
    } finally {
      if (requestId === requestRef.current) setLoading(false)
    }
  }

  useEffect(() => {
    void search('')
    return cancelPending
  }, [])

  return {
    query,
    mode,
    loading,
    entries: results[mode],
    counts: { products: results.products.length, tnved: results.tnved.length },
    changeMode: setMode,
    changeQuery(value: string) {
      setQuery(value)
      cancelPending()
      timer.current = window.setTimeout(() => { void search(value) }, 300)
    },
    searchNow() {
      cancelPending()
      void search(query)
    },
    // Открытие окна: все записи, вкладка остаётся той, что выбирали в прошлый раз.
    restart() {
      cancelPending()
      void search('')
    },
    reset() {
      setQuery('')
      setResults({ products: [], tnved: [] })
    },
  }
}
