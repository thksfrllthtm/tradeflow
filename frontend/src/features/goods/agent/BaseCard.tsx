import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { CodeDecision, decisionStatusLabels } from '@/shared/tnved/decisions'
import { SameProductMatch, SimilarMatch } from './agentApi'
import { Applied, CodeLine } from './ResultCard'

// onApply(code, name): пустая строка — это не применять (только код или только наименование).
export type DecisionActions = { applied: Applied; busy: boolean; rowName: string; onApply: (code: string, name: string) => void; onOpenPhotos: (id: number) => void }

const similarVerdicts: Record<SimilarMatch['verdict'], string> = { same: 'тот же товар', similar: 'похожий товар', '': '' }

// Что нашлось в базе решений: тот же товар (по фото или наименованию) и похожие по фото.
export function BaseCard({ match, similar, searching, hasPhotos, actions }: { match: SameProductMatch[]; similar?: { matches: SimilarMatch[]; description: string }; searching: boolean; hasPhotos: boolean; actions: DecisionActions }) {
  const similarMatches = similar?.matches ?? []
  const found = match.length + similarMatches.length
  return <section className="ag-card ag-base">
    <header className="ag-card-head">
      <h3>В базе решений</h3>
      <small>{searching ? 'ищу…' : found ? `найдено: ${found}` : 'ничего не найдено'}</small>
    </header>
    {match.map(({ decision, byPhoto, byName, photoDiffers }) => <DecisionRow key={`same-${decision.id}`} decision={decision} actions={actions}
      badge={photoDiffers ? <span className="badge warn" title="Наименование то же, но фото другое — фото главнее">то же наименование</span> : <span className="badge ok">тот же товар</span>}
      note={photoDiffers ? 'Наименование совпало, но на фото, похоже, другой товар — сверьте фото, прежде чем применять код.' : undefined}
      meta={`узнан ${[byPhoto && 'по фото', byName && 'по наименованию'].filter(Boolean).join(' и ')} · в инвойсе: «${decision.invoiceName || decision.supplierName}»`} />)}
    {similarMatches.map((item) => <DecisionRow key={`similar-${item.decision.id}`} decision={item.decision} actions={actions}
      badge={<>{item.verdict && <span className={item.verdict === 'same' ? 'badge ok' : 'badge warn'}>{similarVerdicts[item.verdict]}</span>}<span className="badge ag-score" title="Похожесть по фото">{item.score}%</span></>}
      meta={[item.byPhoto && 'снимок похож', item.byDescription && item.description && `в базе: ${item.description}`].filter(Boolean).join(' · ')}
      note={item.why} />)}
    {!searching && found === 0 && <p className="ag-empty-line">{hasPhotos ? 'Такого товара и похожих по фото в базе нет.' : 'Такого товара в базе нет. Похожие ищутся по фото — у строки фото нет.'}</p>}
    {similar?.description && <p className="ag-seen">На фото: {similar.description}</p>}
  </section>
}

function DecisionRow({ decision, actions, badge, meta, note }: { decision: CodeDecision; actions: DecisionActions; badge: React.ReactNode; meta: string; note?: string }) {
  const { applied, busy, rowName, onApply, onOpenPhotos } = actions
  const name = decision.finalName.trim()
  const codeDone = applied.code === decision.code
  const nameDone = !name || applied.name === name || rowName === name
  const noCode = !decision.nomenclature
  return <article className="ag-row">
    {decision.thumb
      ? <button type="button" className="ag-photo ag-thumb" onClick={() => onOpenPhotos(decision.id)} title="Открыть фото из базы крупно" aria-label="Открыть фото записи базы крупно"><img src={decision.thumb} alt="" /></button>
      : <span className="ag-thumb" aria-hidden="true" />}
    <div className="ag-row-body">
      <div className="ag-row-top">
        <CodeLine code={decision.code} name={decision.nomenclature?.name} />
        <span className="ag-pills">{badge}</span>
      </div>
      {decision.finalName && <p className="ag-row-name">{decision.finalName}</p>}
      <p className="ag-row-meta">{decisionStatusLabels[decision.status]}{decision.verifiedByName && ` · проверил ${decision.verifiedByName}`}{meta && ` · ${meta}`}</p>
      {note && <p className="ag-row-note">{note}</p>}
      {!decision.nomenclature && <p className="ag-warn">Кода нет в текущей номенклатуре — проверьте.</p>}
      <div className="ag-actions">
        {codeDone && nameDone && <span className="ag-done">{name ? '✓ код и наименование применены' : '✓ код применён'}</span>}
        {!codeDone && <button type="button" className="sm" onClick={() => onApply(decision.code, nameDone ? '' : name)} disabled={busy || noCode}>{nameDone ? 'Применить код' : 'Применить код и наименование'}</button>}
        {!codeDone && !nameDone && <button type="button" className="button-link" onClick={() => onApply(decision.code, '')} disabled={busy || noCode}>только код</button>}
        {codeDone && !nameDone && <button type="button" className="secondary sm" onClick={() => onApply('', name)} disabled={busy}>Применить наименование</button>}
        {decision.nomenclature?.detailsUrl && <a href={decision.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}
        <Link href={`${paths.decisions}?id=${decision.id}`}>В базе решений</Link>
      </div>
    </div>
  </article>
}
