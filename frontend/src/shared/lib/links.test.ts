import { describe, expect, it } from 'vitest'
import { splitLinks } from './links'

describe('splitLinks', () => {
  it('выделяет адреса и оставляет точку после адреса тексту', () => {
    expect(splitLinks('Акт: https://adilet.zan.kz/rus/docs/H10T0000299. Нужна декларация')).toEqual([
      { text: 'Акт: ' },
      { text: 'https://adilet.zan.kz/rus/docs/H10T0000299', url: 'https://adilet.zan.kz/rus/docs/H10T0000299' },
      { text: '. Нужна декларация' },
    ])
  })

  it('несколько адресов, адрес в скобках и в кавычках-ёлочках', () => {
    const parts = splitLinks('(см. http://a.kz/x) и «https://b.kz/y?z=1»')
    expect(parts.filter((part) => part.url).map((part) => part.url)).toEqual(['http://a.kz/x', 'https://b.kz/y?z=1'])
    expect(parts.map((part) => part.text).join('')).toBe('(см. http://a.kz/x) и «https://b.kz/y?z=1»')
  })

  it('текст без ссылок и javascript: — без ссылок', () => {
    expect(splitLinks('javascript:alert(1) просто текст')).toEqual([{ text: 'javascript:alert(1) просто текст' }])
  })
})
