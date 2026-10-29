import { describe, expect, it } from 'vitest'
import { shortHolder, TrademarkHit, trademarkText } from './trademarks'

const hit: TrademarkHit = {
  regNo: '01595/ТЗ-260822', name: 'SONY', fromLogo: false, hasLogo: false, goods: '', protectedUntil: '2027-06-12',
  holder: 'Компания «Сони Груп Корпорейшн», Sony Group Corporation. Адрес: 1-7-1 Konan, Minato-ku, Tokyo', kind: 'lookalike', found: 'SONV', sameGoods: false, otherGoods: true,
}

describe('trademarks', () => {
  it('убирает адрес и реквизиты правообладателя', () => {
    expect(shortHolder(hit.holder)).toBe('Компания «Сони Груп Корпорейшн», Sony Group Corporation')
    expect(shortHolder('ТОО "JFOOD KAZAKHSTAN" БИН:921040000467 адрес')).toBe('ТОО "JFOOD KAZAKHSTAN"')
  })

  it('пишет, что найдено и где', () => {
    expect(trademarkText(hit)).toBe('ТРОИС: Похоже на знак «SONY» — возможная имитация (№ 01595/ТЗ-260822, Компания «Сони Груп Корпорейшн», Sony Group Corporation, защита до 12.06.2027); в товаре — «SONV». В реестре знак для других товаров')
    expect(trademarkText({ ...hit, kind: 'brand', name: '', fromLogo: true, sameGoods: true, goodsMatch: 'description', otherGoods: false, protectedUntil: undefined, holder: '' })).toBe('ТРОИС: Знак (изображение) (№ 01595/ТЗ-260822); в товаре — «SONV». Та же группа товаров (по описанию в реестре)')
  })
})
