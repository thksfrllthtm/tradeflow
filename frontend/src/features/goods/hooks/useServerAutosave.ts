import { useEffect, useRef } from 'react'
import { GoodsEstimateSummary } from '@/shared/types/goods'
import { goodsEstimatesApi } from '../goodsEstimatesApi'
import { Baseline, baselineOf, changesSince, DraftState, requestsFor } from '../lib/draftPatch'
import { requestErrorMessage } from '../lib/requestErrorMessage'

// Автосохранение сметы на сервер: через паузу после правки отправляются только изменённые
// части — поля параметров, строки (текстом, без фото), результат и фото изменённых строк.
// Всё сравнивается с последней сохранённой на сервере версией; смета перезаписывается.
const SAVE_DELAY_MS = 1500
const RETRY_DELAY_MS = 15000

type Options = {
  estimateId: number
  state: DraftState
  // Черновик загружен с сервера — можно сравнивать и сохранять.
  enabled: boolean
  onSaving: () => void
  onSaved: (summary: GoodsEstimateSummary, saved: DraftState) => void
  onError: (message: string) => void
}

export function useServerAutosave({ estimateId, state, enabled, onSaving, onSaved, onError }: Options) {
  const baselineRef = useRef<Baseline | null>(null)
  const stateRef = useRef(state)
  stateRef.current = state
  const savingRef = useRef<Promise<void> | null>(null)
  const callbacks = useRef({ onSaving, onSaved, onError })
  callbacks.current = { onSaving, onSaved, onError }

  // Сохраняет всё, что отличается от сервера; пока идёт сохранение, следующее ждёт его.
  async function save(): Promise<void> {
    while (savingRef.current) await savingRef.current
    const state = stateRef.current
    const changes = changesSince(baselineRef.current ?? { input: {}, result: '', photos: new Map() }, state)
    if (!changes) return
    const run = (async () => {
      callbacks.current.onSaving()
      try {
        let summary: GoodsEstimateSummary | null = null
        for (const request of requestsFor(changes)) summary = await goodsEstimatesApi.patchDraft(estimateId, request)
        baselineRef.current = changes.next
        if (summary) callbacks.current.onSaved(summary, state)
      } catch (reason) {
        callbacks.current.onError(requestErrorMessage(reason))
        throw reason
      }
    })()
    savingRef.current = run.finally(() => { savingRef.current = null }).catch(() => undefined)
    await run
  }

  useEffect(() => {
    if (!enabled) return
    let retry = 0
    const timer = window.setTimeout(function attempt() {
      save().catch(() => { retry = window.setTimeout(attempt, RETRY_DELAY_MS) })
    }, SAVE_DELAY_MS)
    return () => { window.clearTimeout(timer); window.clearTimeout(retry) }
  }, [enabled, state.params, state.items, state.result, state.source])

  // Переход на другую смету раньше паузы: несохранённое уходит сразу (в браузере оно тоже есть).
  const enabledRef = useRef(enabled)
  enabledRef.current = enabled
  useEffect(() => () => { if (enabledRef.current) void save().catch(() => undefined) }, [])

  return {
    // Что сейчас на сервере: задаётся при загрузке сметы.
    setBaseline: (saved: DraftState) => { baselineRef.current = baselineOf(saved) },
    // Сохранить сразу (кнопка «Сохранить»). Без загруженной с сервера версии уходит всё целиком.
    saveNow: save,
  }
}
