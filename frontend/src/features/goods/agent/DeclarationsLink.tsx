import { AgentCandidate } from './agentApi'

// Ссылки у кода итога: описания из деклараций под этим кодом по словам агента (ifcg.ru), поиск по тем же
// словам на tks.ru и сборник предварительных решений таможенных органов ЕАЭС; число — сколько записей под
// этим кодом агент там нашёл. Декларации — практика других декларантов, решения ЕАЭС — официальные.
// Старые подборы хранят одну ссылку на ifcg.ru.
const siteLabels: Record<string, { label: string; title: string }> = {
  'ifcg.ru': { label: 'ifcg.ru', title: 'Описания из деклараций под этим кодом по словам агента (ifcg.ru). Практика, а не правило: бывают ошибки.' },
  'tks.ru': { label: 'tks.ru', title: 'Примеры декларирования по тем же словам на tks.ru (все слова сразу).' },
  'nsi.eaeunion.org': { label: 'ЕАЭС', title: 'Сборник предварительных решений таможенных органов ЕАЭС по классификации (nsi.eaeunion.org): официальные решения с кодом, описанием и обоснованием.' },
}

export function DeclarationsLink({ candidate, className }: { candidate: AgentCandidate; className?: string }) {
  const declarations = candidate.declarations
  if (!declarations) return null
  if (!declarations.links?.length) {
    return <a className={className} href={declarations.url} target="_blank" rel="noreferrer"
      title={`Как декларировали другие: описания из деклараций под этим кодом по словам «${declarations.query}» (ifcg.ru). Практика, а не правило: бывают ошибки.`}>
      Декларации{declarations.count ? ` · ${declarations.count}` : ''} ↗
    </a>
  }
  return <span className="ag-decl">
    <span className="ag-decl-label">Декларации и решения:</span>
    {declarations.links.map((link) => {
      const site = siteLabels[link.site] ?? { label: link.site, title: link.site }
      return <a key={link.site} className={className} href={link.url} target="_blank" rel="noreferrer" title={`${site.title} Слова поиска: «${declarations.query}».`}>
        {site.label}{link.count ? ` · ${link.count}` : ''} ↗
      </a>
    })}
  </span>
}
