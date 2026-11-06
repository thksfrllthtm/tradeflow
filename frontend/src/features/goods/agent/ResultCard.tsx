import { useId, useState } from 'react'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { LinkText } from '@/shared/components/LinkText/LinkText'
import { nameParts, formatCode } from '@/shared/tnved/codes'
import { TrademarkHits } from '@/shared/trademarks/TrademarkHits'
import { AgentCandidate, AgentMessage, AgentResult, quoteSourceLabels } from './agentApi'
import { AgentEvidence } from './AgentEvidence'
import { DeclarationsLink } from './DeclarationsLink'
import { DocumentsAnswer, DocumentsButton } from './DocumentsAnswer'
import { resultShortText, resultText } from './answerCopy'
import { CopyAnswer } from './CopyAnswer'
import { russianPart } from './ItemCard'

export const verdictLabels: Record<AgentCandidate['verdict'], string> = { fits: 'подходит', doubtful: 'возможно', no: 'не подходит' }
// Тон бейджа (.badge) по вердикту.
const verdictBadge: Record<AgentCandidate['verdict'], string> = { fits: 'badge ok', doubtful: 'badge warn', no: 'badge' }

export type Applied = { code?: string; name?: string; nameRu?: string }

type MessageProps = {
  message: AgentMessage
  applied: Applied
  busy: boolean
  // Код строки при начале подбора (из файла) — сравнить с предложением агента.
  fileCode: string
  onApplyCode: (code: string, record: boolean) => void
  onApplyName: (name: string) => void
  saveToBase: boolean
  onSaveToBase: (value: boolean) => void
  documentsDisabled: boolean
  onDocuments: (code: string) => void
}

export function MessageView({ message, ...props }: MessageProps) {
  switch (message.kind) {
    case 'question': return <div className="ag-msg ag-question"><b>Вопрос агента</b>{message.content.text}</div>
    case 'user': return <div className="ag-msg ag-user">{message.content.text}</div>
    case 'documents': return <div className="ag-msg ag-user">Документы для ввоза · {formatCode(message.content.code)}</div>
    case 'notice': return <p className="ag-notice">{message.content.text}</p>
    case 'error': return <p className="ag-error">{message.content.message}</p>
    case 'rerun': return <p className="ag-divider"><span>подбор заново</span></p>
    case 'result': return message.content.documentsCode ? <DocumentsAnswer result={message.content} /> : <ResultCard result={message.content} {...props} />
    default: return null
  }
}

export function CodeLine({ code, name }: { code: string; name?: string }) {
  const parts = name ? nameParts(name) : []
  return <div className="ag-code-line">
    <CopyCode code={code} className="ag-code" />
    {parts.length > 0 && <span title={parts.join(' › ')}>{parts.slice(-2).join(' › ')}</span>}
  </div>
}

// Наименование агента можно исправить; перевод на китайский запрашивается только по кнопке.
function NameEditor({ suggested, applied, busy, onApply }: { suggested: string; applied: Applied; busy: boolean; onApply: (name: string) => void }) {
  const inputId = useId()
  const [draft, setDraft] = useState(() => russianPart(suggested))
  const value = draft.trim()
  const done = Boolean(applied.name) && applied.nameRu === value
  return <div className="ag-name">
    <label htmlFor={inputId}>Наименование для декларации</label>
    <textarea id={inputId} value={draft} onChange={(event) => setDraft(event.target.value)} rows={2} maxLength={200} disabled={busy} />
    {done && <p className="ag-name-final">{applied.name}</p>}
    <div className="ag-actions">
      {done
        ? <span className="ag-done">✓ применено с переводом на китайский</span>
        : <button type="button" className="secondary sm" onClick={() => onApply(value)} disabled={busy || !value}>{busy ? 'Перевожу…' : 'Применить наименование'}</button>}
      {!done && <span className="ag-muted">перевод на китайский добавится при применении</span>}
    </div>
  </div>
}

// Ставки кода чипами и антидемпинг ЕАЭС (реестр ЕЭК).
export function CandidateRates({ candidate }: { candidate: AgentCandidate }) {
  return <>
    {candidate.nomenclature && <div className="ag-chips">
      <span className="ag-chip">Пошлина {candidate.nomenclature.tariffText || 'не указана'}</span>
      <span className="ag-chip">НДС {candidate.nomenclature.vatPercent || '16'}%</span>
    </div>}
    {candidate.nomenclature?.antiDumping?.map((measure) => <p key={measure.number} className={measure.exact && !measure.suspended ? 'ag-antidumping' : 'ag-antidumping maybe'}>
      {measure.suspended ? 'Антидемпинг временно не применяется' : measure.exact ? 'Антидемпинг' : 'Возможен антидемпинг'}: {measure.number} «{measure.shortName}» — {measure.countries.map((country) => `${measure.countryNames && measure.countries.length === 1 ? measure.countryNames : country} ${(measure.rates[country] ?? '').replace('.', ',')}%`).join(', ')}
      {!measure.exact && ' (мера на часть товаров позиции — сверьте описание)'}
    </p>)}
  </>
}

// «Почему»: цитата из источника, довод агента и что уточнить.
export function CandidateWhy({ candidate }: { candidate: AgentCandidate }) {
  return <>
    {candidate.quote && <blockquote>{candidate.quote}{quoteSourceLabels[candidate.quoteSource] && <cite>{quoteSourceLabels[candidate.quoteSource]}</cite>}</blockquote>}
    {candidate.decisionQuote && <blockquote>{candidate.decisionQuote}<cite>{candidate.decisionSource}{candidate.decisionSource?.includes("(") ? "" : " (TWS.BY)"}</cite></blockquote>}
    {candidate.reason && <p className="ag-reason"><LinkText text={candidate.reason} /></p>}
    {candidate.check && <p className="ag-check"><b>Уточнить:</b> <LinkText text={candidate.check} /></p>}
  </>
}

type CandidateProps = { candidate: AgentCandidate; applied: Applied; busy: boolean; onApplyCode: (code: string, record: boolean) => void; documentsDisabled: boolean; onDocuments: (code: string) => void }

// Другие варианты агента — компактно, под главным предложением.
function CandidateCard({ candidate, applied, busy, onApplyCode, documentsDisabled, onDocuments }: CandidateProps) {
  return <article className={`ag-cand ${candidate.verdict}`}>
    <div className="ag-row-top">
      <CodeLine code={candidate.code} name={candidate.nomenclature?.name} />
      <span className={verdictBadge[candidate.verdict]}>{verdictLabels[candidate.verdict]}</span>
    </div>
    <CandidateRates candidate={candidate} />
    <CandidateWhy candidate={candidate} />
    <div className="ag-actions">
      {candidate.verdict !== 'no' && (applied.code === candidate.code ? <span className="ag-done">✓ применён</span> : <button type="button" className="secondary sm" onClick={() => onApplyCode(candidate.code, true)} disabled={busy}>Применить код</button>)}
      {candidate.verdict !== 'no' && <DocumentsButton code={candidate.code} disabled={documentsDisabled} onClick={onDocuments} />}
      {candidate.nomenclature?.detailsUrl && <a href={candidate.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}
      <DeclarationsLink candidate={candidate} />
    </div>
  </article>
}

// Главное предложение агента: код крупно, позиция, ставки, сравнение с кодом из файла, «Почему», применение.
function ProposalCard({ candidate, result, fileCode, applied, busy, onApplyCode, saveToBase, onSaveToBase, documentsDisabled, onDocuments }: CandidateProps & { result: AgentResult; fileCode: string; saveToBase: boolean; onSaveToBase: (value: boolean) => void }) {
  const checkboxId = useId()
  const parts = candidate.nomenclature ? nameParts(candidate.nomenclature.name) : []
  const same = Boolean(fileCode) && fileCode === candidate.code
  const differs = result.fileCode?.dutyDiffers ? ` · пошлина ${result.fileCode.duty} → ${result.fileCode.bestDuty}` : ''
  return <article className="ag-proposal">
    <div className="ag-proposal-top">
      <CopyCode code={candidate.code} className="ag-proposal-code" />
      <span className={verdictBadge[candidate.verdict]}>{verdictLabels[candidate.verdict]}</span>
    </div>
    {parts.length > 0 && <p className="ag-proposal-name" title={parts.join(' › ')}>{parts.slice(-2).join(' › ')}</p>}
    <CandidateRates candidate={candidate} />
    {fileCode && <span className={same ? 'ag-match same' : 'ag-match differs'}>{same ? 'Совпадает с кодом из файла' : `Отличается от кода из файла${differs}`}</span>}
    {(candidate.quote || candidate.reason || candidate.check) && <div className="ag-why"><b>Почему</b><CandidateWhy candidate={candidate} /></div>}
    <div className="ag-proposal-actions">
      {applied.code === candidate.code
        ? <span className="ag-done">✓ Код применён к строке</span>
        : <button type="button" onClick={() => onApplyCode(candidate.code, true)} disabled={busy}>{same ? 'Подтвердить код' : 'Применить код'}</button>}
      <label className="ag-save" htmlFor={checkboxId}>
        <input id={checkboxId} type="checkbox" checked={saveToBase} onChange={(event) => onSaveToBase(event.target.checked)} />
        Сохранить в базу решений
      </label>
    </div>
    <div className="ag-actions">
      <DocumentsButton code={candidate.code} disabled={documentsDisabled} onClick={onDocuments} />
      {candidate.nomenclature?.detailsUrl && <a href={candidate.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}
      <DeclarationsLink candidate={candidate} />
    </div>
  </article>
}

// Итог: главное предложение, другие подходящие коды, неподходящие — свёрнуты; наименование и замечания.
type ResultProps = Omit<MessageProps, 'message'> & { result: AgentResult }

function ResultCard({ result, fileCode, applied, busy, onApplyCode, onApplyName, saveToBase, onSaveToBase, documentsDisabled, onDocuments }: ResultProps) {
  const main = result.candidates.filter((candidate) => candidate.verdict !== 'no')
  const rejected = result.candidates.filter((candidate) => candidate.verdict === 'no')
  const [best, ...others] = main
  const shared = { applied, busy, onApplyCode, documentsDisabled, onDocuments }
  return <section className="ag-result">
    <header className="ag-section-head"><h3>Предложение агента</h3></header>
    {result.description && <p className="ag-desc"><LinkText text={result.description} /></p>}
    {best ? <ProposalCard candidate={best} result={result} fileCode={fileCode} saveToBase={saveToBase} onSaveToBase={onSaveToBase} {...shared} />
      : <p className="ag-notice">Подходящего кода агент не нашёл — уточните товар в сообщении ниже.</p>}
    <AgentEvidence result={result} />
    {others.length > 0 && <div className="ag-others">
      <h4>Другие варианты</h4>
      {others.map((candidate) => <CandidateCard key={candidate.code} candidate={candidate} {...shared} />)}
    </div>}
    {rejected.length > 0 && <details className="ag-more">
      <summary>Не подходят: {rejected.map((candidate) => formatCode(candidate.code)).join(', ')}</summary>
      {rejected.map((candidate) => <CandidateCard key={candidate.code} candidate={candidate} {...shared} />)}
    </details>}
    {result.name && <NameEditor suggested={result.name} applied={applied} busy={busy} onApply={onApplyName} />}
    {result.notes && <p className="ag-note"><LinkText text={result.notes} /></p>}
    {result.trademarks && <TrademarkHits hits={result.trademarks} />}
    {(result.droppedCodes.length > 0 || result.quotesRemoved > 0) && <p className="ag-notice">
      {result.droppedCodes.length > 0 && <>Программа убрала коды, которых нет в номенклатуре: {result.droppedCodes.map(formatCode).join(', ')}. </>}
      {result.quotesRemoved > 0 && <>Цитат, не найденных в источниках, убрано: {result.quotesRemoved}.</>}
    </p>}
    <CopyAnswer short={resultShortText(result)} full={resultText(result)} />
  </section>
}
