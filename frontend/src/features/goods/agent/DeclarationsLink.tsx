import { AgentCandidate } from './agentApi'

// «Декларации ↗» у кода итога: описания из деклараций под этим кодом на ifcg.ru по словам, которыми искал
// агент, и сколько записей он там нашёл. Практика других декларантов, а не довод, что код верен.
export function DeclarationsLink({ candidate, className }: { candidate: AgentCandidate; className?: string }) {
  const declarations = candidate.declarations
  if (!declarations) return null
  return <a className={className} href={declarations.url} target="_blank" rel="noreferrer"
    title={`Как декларировали другие: описания из деклараций под этим кодом по словам «${declarations.query}» (ifcg.ru). Практика, а не правило: бывают ошибки.`}>
    Декларации{declarations.count ? ` · ${declarations.count}` : ''} ↗
  </a>
}
