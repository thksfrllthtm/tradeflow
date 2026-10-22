import { describe, expect, it } from 'vitest'
import { collides, jump, newRoad, roadScore, step, TRUCK } from './roadPhysics'

const frames = (state: ReturnType<typeof newRoad>, seconds: number, width = 600) => {
  let current = state
  for (let time = 0; time < seconds; time += 1 / 60) current = step(current, 1 / 60, width, () => 0.5)
  return current
}

describe('roadGame', () => {
  it('пробел на старте запускает игру, грузовик подпрыгивает и возвращается на дорогу', () => {
    const started = jump(newRoad())
    expect(started.status).toBe('running')
    const up = frames(started, 0.2)
    expect(up.y).toBeGreaterThan(40)
    const landed = frames(up, 1)
    expect(landed.y).toBe(0)
  })

  it('в воздухе второй прыжок не срабатывает', () => {
    const inAir = frames(jump(newRoad()), 0.1)
    expect(jump(inAir)).toBe(inAir)
  })

  it('шлагбаум на пути — авария, над шлагбаумом в прыжке — нет', () => {
    expect(collides(0, { x: TRUCK.x + 10 })).toBe(true)
    expect(collides(60, { x: TRUCK.x + 10 })).toBe(false)
    expect(collides(0, { x: TRUCK.x + TRUCK.width + 20 })).toBe(false)
  })

  it('без прыжков грузовик рано или поздно врезается; очки растут с расстоянием', () => {
    const over = frames({ ...newRoad(), status: 'running' }, 10)
    expect(over.status).toBe('over')
    expect(roadScore(over)).toBeGreaterThan(0)
  })

  it('после аварии пробел начинает заново', () => {
    const over = { ...newRoad(), status: 'over' as const, distance: 5000 }
    const again = jump(over)
    expect(again.status).toBe('running')
    expect(again.distance).toBe(0)
  })
})
