// «Грузовик и шлагбаумы» — игра, пока ИИ ищет код (как динозаврик Chrome без интернета). Здесь — только
// движение и столкновения, без рисования: так их можно проверить тестом. Всё в браузере, сервер не нагружается.

export const GROUND = 120 // уровень дороги, px от верха поля
export const TRUCK = { x: 40, width: 52, height: 30 }
export const BARRIER = { width: 30, height: 30 }

const GRAVITY = 2200 // px/с²
const JUMP = 560 // px/с — скорость отрыва: прыжок ~70 px, в поле 150 px
const START_SPEED = 260 // px/с
const MAX_SPEED = 620
const ACCELERATION = 9 // px/с за секунду

export type Barrier = { x: number }
export type RoadState = {
  status: 'ready' | 'running' | 'over'
  y: number // высота грузовика над дорогой (0 — на дороге)
  vy: number
  speed: number
  distance: number
  barriers: Barrier[]
  nextGap: number // через сколько px появится следующий шлагбаум
}

export const newRoad = (): RoadState => ({ status: 'ready', y: 0, vy: 0, speed: START_SPEED, distance: 0, barriers: [], nextGap: 360 })

// Очки — пройденное расстояние, по 10 px за очко.
export const roadScore = (state: RoadState) => Math.floor(state.distance / 10)

// Прыжок: на старте — поехали, после аварии — заново, в пути — только с дороги (без двойного прыжка).
export function jump(state: RoadState): RoadState {
  if (state.status !== 'running') return { ...newRoad(), status: 'running', vy: JUMP }
  return state.y === 0 ? { ...state, vy: JUMP } : state
}

// Касание грузовика и шлагбаума (прямоугольники, с небольшим запасом — чтобы задевание краем прощалось).
export function collides(truckY: number, barrier: Barrier) {
  const slack = 4
  const truckLeft = TRUCK.x + slack
  const truckRight = TRUCK.x + TRUCK.width - slack
  const truckBottom = truckY // высота низа грузовика над дорогой
  return truckRight > barrier.x + slack && truckLeft < barrier.x + BARRIER.width - slack && truckBottom < BARRIER.height - slack
}

// Шаг игры на dt секунд. width — ширина поля: шлагбаумы появляются у правого края. random — для теста.
export function step(state: RoadState, dt: number, width: number, random = Math.random): RoadState {
  if (state.status !== 'running') return state
  const speed = Math.min(MAX_SPEED, state.speed + ACCELERATION * dt)
  const moved = speed * dt
  let vy = state.vy - GRAVITY * dt
  let y = state.y + state.vy * dt
  if (y <= 0) { y = 0; vy = 0 }

  const barriers = state.barriers.map((barrier) => ({ x: barrier.x - moved })).filter((barrier) => barrier.x > -BARRIER.width)
  let nextGap = state.nextGap - moved
  if (nextGap <= 0) {
    barriers.push({ x: width })
    // Промежуток растёт со скоростью: прыжок всегда успевает.
    nextGap = speed * (0.9 + random() * 0.9)
  }
  const crashed = barriers.some((barrier) => collides(y, barrier))
  return { status: crashed ? 'over' : 'running', y, vy, speed, distance: state.distance + moved, barriers, nextGap }
}
