// Фото из файла клиента уменьшается для ИИ: 512 px достаточно. В базе фото не хранятся.
export const AI_PHOTO_SIDE = 512

export function toDataUrl(image?: { data: string; mimeType: string }) {
  return image?.data ? `data:${image.mimeType};base64,${image.data}` : ''
}

// JPEG не больше maxSide по длинной стороне; если браузер не смог прочитать фото — ''.
export async function resizePhoto(dataUrl: string, maxSide: number, quality = 0.8): Promise<string> {
  if (!dataUrl || typeof createImageBitmap === 'undefined') return ''
  try {
    const blob = await (await fetch(dataUrl)).blob()
    const bitmap = await createImageBitmap(blob)
    const scale = Math.min(1, maxSide / Math.max(bitmap.width, bitmap.height))
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(bitmap.width * scale))
    canvas.height = Math.max(1, Math.round(bitmap.height * scale))
    const context = canvas.getContext('2d')
    if (!context) { bitmap.close(); return '' }
    context.fillStyle = '#ffffff'
    context.fillRect(0, 0, canvas.width, canvas.height)
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
    bitmap.close()
    return canvas.toDataURL('image/jpeg', quality)
  } catch {
    return ''
  }
}