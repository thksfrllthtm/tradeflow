// Картинки из буфера обмена (Ctrl+V). Excel и другие таблицы кладут вместе с текстом скопированных ячеек
// их картинку-отрисовку (мелкую, размером с ячейку): если в буфере есть текст — скопирован текст, а не фото.

type Clipboard = Pick<DataTransfer, 'getData' | 'files'>

export function clipboardImages(data: Clipboard | null | undefined): File[] {
  if (!data || clipboardText(data) !== '') return []
  return [...data.files].filter((file) => file.type.startsWith('image/'))
}

// Текст из буфера одной строкой: ячейки и строки таблицы — через пробел.
export function clipboardText(data: Clipboard | null | undefined): string {
  return (data?.getData('text/plain') ?? '').replace(/\s+/g, ' ').trim()
}
