import { useMemo, useState } from 'react'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { plural } from '@/shared/lib/plural'
import { formatCode } from '@/shared/tnved/codes'
import { TrademarkHits } from '@/shared/trademarks/TrademarkHits'
import { AgentMessage, AgentResult, AgentSession, BrandCheck } from '@/features/goods/agent/agentApi'
import { brandShortText, brandText } from '@/features/goods/agent/answerCopy'
import { AnswerActions, CopyAnswer } from '@/features/goods/agent/CopyAnswer'
import { DocumentsAnswer, documentsRequested } from '@/features/goods/agent/DocumentsAnswer'
import { Answer } from './Answer'
import { authorName, initial, isWorking } from './chatSession'

const toolIcons: Record<string, string> = { search_nomenclature: '🔍', get_codes: '✔️', list_heading: '📂', open_details: '📄', search_decisions: '🗂️', search_directory: '📚', open_goods_help: '📋', open_link: '🔗', search_in_document: '🔎', web_search: '🌐', web_page: '🌐', photo: '🖼️', look_at_photo: '📷', search_declarations: '📑', check_trademarks: '®️', general: '🗃️', act: '📖' }

// Ответ «/троис»: какие бренды и рисунки ИИ увидел на фото и знаки реестра (или что их нет).
function BrandAnswer({ result, withPhotos }: { result: BrandCheck; withPhotos: boolean }) {
  return <>
    <div className="rc-answer">
      {withPhotos && <p className="rc-answer-text">{result.brands.length > 0 ? <>На фото: <b>{result.brands.join(', ')}</b></> : 'На фото брендов не видно.'}</p>}
      {result.pictures.length > 0 && <p className="rc-answer-text">Рисунок на знаке: <b>{result.pictures.join(', ')}</b></p>}
      {result.compared > 0 && <p className="rc-note">Фото сверено с {result.compared} {plural(result.compared, 'логотипом', 'логотипами', 'логотипами')} реестра: с похожим рисунком и той же группы товаров.</p>}
      {result.photoNote && <p className="rc-note">{result.photoNote}</p>}
      {result.textNote && <p className="rc-note">{result.textNote}</p>}
      {result.hits.length > 0 ? <>
        <TrademarkHits hits={result.hits} />
        <p className="rc-note">Решение за декларантом: совпадение — повод проверить, нужен ли товарный знак и согласие правообладателя.</p>
      </> : <p className="rc-answer-text">В реестре ТРОИС по написанию не найдено: ни этого знака, ни отличающихся на одну букву. Это не значит, что знака нет у правообладателя.</p>}
      <CopyAnswer short={brandShortText(result)} full={brandText(result)} />
    </div>
  </>
}

// Разговор: вопрос сотрудника, что нашлось в базе, ход поиска, вопросы агента, ответ.
// onShare — «Поделиться с сотрудником» в строке копирования последнего ответа (нет — вопрос чужой или агент ищет).
type ThreadProps = { session: AgentSession; onPhoto: (photos: string[], index: number) => void; documentsDisabled: boolean; onDocuments: (code: string) => void; onShare?: () => void }

export function Thread({ session, onPhoto, documentsDisabled, onDocuments, onShare }: ThreadProps) {
  const photos = session.photos ?? []
  const blocks = useMemo(() => threadBlocks(session.messages), [session.messages])
  const working = isWorking(session)
  let lastAnswer = -1
  blocks.forEach((block, index) => { if (block.type !== 'steps' && (block.message.kind === 'result' || block.message.kind === 'trademarks')) lastAnswer = index })
  const share = onShare && <button type="button" className="share" onClick={onShare} title="Открыть вопрос коллеге: он увидит его в своей истории и сможет только читать">
    <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><circle cx="6" cy="5.5" r="2.5" /><path d="M1.8 13.5c.6-2.3 2.2-3.5 4.2-3.5s3.6 1.2 4.2 3.5M11 4.5v4M9 6.5h4" /></svg>
    Поделиться с сотрудником</button>
  return <>
    {session.shared && <div className="rc-shared-banner" role="note">
      <span className="rc-shared-avatar" aria-hidden="true">{initial(session.sharedBy)}</span>
      <span><b>Чат {authorName(session.sharedBy)}</b><small>Открыт вам для просмотра — уточнять и перезапускать может только автор</small></span>
    </div>}
    {blocks.map((block, index) => <AnswerActions.Provider key={block.type === 'steps' ? `steps-${block.messages[0].id}` : block.message.id} value={index === lastAnswer ? share : null}>{(() => {
      if (block.type === 'steps') return <Steps key={`steps-${block.messages[0].id}`} messages={block.messages} live={working && index === blocks.length - 1} />
      const message = block.message
      switch (message.kind) {
        case 'input': return <div key={message.id} className="rc-user">
          {session.shared && <small className="rc-user-author">{session.sharedBy || 'Коллега'}</small>}
          {photos.length > 0 && <div className="rc-user-photos">{photos.map((photo, at) => <button key={at} type="button" onClick={() => onPhoto(photos, at)} aria-label="Открыть фото крупно"><img src={photo} alt="" /></button>)}</div>}
          {session.trademarkCheck ? <p>Проверка по ТРОИС{message.content.item.name && ` · ${message.content.item.name}`}</p>
            : message.content.item.name && <p>{message.content.item.name}</p>}
        </div>
        case 'trademarks': return <BrandAnswer key={message.id} result={message.content} withPhotos={photos.length > 0} />
        case 'user': return <div key={message.id} className="rc-user">{session.shared && <small className="rc-user-author">{session.sharedBy || 'Коллега'}</small>}<p>{message.content.text}</p></div>
        case 'documents': return <div key={message.id} className="rc-user"><p>Документы для ввоза · <code>{formatCode(message.content.code)}</code></p></div>
        case 'question': return <div key={message.id} className="rc-bot rc-question"><b>Уточните</b>{message.content.text}</div>
        case 'match': return message.content.matches.length > 0 ? <BaseHits key={message.id} items={message.content.matches.map((match) => ({ id: match.decision.id, code: match.decision.code, name: match.decision.finalName || match.decision.invoiceName, thumb: match.decision.thumb, note: match.byPhoto ? 'то же фото' : 'то же наименование', verified: match.decision.status === 'verified' }))} title="Этот товар уже оформляли" /> : null
        // Только записи, которые сверка фото подтвердила («тот же», «похожий»): похожесть по описанию без
        // сверки приводила мультиметр к салфеткам.
        case 'similar': {
          const confirmed = message.content.matches.filter((match) => match.verdict === 'same' || match.verdict === 'similar')
          return confirmed.length > 0 ? <BaseHits key={message.id} items={confirmed.map((match) => ({ id: match.decision.id, code: match.decision.code, name: match.decision.finalName || match.decision.invoiceName, thumb: match.decision.thumb, note: match.verdict === 'same' ? `тот же товар · ${match.score}%` : `похожий · ${match.score}%`, verified: match.decision.status === 'verified' }))} title="Похожие товары в базе решений" /> : null
        }
        case 'result': return message.content.documentsCode ? <DocumentsAnswer key={message.id} result={message.content} />
          : <Answer key={message.id} sessionId={session?.id ?? 0} result={message.content} previous={block.previous} documentsDisabled={documentsDisabled} onDocuments={onDocuments} />
        case 'notice': return <p key={message.id} className="rc-notice">{message.content.text}</p>
        case 'error': return <p key={message.id} className="rc-error">{message.content.message}
          {block.retryDocuments && <button type="button" className="secondary sm rc-retry" onClick={() => onDocuments(block.retryDocuments!)} disabled={documentsDisabled}>Повторить</button>}
        </p>
        case 'rerun': return <p key={message.id} className="rc-divider"><span>заново</span></p>
        default: return null
      }
    })()}</AnswerActions.Provider>)}
    {session.status === 'searching' && <p className="rc-typing"><span /><span /><span /> ищу в базе решений…</p>}
    {session.status === 'running' && <p className="rc-typing"><span /><span /><span /> {documentsRequested(session.messages) ? 'агенты разбирают справку и перечни параллельно…' : 'агент ищет код…'}</p>}
  </>
}

// previous — прошлый ответ в разговоре: с ним сравниваются коды ответа на уточнение. retryDocuments — у
// ошибки запроса «Документы для ввоза»: код для кнопки «Повторить».
type Block = { type: 'steps'; messages: AgentMessage[] } | { type: 'message'; message: AgentMessage; previous?: AgentResult; retryDocuments?: string }

function threadBlocks(messages: AgentMessage[]): Block[] {
  const blocks: Block[] = []
  let previous: AgentResult | undefined
  let documents = ''
  for (const message of messages) {
    if (message.kind === 'documents') documents = message.content.code
    else if (message.kind === 'user' || message.kind === 'rerun' || message.kind === 'result') documents = ''
    if (message.kind === 'error') {
      blocks.push({ type: 'message', message, retryDocuments: documents || undefined })
      continue
    }
    if (message.kind === 'result') {
      blocks.push({ type: 'message', message, previous })
      // Ответ про документы — не ответ о коде: следующее уточнение сравнивается с прежним.
      if (!message.content.documentsCode) previous = message.content
      continue
    }
    if (message.kind === 'rerun') previous = undefined
    if (message.kind === 'start') continue
    if (message.kind === 'tool' || message.kind === 'text') {
      const last = blocks[blocks.length - 1]
      if (last?.type === 'steps') last.messages.push(message)
      else blocks.push({ type: 'steps', messages: [message] })
      continue
    }
    blocks.push({ type: 'message', message })
  }
  return blocks
}

function Steps({ messages, live }: { messages: AgentMessage[]; live: boolean }) {
  const [open, setOpen] = useState(false)
  const count = messages.filter((message) => message.kind === 'tool').length
  const expanded = open || live
  return <div className={expanded ? 'rc-steps open' : 'rc-steps'}>
    <button type="button" onClick={() => setOpen((value) => !value)} disabled={live} aria-expanded={expanded}>
      Ход поиска · {count} {count === 1 ? 'шаг' : count > 1 && count < 5 ? 'шага' : 'шагов'}{!live && <i aria-hidden="true">{expanded ? '▴' : '▾'}</i>}
    </button>
    {expanded && <ol>{messages.map((message) => message.kind === 'tool'
      ? <li key={message.id} className={[message.content.failed && 'failed', (message.content.tool === 'photo' || message.content.tool === 'look_at_photo') && 'block'].filter(Boolean).join(' ') || undefined}><span aria-hidden="true">{toolIcons[message.content.tool] ?? '•'}</span>{message.content.label}<small>{message.content.detail}</small>{message.content.url && <a href={message.content.url} target="_blank" rel="noreferrer">↗</a>}</li>
      : message.kind === 'text' ? <li key={message.id} className="thought">{message.content.text}</li> : null)}</ol>}
  </div>
}

type Hit = { id: number; code: string; name: string; thumb: string; note: string; verified: boolean }

function BaseHits({ title, items }: { title: string; items: Hit[] }) {
  return <div className="rc-base">
    <p className="rc-base-title">{title}</p>
    {items.map((item) => <div key={item.id} className="rc-base-row">
      {item.thumb ? <img src={item.thumb} alt="" /> : <span className="rc-base-nophoto" aria-hidden="true" />}
      <div>
        <CopyCode code={item.code} className="rc-code" />
        <span className="rc-base-name">{item.name}</span>
      </div>
      <small>{item.verified ? 'проверено' : 'не проверено'} · {item.note}</small>
    </div>)}
  </div>
}
