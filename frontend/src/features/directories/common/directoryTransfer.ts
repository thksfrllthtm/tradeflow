// Выгрузка справочника в Excel и загрузка из Excel (только администратор).

export type DirectoryImportResult = {
  created: number
  updated: number
  skipped: number
  errors: { row: number; message: string }[]
}

type ImportError = Error & { result?: DirectoryImportResult }

// Скачивает XLSX: имя файла приходит от сервера («Справочник ТН ВЭД 2026-09-26.xlsx»).
export async function downloadDirectory(path: string, fallbackName: string) {
  const response = await fetch(path, { credentials: 'include' })
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: { message?: string } }
    throw new Error(body.error?.message ?? 'Не удалось выгрузить справочник')
  }
  const disposition = response.headers.get('Content-Disposition') ?? ''
  const encoded = /filename\*=UTF-8''([^;]+)/i.exec(disposition)?.[1]
  const name = encoded ? decodeURIComponent(encoded) : fallbackName
  const url = URL.createObjectURL(await response.blob())
  const link = document.createElement('a')
  link.href = url
  link.download = name
  link.click()
  URL.revokeObjectURL(url)
}

// Загружает XLSX. При ошибках в строках сервер ничего не сохраняет и возвращает их список
// в error.result.
export async function uploadDirectory(path: string, file: File, update: boolean): Promise<DirectoryImportResult> {
  const form = new FormData()
  form.append('file', file)
  const response = await fetch(`${path}?update=${update}`, { method: 'POST', credentials: 'include', body: form })
  const body = await response.json().catch(() => ({})) as DirectoryImportResult & { error?: { message?: string }; result?: DirectoryImportResult }
  if (!response.ok) {
    const error = new Error(body.error?.message ?? 'Не удалось загрузить файл') as ImportError
    error.result = body.result
    throw error
  }
  return body
}
