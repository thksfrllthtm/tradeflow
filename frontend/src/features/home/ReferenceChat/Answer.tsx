import { useState } from 'react'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { CopyIcon } from '@/shared/components/CopyCode/CopyIcon'
import { LinkText } from '@/shared/components/LinkText/LinkText'
import { errorMessage } from '@/shared/lib/errorMessage'
import { nameParts } from '@/shared/tnved/codes'
import { TrademarkHits } from '@/shared/trademarks/TrademarkHits'
import { AgentCandidate, AgentResult, agentApi, quoteSourceLabels } from '@/features/goods/agent/agentApi'
import { resultShortText, resultText } from '@/features/goods/agent/answerCopy'
import { CopyAnswer } from '@/features/goods/agent/CopyAnswer'
import { AgentEvidence } from '@/features/goods/agent/AgentEvidence'
import { DeclarationsLink } from '@/features/goods/agent/DeclarationsLink'
import { DocumentsButton } from '@/features/goods/agent/DocumentsAnswer'
import { sameAnswer } from './answerText'

const verdictLabels: Record<AgentCandidate['verdict'], string> = { fits: 'подходит', doubtful: 'возможно', no: 'не подходит' }
const verdictTone: Record<AgentCandidate['verdict'], string> = { fits: 'ok', doubtful: 'warn', no: '' }

// Подходящие коды ответа с вердиктами — по ним видно, поменялся ли ответ после уточнения.
function mainCodes(result: AgentResult): string {
  return result.candidates.filter((candidate) => candidate.verdict !== 'no').map((candidate) => `${candidate.code}:${candidate.verdict}`).join(',')
}

// Ответ агента: что за товар, подходящие коды с цитатой и ставками, затем почему не соседние.
// Ответ на уточнение с теми же кодами — только текст и строка с кодом, без повторных карточек.
type AnswerProps = { sessionId: number; result: AgentResult; previous?: AgentResult; documentsDisabled: boolean; onDocuments: (code: string) => void }

export function Answer({ sessionId, result, previous, documentsDisabled, onDocuments }: AnswerProps) {
  const main = result.candidates.filter((candidate) => candidate.verdict !== 'no')
  const rejected = result.candidates.filter((candidate) => candidate.verdict === 'no')
  if (previous && main.length > 0 && mainCodes(previous) === mainCodes(result)) {
    return <div className="rc-answer rc-answer-short">
      {sameAnswer(result.description, previous.description) ? <p className="rc-answer-text rc-muted">Ответ тот же.</p>
        : result.description && <p className="rc-answer-text"><LinkText text={result.description} /></p>}
      {result.notes && <p className="rc-note"><LinkText text={result.notes} /></p>}
      {result.trademarks && <TrademarkHits hits={result.trademarks} />}
      <p className="rc-same-code">
        <span>{main.length > 1 ? 'Коды без изменений' : 'Код без изменений'}</span>
        {main.map((candidate) => <CopyCode key={candidate.code} code={candidate.code} className="rc-code small" />)}
      </p>
      <CopyAnswer short={resultShortText(result)} full={resultText(result)} />
    </div>
  }
  return <div className="rc-answer">
    {result.description && <p className="rc-answer-text"><LinkText text={result.description} /></p>}
    <AgentEvidence result={result} />
    {main.map((candidate) => <Candidate key={candidate.code} candidate={candidate} documentsDisabled={documentsDisabled} onDocuments={onDocuments} />)}
    {rejected.length > 0 && <div className="rc-rejected">
      <p>Когда был бы другой код</p>
      {rejected.map((candidate) => <div key={candidate.code} className="rc-rejected-row">
        <CopyCode code={candidate.code} className="rc-code small" />
        <span><LinkText text={candidate.reason} /> <DeclarationsLink candidate={candidate} className="rc-more" /></span>
      </div>)}
    </div>}
    <DeclarationNames sessionId={sessionId} short={result.name} full={result.nameFull ?? ''} />
    {result.notes && <p className="rc-note"><LinkText text={result.notes} /></p>}
    {result.trademarks && <TrademarkHits hits={result.trademarks} />}
    <CopyAnswer short={resultShortText(result)} full={resultText(result)} />
  </div>
}

// Наименование для декларации — краткое и подробное: копировать или перевести на китайский
// («русское / 中文», перевод — тем же запросом, что «Применить наименование» в подборе строки).
function DeclarationNames({ sessionId, short, full }: { sessionId: number; short: string; full: string }) {
  const variants = [{ label: 'Краткое', name: short.trim() }, { label: 'Подробное', name: full.trim() }]
    .filter((variant, index, all) => variant.name && all.findIndex((other) => other.name === variant.name) === index)
  if (variants.length === 0) return null
  return <div className="rc-names">
    <p>Наименование для декларации</p>
    {variants.map((variant) => <NameVariant key={variant.label} sessionId={sessionId} label={variants.length > 1 ? variant.label : ''} name={variant.name} />)}
  </div>
}

function NameVariant({ sessionId, label, name }: { sessionId: number; label: string; name: string }) {
  const [shown, setShown] = useState(name)
  const [busy, setBusy] = useState(false)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')
  const translated = shown !== name

  async function copy() {
    try {
      await navigator.clipboard.writeText(shown)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1400)
    } catch {
      // Буфер обмена недоступен — текст можно выделить и скопировать вручную.
    }
  }

  async function translate() {
    setBusy(true); setError('')
    try { setShown((await agentApi.name(sessionId, name)).name) } catch (reason) { setError(errorMessage(reason)) } finally { setBusy(false) }
  }

  return <div className="rc-name-row">
    {label && <span className="rc-name-label">{label}</span>}
    <span className="rc-name-text">{shown}</span>
    <span className="rc-name-actions">
      <button type="button" className="button-link rc-copy" onClick={() => void copy()}><CopyIcon copied={copied} />{copied ? 'Скопировано' : 'Копировать'}</button>
      {!translated && sessionId > 0 && <button type="button" className="button-link" onClick={() => void translate()} disabled={busy}>{busy ? 'Перевод…' : 'Перевести на китайский'}</button>}
    </span>
    {error && <span className="rc-name-error" role="alert">{error}</span>}
  </div>
}

function Candidate({ candidate, documentsDisabled, onDocuments }: { candidate: AgentCandidate; documentsDisabled: boolean; onDocuments: (code: string) => void }) {
  const parts = candidate.nomenclature ? nameParts(candidate.nomenclature.name) : []
  const nomenclature = candidate.nomenclature
  return <article className={`rc-cand ${candidate.verdict}`}>
    <div className="rc-cand-top">
      <CopyCode code={candidate.code} className="rc-code big" />
      <i className={`badge rc-verdict ${verdictTone[candidate.verdict]}`}>{verdictLabels[candidate.verdict]}</i>
    </div>
    {parts.length > 0 && <p className="rc-cand-name" title={parts.join(' › ')}>{parts.slice(-2).join(' › ')}</p>}
    {nomenclature && <p className="rc-rates">
      <span>Пошлина {nomenclature.tariffText || 'не указана'}</span>
      <span>НДС {nomenclature.vatPercent || '16'}%</span>
      {nomenclature.antiDumping?.map((measure) => <span key={measure.number} className={measure.exact && !measure.suspended ? 'dump' : 'dump maybe'} title={`${measure.number} «${measure.shortName}»`}>
        {measure.suspended ? 'антидемпинг не применяется' : `${measure.exact ? 'антидемпинг' : 'возможен антидемпинг'} ${measure.countries.map((country) => `${(measure.rates[country] ?? '').replace('.', ',')}%`).join(' / ')} (${measure.countryNames})`}
      </span>)}
    </p>}
    {candidate.quote && <blockquote>{candidate.quote}{quoteSourceLabels[candidate.quoteSource] && <cite>{quoteSourceLabels[candidate.quoteSource]}</cite>}</blockquote>}
    {candidate.decisionQuote && <blockquote>{candidate.decisionQuote}<cite>{candidate.decisionSource} (TWS.BY)</cite></blockquote>}
    {candidate.reason && <p className="rc-reason"><LinkText text={candidate.reason} /></p>}
    {candidate.check && <p className="rc-check"><b>Уточнить:</b> <LinkText text={candidate.check} /></p>}
    <div className="rc-cand-actions">
      {candidate.verdict !== 'no' && <DocumentsButton code={candidate.code} disabled={documentsDisabled} onClick={onDocuments} />}
      {nomenclature?.detailsUrl && <a className="rc-more" href={nomenclature.detailsUrl} target="_blank" rel="noreferrer">Страница кода ↗</a>}
      <DeclarationsLink candidate={candidate} className="rc-more" />
    </div>
  </article>
}
