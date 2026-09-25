import { photosOf } from '../../shared/lib/itemPhotos'
import { GlovesCalculationResult, GlovesItemInput, GlovesParams, GlovesSourceSnapshot } from '../../shared/types'
import { DraftPatch } from '../goodsEstimatesApi'

// Что изменилось в смете с последнего сохранения на сервер и какими запросами это отправить.

// Фото уходят порциями: большой запрос через Vercel может не пройти.
const MAX_REQUEST_CHARS = 3_000_000

export type DraftState = { params: GlovesParams; items: GlovesItemInput[]; result: GlovesCalculationResult | null; source: GlovesSourceSnapshot | null }

// Что лежит на сервере: JSON каждого поля ввода, JSON результата и фото строк.
export type Baseline = { input: Record<string, string>; result: string; photos: Map<string, string[]> }

const withoutPhotos = (items: GlovesItemInput[]) => items.map(({ photoUrl: _photoUrl, photoUrls: _photoUrls, ...item }) => item)

function inputFields({ params, items, source }: DraftState): Record<string, unknown> {
  return { ...params, items: withoutPhotos(items), source }
}

export function baselineOf(state: DraftState): Baseline {
  const input: Record<string, string> = {}
  for (const [key, value] of Object.entries(inputFields(state))) input[key] = JSON.stringify(value ?? null)
  return { input, result: JSON.stringify(state.result ?? null), photos: new Map(state.items.map((item) => [item.id ?? '', photosOf(item)])) }
}

const samePhotos = (a: string[] = [], b: string[] = []) => a.length === b.length && a.every((photo, index) => photo === b[index])

// Отличия от сохранённого: поля ввода, результат и фото по строкам. null — отличий нет.
export function changesSince(saved: Baseline, state: DraftState) {
  const next = baselineOf(state)
  const input: Record<string, unknown> = {}
  const fields = inputFields(state)
  for (const [key, json] of Object.entries(next.input)) if (saved.input[key] !== json) input[key] = fields[key] ?? null
  const result = next.result !== saved.result ? state.result : undefined
  const photos: Record<string, string[]> = {}
  next.photos.forEach((list, id) => { if (id && !samePhotos(saved.photos.get(id), list)) photos[id] = list })
  if (Object.keys(input).length === 0 && result === undefined && Object.keys(photos).length === 0) return null
  return { input, result, photos, next }
}

// Запросы: первый — поля, результат и сколько влезет фото; дальше — остальные фото порциями.
export function requestsFor(changes: NonNullable<ReturnType<typeof changesSince>>, maxRequestChars = MAX_REQUEST_CHARS): DraftPatch[] {
  const first: DraftPatch = {}
  if (Object.keys(changes.input).length > 0) first.input = changes.input
  if (changes.result !== undefined) first.result = changes.result
  const requests: DraftPatch[] = [first]
  let size = JSON.stringify(first).length
  for (const [id, list] of Object.entries(changes.photos)) {
    const photoSize = list.reduce((sum, photo) => sum + photo.length, id.length + 16)
    let current = requests[requests.length - 1]
    if (size + photoSize > maxRequestChars && (current.input || current.result !== undefined || current.photos)) {
      current = {}
      requests.push(current)
      size = 0
    }
    current.photos = { ...(current.photos ?? {}), [id]: list }
    size += photoSize
  }
  return requests.filter((request) => request.input || request.result !== undefined || request.photos)
}
