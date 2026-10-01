// Уменьшение фото в браузере перед отправкой на сервер: ИИ и базе решений не нужен оригинал
// в несколько мегабайт. Возвращает JPEG data URL; если картинку не прочитать — исходное значение.

export async function resizePhoto(source: string, maxSide = 1024, quality = 0.85): Promise<string> {
  try {
    const image = await loadImage(source)
    const scale = Math.min(1, maxSide / Math.max(image.naturalWidth, image.naturalHeight))
    const width = Math.max(1, Math.round(image.naturalWidth * scale))
    const height = Math.max(1, Math.round(image.naturalHeight * scale))
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const context = canvas.getContext('2d')
    if (!context) return source
    // Прозрачный PNG на белом фоне, а не на чёрном.
    context.fillStyle = '#fff'
    context.fillRect(0, 0, width, height)
    context.drawImage(image, 0, 0, width, height)
    return canvas.toDataURL('image/jpeg', quality)
  } catch {
    return source
  }
}

function loadImage(source: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('Не удалось прочитать фото'))
    image.src = source
  })
}
