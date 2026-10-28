import { describe, expect, it } from 'vitest'
import { estimateDisplayTitle, estimateName, estimateSubtitle } from './estimateTitle'

describe('estimateDisplayTitle', () => {
  it('shows the date in the name as dd.mm.yyyy', () => {
    expect(estimateDisplayTitle('Смета товаров · 2026-10-14')).toBe('Смета товаров · 14.10.2026')
    expect(estimateDisplayTitle('Смета №78')).toBe('Смета №78')
  })
})

describe('estimateName и estimateSubtitle', () => {
  it('стандартное название — клиент, без клиента — номер; своё — как есть', () => {
    const client = { id: 7, name: 'Смета товаров · 2026-10-05', client: 'ТОО «Стройдом»' }
    expect(estimateName(client)).toBe('ТОО «Стройдом»')
    expect(estimateSubtitle(client)).toBe('Смета №7')
    const bare = { id: 8, name: 'Смета товаров', client: ' ' }
    expect(estimateName(bare)).toBe('Смета №8')
    expect(estimateSubtitle(bare)).toBe('Без клиента')
    const own = { id: 9, name: 'Керамика, партия 460', client: 'ИП Ахметов' }
    expect(estimateName(own)).toBe('Керамика, партия 460')
    expect(estimateSubtitle(own)).toBe('ИП Ахметов')
  })
})
