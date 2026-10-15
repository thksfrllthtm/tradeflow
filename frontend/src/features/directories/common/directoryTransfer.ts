// Загрузка справочника из Excel (только администратор); выгрузка — shared/api/download.

export type DirectoryImportResult = {
  created: number
  updated: number
  skipped: number
  errors: { row: number; message: string }[]
}

type ImportError = Error & { result?: DirectoryImportResult }

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
