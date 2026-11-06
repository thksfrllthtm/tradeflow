import { useEffect, useState } from 'react'
import { Drawer } from '@/shared/components/Drawer/Drawer'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { LinkText } from '@/shared/components/LinkText/LinkText'
import { errorMessage } from '@/shared/lib/errorMessage'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { formatCode } from '@/shared/tnved/codes'
import { decisionStatusLabels, decisionStatusTones } from '@/shared/tnved/decisions'
import { AgentMessage, AgentResult } from '@/features/goods/agent/agentApi'
import { AgentEvidence } from '@/features/goods/agent/AgentEvidence'
import { DocumentsAnswer } from '@/features/goods/agent/DocumentsAnswer'
import { DeclarationsLink } from '@/features/goods/agent/DeclarationsLink'
import { CandidateRates, CandidateWhy, chatBlocks, CodeLine, StepsGroup, verdictLabels } from '@/features/goods/agent/AgentPanel'
import { agentLogApi, LogEntry, LogModel, ShadowMessage, TranscriptMessage } from './agentLogApi'
import { comparison, costText, durationText, modelOutcome, statusLabels } from './logRows'

// Карточка подбора: товар с фото, применённый код и проверка, две колонки — основная модель и тень:
// лента событий как в чате сотрудника и полная переписка с моделью (каждый вызов инструмента с ответом).
export function AgentLogEntryPanel({ id, onClose }: { id: number; onClose: () => void }) {
  const [entry, setEntry] = useState<LogEntry | null>(null)
  const [error, setError] = useState('')
  const [photo, setPhoto] = useState<number | null>(null)

  useEffect(() => {
    setEntry(null); setError('')
    agentLogApi.entry(id).then(setEntry).catch((reason) => setError(errorMessage(reason)))
  }, [id])

  const row = entry?.row
  const item = entry?.session.item
  const photos = entry?.session.photos ?? []
  const marks = row ? comparison(row) : []
  const shadowMessages: AgentMessage[] = (entry?.shadow?.messages ?? []).map((message: ShadowMessage, index) => ({ ...message, id: index + 1 } as AgentMessage))
  const facts = item ? [
    item.quantity && `${item.quantity} ${item.unit || 'шт'}`, item.netKg && `нетто ${item.netKg} кг`, item.pricePerUnit && `$${item.pricePerUnit} за ед.`,
    item.code && `код из файла ${formatCode(item.code)}`,
  ].filter(Boolean) : []

  // Пока открыто фото, Esc закрывает только его (у просмотра фото свой обработчик Esc).
  return <>
    <Drawer className="alog-entry" onClose={onClose} title={row ? row.name || 'Подбор' : 'Подбор'}
      subtitle={row ? <>{row.user} · <time dateTime={row.createdAt}>{new Date(row.createdAt).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' })}</time> · {row.mode === 'reference' ? 'чат-справочник' : 'строка сметы'}{row.runs > 1 ? ` · ходов: ${row.runs}` : ''}</> : 'Загрузка…'}>
        {error && <p className="banner error" role="alert">{error}</p>}
        {entry && row && item && <>
          <section className="alog-item">
            <div>
              {item.name && <p><b>{item.name}</b></p>}
              {item.supplierName && <p>{item.supplierName}</p>}
              {item.otherNames?.map((name) => <p key={name}>{name}</p>)}
              {item.details?.map((detail) => <p key={detail} className="alog-detail">{detail}</p>)}
              {facts.length > 0 && <small>{facts.join(' · ')}</small>}
              <p className="alog-applied">
                {row.decision
                  ? <>Сотрудник применил <code>{formatCode(row.decision.code)}</code><span className={`badge ${decisionStatusTones[row.decision.status]}`}>{decisionStatusLabels[row.decision.status]}</span></>
                  : <span className="alog-none">Код к строке не применён</span>}
                {marks.map((mark) => <span key={mark.text} className={`status-dot ${mark.tone === 'bad' ? 'error' : mark.tone} alog-mark`}>{mark.text}</span>)}
              </p>
              <div className="alog-item-links">
                {row.estimateId && <Link href={paths.goods(row.estimateId)}>Открыть смету ↗</Link>}
                {row.decision && <Link href={paths.decisions}>Запись в базе решений ↗</Link>}
              </div>
            </div>
            {photos.length > 0 && <div className="alog-photos">
              {photos.map((source, index) => <button key={index} type="button" onClick={() => setPhoto(index)} aria-label={`Фото ${index + 1}`}><img src={source} alt="" /></button>)}
            </div>}
          </section>
          <div className="alog-columns">
            <ModelColumn title="Основная модель" model={row.main} messages={entry.session.messages} transcript={entry.transcript} />
            <ModelColumn title="Теневая модель" model={row.shadow} messages={shadowMessages} transcript={entry.shadow?.transcript ?? []} />
          </div>
        </>}
    </Drawer>
    {photo !== null && <PhotoLightbox photos={photos} startIndex={photo} onClose={() => setPhoto(null)} />}
  </>
}

function ModelColumn({ title, model, messages, transcript }: { title: string; model: LogModel | null; messages: AgentMessage[]; transcript: TranscriptMessage[] }) {
  const outcome = modelOutcome(model)
  return <section className="alog-column">
    <header>
      <h3>{title}{model && <small>{model.model}</small>}</h3>
      {model ? <div className="alog-metrics">
        <div><small>Код</small><b>{outcome.tone === 'ok' ? <code>{outcome.text}</code> : <span className={`alog-state ${outcome.tone}`}>{outcome.text}</span>}</b></div>
        <div><small>Статус</small><b><i className={model.status === 'done' ? 'alog-dot on' : 'alog-dot'} aria-hidden="true" />{statusLabels[model.status] ?? model.status}</b></div>
        <div title="Токены × цены моделей из настроек сервера (запись в кэш Claude +25%). Это расчёт по прайсу, а не счёт провайдера: скидки DeepSeek за непиковые часы не учтены.">
          <small>Стоимость</small><b>{costText(model.costUsd)}</b><span>весь разговор</span>
        </div>
        <div title="От запуска модели до первого итога, вопроса или ошибки. Описание фото и поиск в базе решений до запуска не входят; у обеих моделей отсчёт от одного момента.">
          <small>Время</small><b>{durationText(model.seconds)}</b><span>до первого итога</span>
        </div>
        <div title="Вызовов модели за весь разговор: подбор, уточнения, под-агенты по актам.">
          <small>Шаги</small><b>{model.steps}</b><span>весь разговор</span>
        </div>
      </div> : <p className="alog-none">Тень для этого подбора не запускалась.</p>}
      {model?.error && <p className="ag-error">{model.error}</p>}
    </header>
    {chatBlocks(messages).map((block, index) => block.type === 'steps'
      ? <StepsGroup key={index} messages={block.messages} live={false} />
      : <LogMessage key={index} message={block.message} />)}
    {transcript.length > 0 && <details className="alog-transcript">
      <summary>Полная переписка с моделью · {transcript.length} сообщений</summary>
      {transcript.map((message, index) => <TranscriptView key={index} message={message} />)}
    </details>}
  </section>
}

function LogMessage({ message }: { message: AgentMessage }) {
  switch (message.kind) {
    case 'question': return <div className="ag-msg ag-question"><b>Вопрос агента</b>{message.content.text}</div>
    case 'user': return <div className="ag-msg ag-user">{message.content.text}</div>
    case 'documents': return <div className="ag-msg ag-user">Документы для ввоза · {formatCode(message.content.code)}</div>
    case 'notice': return <p className="ag-notice">{message.content.text}</p>
    case 'error': return <p className="ag-error">{message.content.message}</p>
    case 'rerun': return <p className="ag-divider"><span>подбор заново</span></p>
    case 'result': return message.content.documentsCode ? <DocumentsAnswer result={message.content} /> : <ReadOnlyResult result={message.content} />
    default: return null
  }
}

// Итог без кнопок применения: кандидаты с вердиктом, ставками и «почему», доказательства, наименование.
function ReadOnlyResult({ result }: { result: AgentResult }) {
  const main = result.candidates.filter((candidate) => candidate.verdict !== 'no')
  const rejected = result.candidates.filter((candidate) => candidate.verdict === 'no')
  const card = (candidate: AgentResult['candidates'][number]) => <article key={candidate.code} className={`ag-cand ${candidate.verdict}`}>
    <div className="ag-row-top"><CodeLine code={candidate.code} name={candidate.nomenclature?.name} /><i className={`ag-pill ${candidate.verdict}`}>{verdictLabels[candidate.verdict]}</i></div>
    <CandidateRates candidate={candidate} />
    <CandidateWhy candidate={candidate} />
    {candidate.declarations && <div className="ag-actions"><DeclarationsLink candidate={candidate} /></div>}
  </article>
  return <section className="ag-result">
    <header className="ag-section-head"><h3>Предложение</h3></header>
    {result.description && <p className="ag-desc"><LinkText text={result.description} /></p>}
    {main.length > 0 ? main.map(card) : <p className="ag-notice">Подходящего кода модель не нашла.</p>}
    <AgentEvidence result={result} />
    {rejected.length > 0 && <details className="ag-more"><summary>Не подходят: {rejected.map((candidate) => formatCode(candidate.code)).join(', ')}</summary>{rejected.map(card)}</details>}
    {result.name && <p className="ag-name">Наименование: {result.name}</p>}
    {result.notes && <p className="ag-note"><LinkText text={result.notes} /></p>}
  </section>
}

const roleLabels: Record<TranscriptMessage['role'], string> = { system: 'Подсказка', user: 'Программа', assistant: 'Модель', tool: 'Инструмент' }

// Одно сообщение переписки: роль, текст (фото — пометкой), размышления и вызовы инструментов — как есть.
function TranscriptView({ message }: { message: TranscriptMessage }) {
  const text = typeof message.content === 'string' ? message.content
    : (message.content ?? []).map((part) => part.type === 'text' ? part.text : part.type === 'image_ref' ? `[фото ${part.index + 1}]` : '[фото]').join('\n')
  return <div className={`alog-msg ${message.role}`}>
    <b>{roleLabels[message.role] ?? message.role}{message.tool_call_id && <small> · ответ на {message.tool_call_id}</small>}</b>
    {message.reasoning_content && <details><summary>Размышления</summary><pre>{message.reasoning_content}</pre></details>}
    {text && <pre>{text}</pre>}
    {message.tool_calls?.map((call) => <pre key={call.id} className="alog-call">{call.function.name}({call.function.arguments}) <small>#{call.id}</small></pre>)}
  </div>
}
