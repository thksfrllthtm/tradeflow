import { request } from '@/shared/api/client'
import { NomenclatureCode, NomenclatureImport } from '@/shared/tnved/nomenclature'
import { DirectoryImportResult } from '../common/directoryTransfer'
import { KzImport } from '../kzList/kzListApi'

// Полная номенклатура ТН ВЭД ЕАЭС со ставками пошлины ЕТТ (источник — выгрузка TWS.BY).

type NomenclaturePage = { items: NomenclatureCode[]; total: number; import: NomenclatureImport | null; kzImport: KzImport | null }

type ImportError = Error & { rows?: DirectoryImportResult['errors'] }

export const nomenclatureApi = {
  list: (params: { q: string; offset: number; limit: number }) => request<NomenclaturePage>(`/api/tnved-nomenclature?${new URLSearchParams({ q: params.q, offset: String(params.offset), limit: String(params.limit) })}`),
  // Вставленные коды: найденные в номенклатуре (остальных в items нет) и откуда она загружена.
  lookup: (codes: string[]) => request<Omit<NomenclaturePage, 'total'>>('/api/tnved-nomenclature/lookup', { method: 'POST', body: JSON.stringify({ codes }) }),

  // При ошибках в строках сервер ничего не сохраняет и возвращает их в error.rows.
  async upload(file: File): Promise<NomenclatureImport> {
    const form = new FormData()
    form.append('file', file)
    const response = await fetch('/api/tnved-nomenclature/import', { method: 'POST', credentials: 'include', body: form })
    const body = await response.json().catch(() => ({})) as { import?: NomenclatureImport; error?: { message?: string }; result?: { errors?: DirectoryImportResult['errors'] } }
    if (!response.ok || !body.import) {
      const error = new Error(body.error?.message ?? 'Не удалось загрузить файл') as ImportError
      error.rows = body.result?.errors
      throw error
    }
    return body.import
  },
}
