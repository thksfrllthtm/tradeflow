import { Dispatch, SetStateAction } from 'react'
import { GlovesParseRow } from '../types'

// Этап 1: назначение колонки полю строки (одна колонка — одно поле) и выбор строк Excel для импорта.
export function columnMappingActions<M extends Record<string, string>>(setMapping: Dispatch<SetStateAction<M>>, setRawRows: Dispatch<SetStateAction<GlovesParseRow[]>>) {
  return {
    setMappingForHeader(header: string, field: keyof M | '') {
      setMapping((current) => {
        const next = { ...current }
        ;(Object.keys(next) as (keyof M)[]).forEach((key) => {
          if (next[key] === header) next[key] = '' as M[keyof M]
        })
        if (field) next[field] = header as M[keyof M]
        return next
      })
    },

    setRawRowIncluded(sourceRow: number, included: boolean) {
      setRawRows((current) => current.map((row) => row.sourceRow === sourceRow ? { ...row, included } : row))
    },
  }
}
