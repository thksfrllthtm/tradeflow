import { formatRuDate } from '@/shared/lib/dates'
import { goodsNote, registryGoods, shortHolder, TrademarkHit, trademarkLogoUrl, trademarkName } from './trademarks'
import './TrademarkHits.css'

// Знаки ТРОИС, найденные в товаре: логотип из реестра (если есть), что совпало и с чем, правообладатель
// и срок защиты. Сотрудник решает сам: нужен ли товарный знак и согласие правообладателя.
export function TrademarkHits({ hits }: { hits: TrademarkHit[] }) {
  if (hits.length === 0) return null
  return <section className="tm-hits" aria-label="Таможенный реестр ОИС">
    <strong>Таможенный реестр ОИС (ТРОИС)</strong>
    <ul>{hits.map((hit) => <li key={hit.regNo} className={hit.kind === 'lookalike' ? 'tm-hit lookalike' : 'tm-hit'}>
      {hit.hasLogo && <img src={trademarkLogoUrl(hit.regNo)} alt={hit.name || 'Логотип из реестра'} loading="lazy" />}
      <div>
        <b>{hit.kind === 'lookalike' ? `Похоже на ${trademarkName(hit)} — возможная имитация` : `Знак ${trademarkName(hit)}`}</b>
        <span>{hit.byPicture ? `по изображению${hit.why ? `: ${hit.why}` : ''}` : `в товаре: «${hit.found}»`}{goodsNote(hit) && ` · ${goodsNote(hit).toLowerCase()}`}</span>
        {registryGoods(hit.goods) && <small>Зарегистрирован на: {registryGoods(hit.goods)}</small>}
        <small>№ {hit.regNo}{hit.holder && ` · ${shortHolder(hit.holder)}`}{hit.protectedUntil && <> · защита до <time dateTime={hit.protectedUntil.slice(0, 10)}>{formatRuDate(hit.protectedUntil)}</time></>}</small>
      </div>
    </li>)}</ul>
  </section>
}
