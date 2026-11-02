import { Declaration } from './declarationPdf'

export type CheckStatus = 'ok' | 'error' | 'warn' | 'skip'
export type Check = { label: string; invoice: string; declaration: string; status: CheckStatus; note?: string }
export type RowCheck = {
  no: number
  name: string
  codeInvoice: string
  codeDeclaration: string
  declarationNo: number
  declarationName: string
  excelRow: number
  file: string
  status: CheckStatus
  checks: Check[]
}
export type Report = {
  invoiceNumber: string
  invoiceRows: number
  declarationItems: number
  totals: Check[]
  details: Check[]
  rows: RowCheck[]
  problems: string[]
  notes: string[]
  errors: number
  warnings: number
}

// Сверка инвойса с экспортной декларацией: инвойс Excel уходит файлом, декларации — уже разобранные
// браузером из PDF.
export const exportCheckApi = {
  check: async (invoice: File, declarations: Declaration[]): Promise<Report> => {
    const form = new FormData()
    form.append('invoice', invoice)
    form.append('declarations', JSON.stringify(declarations))
    const response = await fetch('/api/export-check', { method: 'POST', credentials: 'include', body: form })
    const body = await response.json().catch(() => ({})) as Report & { error?: { message?: string } }
    if (!response.ok) throw new Error(body.error?.message ?? 'Не удалось выполнить сверку')
    return body
  },
}
