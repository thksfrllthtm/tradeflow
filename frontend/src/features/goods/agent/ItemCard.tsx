import { useEffect, useState } from 'react'
import { decimalText } from '@/shared/lib/rounding'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { formatCode } from '@/shared/tnved/codes'
import { lookupNomenclature } from '../lib/nomenclatureRates'
import { AgentItem } from './agentApi'

// Товар строки: название крупно, остальные названия (китайское, поставщика) — серым ниже, чипы количества и веса.
export function ItemCard({ item, photos, onOpenPhoto, onAddPhoto }: { item: AgentItem; photos: string[]; onOpenPhoto: (index: number) => void; onAddPhoto: () => void }) {
  const unitNet = perUnit(item.netKg || item.grossKg, item.quantity)
  const title = russianPart(item.name) || item.supplierName || 'Без наименования'
  const rest = item.name.slice(russianPart(item.name).length).replace(/^\s*\/\s*/, '')
  const others = [...new Set([rest, item.supplierName, ...(item.otherNames ?? [])].map((name) => name?.trim()).filter((name): name is string => Boolean(name) && name !== title))]
  return <section className="ag-item">
    {photos.length > 0
      ? <div className="ag-item-photos">{photos.slice(0, 3).map((photo, index) => <button key={index} type="button" className="ag-photo" onClick={() => onOpenPhoto(index)} title="Открыть фото крупно" aria-label={`Открыть фото товара ${index + 1} крупно`}><img src={photo} alt="" /></button>)}</div>
      : <button type="button" className="ag-add-photo" onClick={onAddPhoto} title="Фото добавится к строке сметы; агент увидит его при подборе заново">
        <svg width="18" height="18" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"><path d="M8 4.5v7M4.5 8h7" /></svg>
        Добавить фото
      </button>}
    <div className="ag-item-text">
      <strong>{title}</strong>
      {others.map((other) => <span key={other}>{other}</span>)}
      {item.details?.map((detail) => <span key={detail} className="ag-item-detail">{detail}</span>)}
      <div className="ag-chips">
        {item.quantity && <span className="ag-chip">{formatRuNumber(item.quantity)} {item.unit}</span>}
        {unitNet && <span className="ag-chip">{unitNet} кг / {item.unit}</span>}
        {item.pricePerUnit && <span className="ag-chip">{formatRuNumber(item.pricePerUnit)} USD / {item.unit}</span>}
      </div>
    </div>
  </section>
}

// «Код из файла 8305 20 000 0 · пошлина 13%»: ставка — из номенклатуры.
export function FileCode({ code }: { code: string }) {
  const [duty, setDuty] = useState<string | null>(null)
  useEffect(() => {
    let active = true
    void lookupNomenclature([code]).then((found) => {
      if (!active) return
      const official = found?.byCode.get(code)
      setDuty(official ? `пошлина ${official.tariffText || 'не указана'}` : found ? 'нет в номенклатуре' : '')
    }).catch(() => undefined)
    return () => { active = false }
  }, [code])
  return <p className="ag-filecode">Код из файла <b>{formatCode(code)}</b>{duty && <> · {duty}</>}</p>
}

function perUnit(total: string, quantity: string) {
  const value = Number(String(total).replace(',', '.'))
  const count = Number(String(quantity).replace(',', '.'))
  if (!(value > 0) || !(count > 0)) return ''
  return decimalText(value / count, 4).replace('.', ',')
}

// Русская часть до « / » с китайским текстом (у старых подборов агент писал и перевод).
export function russianPart(name: string) {
  const index = name.lastIndexOf('/')
  return index > 0 && /\p{Script=Han}/u.test(name.slice(index)) ? name.slice(0, index).trim() : name.trim()
}
