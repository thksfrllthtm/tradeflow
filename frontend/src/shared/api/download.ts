// Скачивает файл с сервера: имя приходит в Content-Disposition (filename*=UTF-8''…), иначе fallbackName.
export async function downloadFile(path: string, fallbackName: string, errorText = 'Не удалось скачать файл') {
  const response = await fetch(path, { credentials: 'include' })
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: { message?: string } }
    throw new Error(body.error?.message ?? errorText)
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
