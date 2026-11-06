import { useEffect, useRef, useState } from 'react'
import { GoodsParams } from '@/shared/types/goods'
import { goodsEstimatesApi } from '../goodsEstimatesApi'
import { requestErrorMessage } from '../lib/requestErrorMessage'

type Options = {
  params: GoodsParams
  // Смета загружена: до этого курс не подставляем.
  ready: boolean
  setParam: (field: keyof GoodsParams, value: string) => void
  setError: (message: string) => void
}

// Курс USD НБРК на дату расчёта: загрузка по кнопке «НБРК» и автоматическая подстановка.
export function useUsdRate({ params, ready, setParam, setError }: Options) {
  const [loading, setLoading] = useState(false)
  const [notice, setNotice] = useState('')
  const autoRequestedRef = useRef(false)
  // Дата, на которую курс взят из НБРК; пусто — курс введён вручную или его нет.
  const nbkRateDateRef = useRef('')
  const requestRef = useRef(0)

  // quiet — автоматическая загрузка: ошибку не показываем, курс можно взять кнопкой «НБРК» или ввести.
  async function load(quiet = false) {
    const date = params.assessmentDate
    if (!date) {
      if (!quiet) setError('Сначала укажите дату расчёта')
      return
    }
    // Ответ на устаревший запрос (дату успели сменить) не применяется.
    const request = ++requestRef.current
    setLoading(true)
    if (!quiet) setError('')
    try {
      const rate = await goodsEstimatesApi.usdRate(date)
      if (request !== requestRef.current) return
      setParam('usdKzt', rate.rate)
      nbkRateDateRef.current = date
      setNotice(`${rate.source} · ${rate.date}`)
    } catch (reason) {
      if (request !== requestRef.current) return
      if (!quiet) setError(requestErrorMessage(reason))
      setNotice('')
    } finally {
      if (request === requestRef.current) setLoading(false)
    }
  }

  // Курс НБРК подставляется сам: сразу при открытии сметы без курса и после смены даты, если
  // прежний курс тоже был из НБРК. Введённый вручную курс не меняется.
  useEffect(() => {
    if (!ready || !params.assessmentDate) return
    const missing = params.usdKzt.trim() === '' && !autoRequestedRef.current
    const outdated = nbkRateDateRef.current !== '' && nbkRateDateRef.current !== params.assessmentDate
    if (!missing && !outdated) return
    const timer = window.setTimeout(() => {
      autoRequestedRef.current = true
      void load(true)
    }, 400)
    return () => window.clearTimeout(timer)
  }, [ready, params.assessmentDate, params.usdKzt])

  return {
    loading,
    notice,
    load,
    // Поле сметы изменили вручную: дата или курс — подпись источника курса убирается, курс — больше не из НБРК.
    paramChanged(field: keyof GoodsParams) {
      if (field === 'assessmentDate' || field === 'usdKzt') setNotice('')
      if (field === 'usdKzt') nbkRateDateRef.current = ''
    },
    // Смету очистили: курс снова подставится сам.
    reset() {
      autoRequestedRef.current = false
      nbkRateDateRef.current = ''
    },
  }
}
