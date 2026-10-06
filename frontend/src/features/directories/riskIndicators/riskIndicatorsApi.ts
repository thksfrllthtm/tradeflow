import { request } from '@/shared/api/client'
import { DirectoryImportResult } from '../common/directoryTransfer'

// Стоимостные индикаторы риска (СИР) КГД: загруженные профили риска, загрузка файла «…_прил CN.xlsx»
// и удаление профиля (администратор).

export type RiskProfile = { id: number; number: string; fileName: string; codeCount: number; groups: string[]; importedAt: string; importedByName: string }
export type RiskImportResult = { profile: RiskProfile; replaced: boolean; defaultCountryCodes: number }
export type RiskImportError = Error & { result?: { errors?: DirectoryImportResult['errors'] } }

export const riskIndicatorsApi = {
  list: () => request<{ profiles: RiskProfile[] }>('/api/risk-indicators').then((body) => body.profiles),
  async upload(file: File): Promise<RiskImportResult> {
    const form = new FormData()
    form.append('file', file)
    const response = await fetch('/api/risk-indicators/import', { method: 'POST', credentials: 'include', body: form })
    if (!response.ok) {
      const body = await response.json().catch(() => ({})) as { error?: { message?: string }; result?: RiskImportError['result'] }
      const error = new Error(body.error?.message ?? 'Не удалось загрузить файл') as RiskImportError
      error.result = body.result
      throw error
    }
    return response.json() as Promise<RiskImportResult>
  },
  remove: (id: number) => request<void>(`/api/risk-indicators/${id}`, { method: 'DELETE' }),
}
