import { AgentResult } from './agentApi'
import './AgentEvidence.css'

const confidenceLabels = { high: 'высокая', medium: 'средняя', low: 'низкая — проверьте код' }

// Уверенность агента в лучшем коде и страницы из интернета, на которые он опирался (карточка товара,
// решение таможни). Ссылки — только те, что агент действительно открывал: чужие программа убирает.
export function AgentEvidence({ result }: { result: AgentResult }) {
  if (!result.confidence && !result.sources?.length) return null
  return <div className="ag-evidence">
    {result.confidence && <span className={`ag-confidence ${result.confidence}`}>Уверенность: {confidenceLabels[result.confidence]}</span>}
    {result.confidence && result.confidenceReason && <p className="ag-confidence-why">{result.confidenceReason}</p>}
    {result.sources?.length ? <ul className="ag-sources" aria-label="Источники">
      {result.sources.map((source) => <li key={source.url}>
        <a href={source.url} target="_blank" rel="noreferrer">{source.title || new URL(source.url).hostname}</a>
        <small>{new URL(source.url).hostname}</small>
      </li>)}
    </ul> : null}
  </div>
}
