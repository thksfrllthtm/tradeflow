import { useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuTime } from '@/shared/lib/dates'
import { User } from '@/shared/types/user'
import { formatCode } from '@/shared/tnved/codes'
import { decisionStatusLabels, decisionStatusTones } from '@/shared/tnved/decisions'
import { Employee, employeesApi } from '@/features/employees/employeesApi'
import { AgentLogEntryPanel } from './AgentLogEntry'
import { agentLogApi, LogFilter, LogModel, LogPage, LogRow } from './agentLogApi'
import { comparison, durationText, isoDaysAgo, metaText, modelOutcome, percent, rangeText } from './logRows'
import '@/shared/styles/list-page.css'
import './agentLog.css'
import { PageHead } from '@/shared/components/PageHead/PageHead'

const pageSize = 50
const onlyOptions: { value: LogFilter['only']; label: string }[] = [
  { value: '', label: 'Все' }, { value: 'disagree', label: 'Разошлись' }, { value: 'verified', label: 'С проверкой' },
  { value: 'wrong', label: 'Основная ошиблась' }, { value: 'documents', label: 'Разошлись по документам' }, { value: 'shadow', label: 'С тенью' },
]
const modeOptions: { value: string; label: string }[] = [{ value: '', label: 'Все' }, { value: 'row', label: 'Строки смет' }, { value: 'reference', label: 'Справочник' }]
const modeLabels: Record<string, string> = { row: 'строка сметы', reference: 'справочник' }
const defaultFilter = (): LogFilter => ({ from: isoDaysAgo(29), to: isoDaysAgo(0), userId: '', mode: '', only: '' })

// Журнал агента: все подборы всех сотрудников, рядом итог теневой модели и запись базы решений. Страница
// только для администратора: сервер проверяет права сам, приложение не показывает её остальным.
// Фильтры применяются сразу; таблица занимает оставшуюся высоту экрана, шапка закреплена.
export function AgentLogPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [filter, setFilter] = useState<LogFilter>(defaultFilter)
  const [page, setPage] = useState(1)
  const [data, setData] = useState<LogPage | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [employees, setEmployees] = useState<Employee[]>([])
  const [selected, setSelected] = useState<number | null>(null)

  useEffect(() => { employeesApi.list().then(setEmployees).catch(() => setEmployees([])) }, [])
  useEffect(() => {
    let stale = false
    setLoading(true)
    agentLogApi.list(filter, page, pageSize)
      .then((result) => { if (!stale) { setData(result); setError('') } })
      .catch((reason) => { if (!stale) setError(errorMessage(reason)) })
      .finally(() => { if (!stale) setLoading(false) })
    return () => { stale = true }
  }, [filter, page])

  const change = (patch: Partial<LogFilter>) => { setFilter((current) => ({ ...current, ...patch })); setPage(1) }
  const summary = data?.summary
  const shadow = data?.shadow
  const total = summary?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / pageSize))

  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="agentLog" onLogout={onLogout} />
    <section className="directory-page alog">
      <PageHead title="Журнал агента" count={summary ? total : undefined}
        subtitle="Подборы кодов всех сотрудников: что предложила основная модель, что — теневая, какой код применили и что подтвердила проверка." />
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
      {summary && shadow && <div className="alog-summary">
        <Stat label="Подборов" value={String(total)} note={`с итогом: ${summary.withResult}`} />
        <Stat label="Модели согласны" value={percent(summary.agree, summary.agree + summary.disagree)} note={`код: ${summary.agree} из ${summary.agree + summary.disagree} · документы: ${summary.documentsAgree} из ${summary.documentsCompared}`} />
        <Stat label="Основная права" value={percent(summary.mainCorrect, summary.mainVerified)} note={`${summary.mainCorrect} из ${summary.mainVerified} проверенных`} />
        <Stat label="Тень права" value={percent(summary.shadowCorrect, summary.shadowVerified)} note={`${summary.shadowCorrect} из ${summary.shadowVerified} проверенных`} />
        <Stat label="Расход" value={`$${Number(summary.costUsd).toFixed(2)}`} note={`тень: $${Number(summary.shadowCostUsd).toFixed(2)}`} />
        <Stat label="Время до итога" value={durationText(summary.seconds)} note={`тень: ${durationText(summary.shadowSeconds)}`} />
        <ShadowStat shadow={shadow} />
      </div>}
      <div className="alog-card">
        <div className="alog-filterbar">
          <label className="alog-period">
            <span>Период</span>
            <input type="date" value={filter.from} max={filter.to} onChange={(event) => change({ from: event.target.value })} aria-label="С" />
            <i>–</i>
            <input type="date" value={filter.to} min={filter.from} onChange={(event) => change({ to: event.target.value })} aria-label="По" />
          </label>
          <label className="alog-select">
            <span>Сотрудник</span>
            <select value={filter.userId} onChange={(event) => change({ userId: event.target.value })}>
              <option value="">все</option>
              {employees.map((employee) => <option key={employee.id} value={employee.id}>{employee.name || employee.email}</option>)}
            </select>
          </label>
          <Tabs label="Выборка" value={filter.only} options={onlyOptions} onChange={(value) => change({ only: value as LogFilter['only'] })} />
          <Tabs label="Режим" value={filter.mode} options={modeOptions} onChange={(value) => change({ mode: value })} />
          {loading && <span className="alog-loading" role="status">Загрузка…</span>}
        </div>
        <div className={loading && data ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
          <table className="sheet dir-table alog-table">
            <thead><tr>
              <th className="c-when">Когда</th><th className="c-user">Сотрудник</th><th className="c-name">Товар</th>
              <th className="c-model">Основная модель</th><th className="c-model">Теневая модель</th>
              <th className="c-applied">Код сотрудника</th><th className="c-verify">Проверка</th><th className="c-cmp">Сравнение</th>
            </tr></thead>
            <tbody>
              {!data || data.rows.length === 0
                ? <tr><td colSpan={8} className="dir-empty">{loading ? 'Загрузка…' : 'Подборов за период нет. Расширьте период или снимите фильтры.'}</td></tr>
                : data.rows.map((row) => <LogRowView key={row.id} row={row} onOpen={() => setSelected(row.id)} />)}
            </tbody>
          </table>
        </div>
        <div className="alog-more">
          <span>{rangeText(page, pageSize, total)}</span>
          {pages > 1 && <div>
            <button type="button" className="secondary" onClick={() => setPage(page - 1)} disabled={page <= 1 || loading}>← Новее</button>
            <button type="button" className="secondary" onClick={() => setPage(page + 1)} disabled={page >= pages || loading}>Старее →</button>
          </div>}
        </div>
      </div>
      <p className="dir-foot">Тень подбирает код параллельно с основной моделью и получает те же действия сотрудника: первый вопрос, уточнения, ответы, «подобрать заново» и «Документы для ввоза»; сотрудник её не видит. Правота считается по коду, который администратор подтвердил в базе решений. Щёлкните строку, чтобы открыть ход подбора обеих моделей.</p>
    </section>
    {selected !== null && <AgentLogEntryPanel id={selected} onClose={() => setSelected(null)} />}
  </main>
}

function Tabs({ label, value, options, onChange }: { label: string; value: string; options: { value: string; label: string }[]; onChange: (value: string) => void }) {
  return <div className="alog-tabs" role="group" aria-label={label}>
    <span>{label}</span>
    <div className="dir-seg">{options.map((option) => <button key={option.value} type="button" className={option.value === value ? 'active' : undefined} aria-pressed={option.value === value} onClick={() => onChange(option.value)}>{option.label}</button>)}</div>
  </div>
}

function Stat({ label, value, note }: { label: string; value: string; note: string }) {
  return <div className="alog-stat"><small>{label}</small><strong>{value}</strong><span>{note}</span></div>
}

// Тень: модель и расход против бюджета полоской; выключена — как её включить.
function ShadowStat({ shadow }: { shadow: LogPage['shadow'] }) {
  if (!shadow.enabled) return <div className="alog-stat shadow">
    <small>Тень</small>
    <strong><i className="alog-dot" aria-hidden="true" />выключена</strong>
    <span title="Задайте на сервере TNVED_AI_SHADOW_MODEL и TNVED_AI_SHADOW_BUDGET_USD">не настроена на сервере</span>
  </div>
  const spent = Number(shadow.spentUsd), budget = Number(shadow.budgetUsd)
  const share = budget > 0 ? Math.min(100, Math.round((spent * 100) / budget)) : 0
  return <div className="alog-stat shadow" title={`фото ${shadow.vision ? 'смотрит сама' : 'по описанию модели для фото'}`}>
    <small>Тень</small>
    <strong><i className="alog-dot on" aria-hidden="true" />{shadow.model}</strong>
    <span className="alog-bar" role="progressbar" aria-valuenow={share} aria-valuemin={0} aria-valuemax={100} aria-label="Расход бюджета тени"><i style={{ width: `${share}%` }} /></span>
    <span>${spent.toFixed(2)} из ${budget.toFixed(2)} · сегодня {shadow.today}{shadow.dailyLimit ? ` из ${shadow.dailyLimit}` : ''}</span>
  </div>
}

function when(iso: string) {
  const date = new Date(iso)
  return `${date.toLocaleDateString('ru-RU')} ${formatRuTime(date)}`
}

function ModelCell({ model }: { model: LogModel | null }) {
  if (!model) return <td className="c-model"><span className="alog-none">—</span></td>
  const outcome = modelOutcome(model)
  return <td className="c-model" title={model.model + (model.error ? `\n${model.error}` : '')}>
    {outcome.tone === 'ok' ? <code>{outcome.text}</code> : <span className={`alog-state ${outcome.tone}`}>{outcome.text}</span>}
    {model.status === 'skipped' ? model.error && <small>{model.error}</small> : <small>{metaText(model)}</small>}
  </td>
}

function LogRowView({ row, onOpen }: { row: LogRow; onOpen: () => void }) {
  const marks = comparison(row)
  return <tr onClick={onOpen} tabIndex={0} onKeyDown={(event) => { if (event.key === 'Enter') onOpen() }}>
    <td className="c-when"><time dateTime={row.createdAt}>{when(row.createdAt)}</time>{row.runs > 1 && <small>ходов: {row.runs}</small>}</td>
    <td className="c-user" title={row.user}>{row.user}</td>
    <td className="c-name"><span title={row.name}>{row.name || '—'}</span><small>{modeLabels[row.mode] ?? row.mode}</small></td>
    <ModelCell model={row.main} />
    <ModelCell model={row.shadow} />
    <td className="c-applied">{row.decision ? <code>{formatCode(row.decision.code)}</code> : <span className="alog-none">—</span>}</td>
    <td className="c-verify">{row.decision
      ? <span className={`status-dot ${decisionStatusTones[row.decision.status]}`}>{decisionStatusLabels[row.decision.status]}</span>
      : <span className="alog-none">—</span>}</td>
    <td className="c-cmp">{marks.length === 0 ? <span className="alog-none">—</span> : marks.map((mark) => <span key={mark.text} className={`status-dot ${mark.tone === 'bad' ? 'error' : mark.tone} alog-mark`}>{mark.text}</span>)}</td>
  </tr>
}
