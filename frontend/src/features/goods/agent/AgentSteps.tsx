import { useState } from 'react'
import { plural } from '@/shared/lib/plural'
import { AgentMessage } from './agentApi'

const toolIcons: Record<string, string> = {
  search_nomenclature: '🔍', get_codes: '✔️', list_heading: '📂', open_details: '📄', search_decisions: '🗂️', search_directory: '📚',
  open_goods_help: '📋', open_link: '🔗', search_in_document: '🔎', web_search: '🌐', web_page: '🌐', photo: '🖼️', look_at_photo: '📷', search_declarations: '📑', general: '🗃️', act: '📖',
}

// Сообщения чата после поиска в базе: шаги агента (инструменты и его пояснения) — одной
// сворачиваемой группой, остальное — по одному.
type ChatBlock = { type: 'steps'; messages: AgentMessage[] } | { type: 'message'; message: AgentMessage }

export function chatBlocks(messages: AgentMessage[]): ChatBlock[] {
  const blocks: ChatBlock[] = []
  for (const message of messages) {
    if (message.kind === 'input' || message.kind === 'match' || message.kind === 'similar' || message.kind === 'start') continue
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

// «Что проверил агент»: по источникам — база решений, номенклатура, страницы кодов и документы (и интернет / фото,
// если агент к ним обращался). Пока агент работает, крутится источник его последнего шага.
const checkGroups: { label: string; tools: string[]; always: boolean; summary: (count: number) => string }[] = [
  { label: 'Номенклатура ТН ВЭД', tools: ['search_nomenclature', 'get_codes', 'list_heading'], always: true, summary: (count) => `${count} ${plural(count, 'запрос', 'запроса', 'запросов')}` },
  { label: 'Страницы кодов и документы', tools: ['open_details', 'open_goods_help', 'search_in_document', 'open_link'], always: true, summary: (count) => `открыто: ${count}` },
  { label: 'Фото товара', tools: ['photo', 'look_at_photo'], always: false, summary: (count) => `${count} ${plural(count, 'просмотр', 'просмотра', 'просмотров')}` },
  { label: 'Интернет и декларации', tools: ['web_search', 'web_page', 'search_declarations'], always: false, summary: (count) => `${count} ${plural(count, 'запрос', 'запроса', 'запросов')}` },
]

type CheckState = 'done' | 'run' | 'fail' | 'idle'

export function AgentChecks({ messages, searching, running, baseFound }: { messages: AgentMessage[]; searching: boolean; running: boolean; baseFound: number }) {
  const tools = messages.flatMap((message) => message.kind === 'tool' ? [message.content] : [])
  const lastTool = tools[tools.length - 1]?.tool
  const rows: { label: string; state: CheckState; summary: string }[] = [
    { label: 'База решений', state: searching ? 'run' : 'done', summary: searching ? 'ищу…' : baseFound ? `найдено: ${baseFound}` : 'не найдено' },
  ]
  for (const group of checkGroups) {
    const used = tools.filter((tool) => group.tools.includes(tool.tool))
    if (!used.length && !group.always) continue
    const state: CheckState = running && lastTool && group.tools.includes(lastTool) ? 'run'
      : !used.length ? 'idle' : used.every((tool) => tool.failed) ? 'fail' : 'done'
    rows.push({ label: group.label, state, summary: used.length ? group.summary(used.length) : running ? 'ожидает' : 'не понадобилось' })
  }
  return <ul className="ag-checks">
    {rows.map((row) => <li key={row.label} className={`ag-check-row ${row.state}`}>
      <CheckIcon state={row.state} />
      <span>{row.label}</span>
      <small>{row.summary}</small>
    </li>)}
  </ul>
}

function CheckIcon({ state }: { state: CheckState }) {
  if (state === 'run') return <span className="goods-spinner" aria-label="идёт" />
  if (state === 'fail') return <svg className="ag-check-icon" width="16" height="16" viewBox="0 0 16 16" aria-label="ошибка" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><circle cx="8" cy="8" r="6.5" /><path d="M5.8 5.8l4.4 4.4M10.2 5.8l-4.4 4.4" /></svg>
  if (state === 'idle') return <svg className="ag-check-icon" width="16" height="16" viewBox="0 0 16 16" aria-label="не было" fill="none" stroke="currentColor" strokeWidth="1.4" strokeDasharray="2 2"><circle cx="8" cy="8" r="6.5" /></svg>
  return <svg className="ag-check-icon" width="16" height="16" viewBox="0 0 16 16" aria-label="готово" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><circle cx="8" cy="8" r="6.5" /><path d="M5.3 8.2l1.9 1.9 3.6-3.8" /></svg>
}

// Ход подбора: свёрнут после итога, пока агент работает — открыт.
export function StepsGroup({ messages, live }: { messages: AgentMessage[]; live: boolean }) {
  const [open, setOpen] = useState(false)
  const steps = messages.filter((message) => message.kind === 'tool').length
  const expanded = open || live
  return <section className={expanded ? 'ag-steps open' : 'ag-steps'}>
    <button type="button" className="ag-steps-toggle" onClick={() => setOpen((value) => !value)} aria-expanded={expanded} disabled={live}>
      <span>Ход подбора</span><small>{steps} {plural(steps, 'шаг', 'шага', 'шагов')}</small>
      {!live && <i aria-hidden="true">{expanded ? '▴' : '▾'}</i>}
    </button>
    {expanded && <ol className="ag-steps-list">
      {messages.map((message) => {
        if (message.kind === 'text') return <li key={message.id} className="ag-thought">{message.content.text}</li>
        if (message.kind !== 'tool') return null
        const { tool, label, detail, url, failed } = message.content
        // Описание фото и ответы по фото — длинный текст: под подписью шага, с переносами.
        const block = tool === 'photo' || tool === 'look_at_photo'
        return <li key={message.id} className={'ag-step' + (failed ? ' failed' : '') + (block ? ' block' : '')}>
          <span className="ag-step-icon" aria-hidden="true">{toolIcons[tool] ?? '•'}</span>
          <span className="ag-step-label">{label}</span>
          <span className="ag-step-detail">{detail}</span>
          {url && <a href={url} target="_blank" rel="noreferrer" title="Открыть страницу кода">↗</a>}
        </li>
      })}
    </ol>}
  </section>
}
