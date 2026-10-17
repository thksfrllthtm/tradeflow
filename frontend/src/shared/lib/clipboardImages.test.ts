import { describe, expect, it } from 'vitest'
import { clipboardImages, clipboardText } from './clipboardImages'

const image = new File(['x'], 'image.png', { type: 'image/png' })
const clipboard = (text: string, files: File[]) => ({ getData: (type: string) => (type === 'text/plain' ? text : ''), files: files as unknown as FileList })

describe('clipboardImages', () => {
  it('ячейки Excel — текст, а не их картинка-отрисовка', () => {
    const cells = clipboard('塑料+不锈钢\t气泵\r\n', [image])
    expect(clipboardImages(cells)).toEqual([])
    expect(clipboardText(cells)).toBe('塑料+不锈钢 气泵')
  })

  it('скопированная картинка или скриншот без текста — фото', () => {
    expect(clipboardImages(clipboard('', [image]))).toEqual([image])
    expect(clipboardImages(clipboard('  \n', [image, new File(['x'], 'a.txt', { type: 'text/plain' })]))).toEqual([image])
    expect(clipboardImages(null)).toEqual([])
  })
})
