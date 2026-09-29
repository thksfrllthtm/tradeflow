import { request } from '@/shared/api/client'
import { DirectoryImportResult } from '../directoryTransfer'
import { KzImport } from '../kzList/kzListApi'

// Полная номенклатура ТН ВЭД ЕАЭС со ставками пошлины ЕТТ (источник — выгрузка TWS.BY).

// combined_plus — «процент плюс сумма за единицу», такого типа в справочнике нет.
export type NomenclatureDuty = { type: 'unknown' | 'percentage' | 'specific' | 'combined_minimum' | 'combined_plus'; percent: string; amount: string; currency: string; unit: string }
// detailsUrl — страница кода в источнике («Подробнее» в выгрузке TWS), пусто, если ссылки нет.
// Ставка по перечню РК (ниже ЕТТ) и её действие на сегодня; other — форма, которой нет в справочнике.
export type KzRate = { rateText: string; duty: Omit<NomenclatureDuty, 'type'> & { type: 'percentage' | 'specific' | 'combined_minimum' | 'other' }; validFrom: string; validTo: string; active: boolean; status: string; notesText: string }
export type NomenclatureCode = { code: string; name: string; tariffText: string; duty: NomenclatureDuty; vatPercent: string; detailsUrl: string; inDirectory: boolean; kz: KzRate | null }
export type NomenclatureImport = { source: string; fileName: string; actualDate: string; codeCount: number; addedCount: number; removedCount: number; changedCount: number; importedAt: string; importedByName: string }
export type NomenclaturePage = { items: NomenclatureCode[]; total: number; import: NomenclatureImport | null; kzImport: KzImport | null }

type ImportError = Error & { rows?: DirectoryImportResult['errors'] }

export const nomenclatureApi = {
  list: (params: { q: string; offset: number; limit: number }) => request<NomenclaturePage>(`/api/tnved-nomenclature?${new URLSearchParams({ q: params.q, offset: String(params.offset), limit: String(params.limit) })}`),

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
