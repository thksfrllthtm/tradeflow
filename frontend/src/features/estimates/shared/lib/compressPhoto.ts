// Фото из Excel поставщика бывают по несколько мегабайт. В смете они хранятся в базе
// и в IndexedDB и уходят на сервер при каждом сохранении, поэтому уменьшаем их до
// размера, которого достаточно для просмотра и выгрузки: сторона до 1280 px, JPEG.
const MAX_SIDE = 1280
const JPEG_QUALITY = 0.82
const SMALL_ENOUGH_BYTES = 300 * 1024

type Photo = { data: string; mimeType: string }

// Возвращает сжатое фото или исходное, если оно уже маленькое, сжатие не помогло
// или браузер не смог его прочитать.
export async function compressPhoto(photo: Photo): Promise<Photo> {
  if (photo.data.length * 0.75 <= SMALL_ENOUGH_BYTES || typeof createImageBitmap === 'undefined') return photo
  try {
    const source = base64ToBlob(photo.data, photo.mimeType)
    const bitmap = await createImageBitmap(source)
    const scale = Math.min(1, MAX_SIDE / Math.max(bitmap.width, bitmap.height))
    const width = Math.max(1, Math.round(bitmap.width * scale))
    const height = Math.max(1, Math.round(bitmap.height * scale))
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const context = canvas.getContext('2d')
    if (!context) {
      bitmap.close()
      return photo
    }
    // У JPEG нет прозрачности: прозрачный фон PNG иначе станет чёрным.
    context.fillStyle = '#ffffff'
    context.fillRect(0, 0, width, height)
    context.drawImage(bitmap, 0, 0, width, height)
    bitmap.close()
    const compressed = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', JPEG_QUALITY))
    if (!compressed || compressed.size >= source.size) return photo
    return { data: await blobToBase64(compressed), mimeType: 'image/jpeg' }
  } catch {
    return photo
  }
}

function base64ToBlob(data: string, mimeType: string): Blob {
  const binary = atob(data)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index)
  return new Blob([bytes], { type: mimeType })
}

function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const dataUrl = typeof reader.result === 'string' ? reader.result : ''
      resolve(dataUrl.slice(dataUrl.indexOf(',') + 1))
    }
    reader.onerror = () => reject(reader.error ?? new Error('Не удалось прочитать фото'))
    reader.readAsDataURL(blob)
  })
}
