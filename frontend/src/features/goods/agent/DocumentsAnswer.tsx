import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { LinkText } from '@/shared/components/LinkText/LinkText'
import { AgentDocument, AgentMessage, AgentResult, DocumentKind } from './agentApi'
import './documents.css'

const appliesLabels: Record<AgentDocument['applies'], string> = { yes: 'нужно', maybe: 'возможно', no: 'не относится' }
// Условный выпуск — не документ, а возможность: «есть» или «нет».
const conditionalLabels: Record<AgentDocument['applies'], string> = { yes: 'есть', maybe: 'возможно', no: 'нет' }
// Вид документа коротко, как говорят в компании; other — без метки (мера названа в measure).
export const documentKindLabels: Record<DocumentKind, string> = { cc: 'СС', dc: 'ДС', sgr: 'СГР', conditional: 'Условный выпуск', other: '' }

// Кнопка «Документы для ввоза» на карточке кода (подходит / возможно) — в чате строки и справочнике.
export function DocumentsButton({ code, disabled, onClick }: { code: string; disabled: boolean; onClick: (code: string) => void }) {
  return <button type="button" className="dl-button" onClick={() => onClick(code)} disabled={disabled}
    title="Агент прочитает справку по товару tnved.info и акты по ссылкам">Документы для ввоза</button>
}

// Идёт ли сейчас запуск по кнопке «Документы для ввоза»: последний запрос сотрудника — эта кнопка.
export function documentsRequested(messages: AgentMessage[]) {
  for (let index = messages.length - 1; index >= 0; index--) {
    const kind = messages[index].kind
    if (kind === 'documents') return true
    if (kind === 'user' || kind === 'rerun' || kind === 'start' || kind === 'input') return false
  }
  return false
}

// Уже полученный ответ для кода — повторное нажатие просто прокручивает к нему.
export function scrollToDocuments(container: HTMLElement | null, code: string) {
  const answer = container?.querySelector<HTMLElement>(`[data-documents="${code}"]`)
  if (!answer) return false
  answer.scrollIntoView({ behavior: 'smooth', block: 'start' })
  answer.classList.remove('dl-flash')
  void answer.offsetWidth
  answer.classList.add('dl-flash')
  return true
}

// Ответ «Документы для ввоза»: резюме, меры (сначала нужные, потом «возможно», неподходящие — свёрнуты).
export function DocumentsAnswer({ result }: { result: AgentResult }) {
  const documents = result.documents ?? []
  const shown = [...documents.filter((item) => item.applies === 'yes'), ...documents.filter((item) => item.applies === 'maybe')]
  const rejected = documents.filter((item) => item.applies === 'no')
  return <section className="dl" data-documents={result.documentsCode}>
    <header className="dl-head">
      <h3>Документы для ввоза</h3>
      {result.documentsCode && <CopyCode code={result.documentsCode} className="dl-code" />}
    </header>
    {result.description && <p className="dl-desc"><LinkText text={result.description} /></p>}
    {shown.length > 0 && <>
      <p className="dl-title">Документы и меры</p>
      <ul className="dl-list">{shown.map((item, index) => <DocumentRow key={index} item={item} />)}</ul>
    </>}
    {documents.length === 0 && <p className="dl-empty">Мер для ввоза агент не нашёл — проверьте справку по ссылке в ходе поиска.</p>}
    {rejected.length > 0 && <details className="dl-more">
      <summary>Не относятся к товару ({rejected.length})</summary>
      <ul className="dl-list">{rejected.map((item, index) => <DocumentRow key={index} item={item} />)}</ul>
    </details>}
    {result.notes && <p className="dl-note"><LinkText text={result.notes} /></p>}
    {(result.linksRemoved ?? 0) > 0 && <p className="dl-notice">Программа убрала ссылки, которых не было на открытых страницах: {result.linksRemoved}.</p>}
    {result.quotesRemoved > 0 && <p className="dl-notice">Программа убрала основания-цитаты, которых нет в справке и актах: {result.quotesRemoved}.</p>}
    <p className="dl-disclaimer">Подсказка по справке tnved.info, перечням и актам — перед оформлением сверьте с актом.</p>
  </section>
}

// Заголовок меры — полностью, с большой буквы (и в сохранённых ответах).
const measureTitle = (text: string) => text.charAt(0).toUpperCase() + text.slice(1)

// Зелёная метка слева — коротко: вид документа (СС, ДС, СГР, условный выпуск) или «Санэпиднадзор».
export function documentBadge(item: Pick<AgentDocument, 'kind' | 'measure'>) {
  if (item.kind && documentKindLabels[item.kind]) return documentKindLabels[item.kind]
  return /санитарно[-\s]*эпидемиолог|санэпид/i.test(item.measure) ? 'Санэпиднадзор' : ''
}

// Мера свёрнута до заголовка и акта; по нажатию — пояснение, цитата-основание и подсказка по наименованию.
function DocumentRow({ item }: { item: AgentDocument }) {
  const head = <>
    <div className="dl-top">
      <b>{documentBadge(item) && <span className="dl-kind">{documentBadge(item)}</span>}{measureTitle(item.measure || item.act)}</b>
      <i className={`dl-pill ${item.applies}`}>{(item.kind === 'conditional' ? conditionalLabels : appliesLabels)[item.applies]}</i>
    </div>
    {item.act && item.measure && (item.url
      ? <a className="dl-act" href={item.url} target="_blank" rel="noreferrer">{item.act} ↗</a>
      : <span className="dl-act">{item.act}</span>)}
    {!item.measure && item.url && <a className="dl-act" href={item.url} target="_blank" rel="noreferrer">Открыть акт ↗</a>}
  </>
  if (!item.requirement && !item.basis && !item.nameHint) return <li className={`dl-item ${item.applies}`}>{head}</li>
  return <li>
    <details className={`dl-item ${item.applies}`}>
      <summary>{head}</summary>
      {item.requirement && <p className="dl-req"><LinkText text={item.requirement} /></p>}
      {item.basis && <blockquote className="dl-basis">{item.basis}<cite>основание: {item.basisSource || 'справка или акт'}</cite></blockquote>}
      {item.nameHint && <p className="dl-name"><b>Наименование:</b> <LinkText text={item.nameHint} /></p>}
    </details>
  </li>
}
