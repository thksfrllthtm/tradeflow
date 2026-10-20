import { useEffect, useRef, useState } from 'react'
import { nomenclatureApi } from '@/features/directories/nomenclature/nomenclatureApi'
import { productDirectoryApi } from '@/features/directories/products/productDirectoryApi'
import { DirectoryPickerEntry, DirectoryPickerMode } from '../components/DirectoryPickerModal/DirectoryPickerModal'
import { requestErrorMessage } from '../lib/requestErrorMessage'
import { NomenclatureImport } from '@/shared/tnved/nomenclature'

type Results = Record<DirectoryPickerMode, DirectoryPickerEntry[]>

// Сколько кодов номенклатуры показывать за раз: их ~13 000, уточняйте поиском.
const codeLimit = 50

// Поиск для окна выбора: карточки товаров (название, ИТС, код) и коды номенклатуры ТН ВЭД
// ЕАЭС. Ставки у обеих вкладок — из номенклатуры, как в расчёте. Ввод ищет с задержкой
// 300 мс; устаревшие ответы (пользователь уже ввёл другое) отбрасываются.
export function useDirectorySearch(onError: (message: string) => void) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Results>({ products: [], tnved: [] })
  const [codeTotal, setCodeTotal] = useState(0)
  const [source, setSource] = useState<NomenclatureImport | null>(null)
  const [loading, setLoading] = useState(false)
  const [mode, setMode] = useState<DirectoryPickerMode>('tnved')
  const timer = useRef<ReturnType<typeof window.setTimeout> | null>(null)
  const requestRef = useRef(0)
  const cancelPending = () => { if (timer.current) window.clearTimeout(timer.current) }

  async function search(nextQuery: string) {
    const requestId = ++requestRef.current
    setQuery(nextQuery)
    setLoading(true)
    try {
      const [codes, products] = await Promise.all([nomenclatureApi.list({ q: nextQuery.trim(), offset: 0, limit: codeLimit }), productDirectoryApi.list({ q: nextQuery })])
      const withCode = products.filter((product) => product.code)
      // Ставки товаров — из номенклатуры по их кодам (одним запросом).
      const official = withCode.length ? await nomenclatureApi.lookup([...new Set(withCode.map((product) => product.code))]) : null
      if (requestId !== requestRef.current) return
      const byCode = new Map((official?.items ?? []).map((item) => [item.code, item]))
      setResults({
        products: withCode.map((product) => ({ kind: 'product', key: `product-${product.id}`, code: product.code, product, nomenclature: byCode.get(product.code) ?? null })),
        tnved: codes.items.map((item) => ({ kind: 'code', key: `code-${item.code}`, code: item.code, nomenclature: item })),
      })
      setCodeTotal(codes.total)
      setSource(codes.import)
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
    source,
    entries: results[mode],
    counts: { products: results.products.length, tnved: codeTotal },
    // Кодов больше, чем показано: подсказать уточнить поиск.
    moreCodes: Math.max(0, codeTotal - results.tnved.length),
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
    // Открытие окна: поиск по initial (пусто — все записи), вкладка остаётся той, что выбирали в прошлый раз.
    restart(initial = '') {
      cancelPending()
      void search(initial)
    },
    reset() {
      setQuery('')
      setResults({ products: [], tnved: [] })
    },
  }
}
