import { describe, expect, it } from 'vitest'
import { GoodsItemInput } from '@/shared/types/goods'
import { makeMainRowPhoto, moveRowPhoto, removeRowPhoto } from './rowPhotos'

const row = (id: string, photos: string[]) => ({ id, photoUrl: photos[0] ?? '', photoUrls: photos }) as unknown as GoodsItemInput

describe('фото строк', () => {
  const items = [row('a', ['p1', 'p2', 'p3']), row('b', [])]

  it('удаляет фото, главное — следующее', () => {
    const [a] = removeRowPhoto(items, 'a', 0)
    expect(a.photoUrls).toEqual(['p2', 'p3'])
    expect(a.photoUrl).toBe('p2')
  })

  it('делает фото главным', () => {
    const [a] = makeMainRowPhoto(items, 'a', 2)
    expect(a.photoUrls).toEqual(['p3', 'p1', 'p2'])
    expect(a.photoUrl).toBe('p3')
  })

  it('переносит фото в другую строку', () => {
    const moved = moveRowPhoto(items, 'a', 1, 'b')
    expect(moved?.[0].photoUrls).toEqual(['p1', 'p3'])
    expect(moved?.[1].photoUrls).toEqual(['p2'])
    expect(moved?.[1].photoUrl).toBe('p2')
    expect(moveRowPhoto(items, 'a', 1, 'a')).toBeNull()
    expect(moveRowPhoto(items, 'a', 9, 'b')).toBeNull()
  })
})
