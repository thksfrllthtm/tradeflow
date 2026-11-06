// Скачивает файл с сервера: имя приходит в Content-Disposition (filename*=UTF-8''…), иначе fallbackName.
export async function downloadFile(path: string, fallbackName: string, errorText = 'Не удалось скачать файл') {
  const response = await fetch(path, { credentials: 'include' })
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: { message?: string } }
    throw new Error(body.error?.message ?? errorText)
  }
  await saveResponse(response, fallbackName)
}

// Сохраняет ответ сервера файлом с именем из Content-Disposition (нет — fallbackName).
export async function saveResponse(response: Response, fallbackName: string) {
  const encoded = /filename\*=UTF-8''([^;]+)/i.exec(response.headers.get('Content-Disposition') ?? '')?.[1]
  saveBlob(await response.blob(), encoded ? decodeURIComponent(encoded) : fallbackName)
}

// Отдаёт браузеру blob как скачиваемый файл.
export function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = name
  link.click()
  URL.revokeObjectURL(url)
}
