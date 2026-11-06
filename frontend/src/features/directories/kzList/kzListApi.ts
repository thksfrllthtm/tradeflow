import { request } from '@/shared/api/client'
import { saveResponse } from '@/shared/api/download'
import { DirectoryImportResult } from '../common/directoryTransfer'
import { KzListExtract } from './kzListPdf'

// «Перечень РК» (администратор): Excel из прочитанного PDF и загрузка перечня из PDF или Excel.

export type KzImport = { source: 'pdf' | 'excel'; fileName: string; rowCount: number; importedAt: string; importedByName: string }
export type KzImportSummary = { active: number; notYet: number; expired: number; notInNomenclature: string[] }
export type KzImportResult = { import: KzImport; summary: KzImportSummary }
export type KzListError = Error & { result?: { errors?: DirectoryImportResult['errors'] } }

async function failure(response: Response, fallback: string): Promise<KzListError> {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string }; result?: KzListError['result'] }
  const error = new Error(body.error?.message ?? fallback) as KzListError
  error.result = body.result
  return error
}

export const kzListApi = {
  // Excel для проверки: сервер считает даты действия по сноскам и добавляет ставку ЕТТ.
  async excel(extract: KzListExtract) {
    const response = await fetch('/api/tnved-kz-rates/excel', { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(extract) })
    if (!response.ok) throw await failure(response, 'Не удалось сделать Excel')
    await saveResponse(response, 'Перечень РК.xlsx')
  },
  importPdf: (extract: KzListExtract) => request<KzImportResult>('/api/tnved-kz-rates/import-pdf', { method: 'POST', body: JSON.stringify(extract) }),
  async importExcel(file: File): Promise<KzImportResult> {
    const form = new FormData()
    form.append('file', file)
    const response = await fetch('/api/tnved-kz-rates/import', { method: 'POST', credentials: 'include', body: form })
    if (!response.ok) throw await failure(response, 'Не удалось загрузить файл')
    return response.json() as Promise<KzImportResult>
  },
}
