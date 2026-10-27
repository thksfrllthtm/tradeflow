import { useEffect, useRef, useState } from 'react'
import { BARRIER, GROUND, jump, newRoad, RoadState, roadScore, step, TRUCK } from './roadPhysics'

const HEIGHT = 150
const RECORD_KEY = 'tf-road-game-record'

// Поле ввода в фокусе — пробел печатает, а не прыгает.
const typing = (target: EventTarget | null) => target instanceof HTMLElement && (target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName))

function readRecord() {
  try { return Number(localStorage.getItem(RECORD_KEY)) || 0 } catch { return 0 }
}
function saveRecord(value: number) {
  try { localStorage.setItem(RECORD_KEY, String(value)) } catch { /* браузер не даёт хранить — рекорд только до перезагрузки */ }
}

// Игра, пока ИИ ищет код: грузовик с контейнером прыгает через шлагбаумы таможни (как динозаврик Chrome).
// Пробел, ↑ или щелчок по полю — прыжок. Всё рисуется в браузере, сервер не нагружается.
export function RoadGame({ onClose }: { onClose: () => void }) {
  const canvas = useRef<HTMLCanvasElement>(null)
  const state = useRef<RoadState>(newRoad())
  const [record, setRecord] = useState(readRecord)
  const recordRef = useRef(record)

  const press = () => { state.current = jump(state.current) }

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.code !== 'Space' && event.key !== 'ArrowUp') return
      if (typing(event.target) || event.repeat) return
      event.preventDefault()
      press()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  useEffect(() => {
    const element = canvas.current
    const context = element?.getContext('2d')
    if (!element || !context) return
    let frame = 0
    let last = performance.now()
    const loop = (now: number) => {
      const dt = Math.min(0.05, (now - last) / 1000)
      last = now
      // Размер поля — по ширине карточки, чётко на экранах с высокой плотностью точек.
      const width = element.clientWidth
      const ratio = window.devicePixelRatio || 1
      if (element.width !== Math.round(width * ratio)) { element.width = Math.round(width * ratio); element.height = Math.round(HEIGHT * ratio) }
      context.setTransform(ratio, 0, 0, ratio, 0, 0)

      const before = state.current.status
      state.current = step(state.current, dt, width)
      if (before === 'running' && state.current.status === 'over') {
        const score = roadScore(state.current)
        if (score > recordRef.current) { recordRef.current = score; saveRecord(score); setRecord(score) }
      }
      draw(context, width, state.current, recordRef.current)
      frame = requestAnimationFrame(loop)
    }
    frame = requestAnimationFrame(loop)
    return () => cancelAnimationFrame(frame)
  }, [])

  return <section className="rc-game" aria-label="Игра, пока ИИ ищет код">
    <div className="rc-game-head">
      <span>Пока ИИ ищет код — сыграйте: <kbd>Пробел</kbd> или щелчок — прыжок</span>
      {record > 0 && <b>Рекорд {record}</b>}
      <button type="button" className="rc-game-close" onClick={onClose} aria-label="Закрыть игру" title="Закрыть игру">
        <svg width="12" height="12" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"><path d="M4 4l8 8M12 4l-8 8" /></svg>
      </button>
    </div>
    <canvas ref={canvas} height={HEIGHT} onPointerDown={(event) => { event.preventDefault(); press() }}
      role="img" aria-label="Грузовик с контейнером прыгает через шлагбаумы" />
  </section>
}

// Рисование кадра: дорога с разметкой, шлагбаумы (столб и полосатая стрела), грузовик с контейнером, очки.
function draw(context: CanvasRenderingContext2D, width: number, state: RoadState, record: number) {
  const ink = '#1d2a22'
  const green = '#1e5b3c'
  const muted = '#8a948e'
  context.clearRect(0, 0, width, HEIGHT)

  // Дорога и бегущая разметка.
  context.fillStyle = '#d7ddd8'
  context.fillRect(0, GROUND, width, 2)
  context.fillStyle = '#c3cbc5'
  const dash = 40
  for (let x = -(state.distance % dash); x < width; x += dash) context.fillRect(x, GROUND + 12, 18, 2)

  // Шлагбаумы: столб и красно-белая стрела.
  for (const barrier of state.barriers) {
    const top = GROUND - BARRIER.height
    context.fillStyle = ink
    context.fillRect(barrier.x, top, 5, BARRIER.height)
    for (let index = 0; index < 5; index++) {
      context.fillStyle = index % 2 === 0 ? '#c8372d' : '#ffffff'
      context.fillRect(barrier.x + 5 + index * 5, top + 6, 5, 6)
    }
    context.strokeStyle = '#c8372d'
    context.lineWidth = 1
    context.strokeRect(barrier.x + 5.5, top + 6.5, 24, 5)
  }

  // Грузовик: контейнер, кабина, окно, колёса.
  const bottom = GROUND - state.y
  const top = bottom - TRUCK.height
  context.fillStyle = green
  context.fillRect(TRUCK.x, top, 34, TRUCK.height - 8)
  context.fillStyle = 'rgba(255,255,255,.35)'
  for (let x = TRUCK.x + 6; x < TRUCK.x + 32; x += 7) context.fillRect(x, top + 3, 2, TRUCK.height - 14)
  context.fillStyle = ink
  context.fillRect(TRUCK.x + 36, top + 8, 16, TRUCK.height - 16)
  context.fillStyle = '#cfe3d6'
  context.fillRect(TRUCK.x + 43, top + 11, 7, 6)
  context.fillStyle = ink
  for (const x of [TRUCK.x + 9, TRUCK.x + 26, TRUCK.x + 44]) { context.beginPath(); context.arc(x, bottom - 4, 4, 0, Math.PI * 2); context.fill() }

  // Очки и подсказки.
  context.font = '600 13px "JetBrains Mono", monospace'
  context.textAlign = 'right'
  context.fillStyle = ink
  context.fillText(String(roadScore(state)).padStart(5, '0'), width - 12, 20)
  context.textAlign = 'center'
  if (state.status === 'ready') {
    context.fillStyle = muted
    context.fillText('Пробел — поехали', width / 2, 56)
  }
  if (state.status === 'over') {
    context.fillStyle = ink
    context.fillText(roadScore(state) >= record && record > 0 ? 'Новый рекорд!' : 'Шлагбаум!', width / 2, 48)
    context.fillStyle = muted
    context.fillText('Пробел — заново', width / 2, 68)
  }
}
