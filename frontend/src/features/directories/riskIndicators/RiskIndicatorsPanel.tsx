import { ChangeEvent, useEffect, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { plural } from '@/shared/lib/plural'
import { formatRuDate } from '@/shared/lib/dates'
import { DirectoryImportResult } from '../common/directoryTransfer'
import { RiskImportError, RiskProfile, riskIndicatorsApi } from './riskIndicatorsApi'
import '../kzList/KzListPanel.css'
import './RiskIndicatorsPanel.css'

const codes = (count: number) => `${formatDisplayNumber(count)} ${plural(count, 'код', 'кода', 'кодов')}`

// Загруженные профили. onRemove — режим страницы «Обновление данных»: кнопка «Удалить», без групп стран (они — в номенклатуре).
function ProfilesTable({ profiles, onRemove, busy }: { profiles: RiskProfile[]; onRemove?: (profile: RiskProfile) => void; busy?: boolean }) {
  return <table className="sheet sir-profiles">
    <thead><tr><th>Профиль риска</th><th className="sir-num">Кодов</th>{!onRemove && <th>Группы стран (колонки файла)</th>}<th>Загружен</th>{onRemove && <th aria-label="Действия" />}</tr></thead>
    <tbody>{profiles.map((profile) => <tr key={profile.id}>
      <td><b>{profile.number}</b>{profile.fileName && <small>{profile.fileName}</small>}</td>
      <td className="sir-num">{formatDisplayNumber(profile.codeCount)}</td>
      {!onRemove && <td><span className="sir-groups">{profile.groups.map((group, index) => <span key={`${index}-${group}`} className={group.split(', ').includes('CN') ? 'cn' : undefined}>{group}</span>)}</span></td>}
      <td><time dateTime={profile.importedAt}>{formatRuDate(profile.importedAt.slice(0, 10))}</time>{profile.importedByName && <small>{profile.importedByName}</small>}</td>
      {onRemove && <td><button type="button" className="button-link" onClick={() => onRemove(profile)} disabled={busy}>Удалить</button></td>}
    </tr>)}</tbody>
  </table>
}

function useProfiles() {
  const [profiles, setProfiles] = useState<RiskProfile[] | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    riskIndicatorsApi.list().then((list) => { if (active) setProfiles(list) }).catch((reason) => { if (active) setError(errorMessage(reason)) })
    return () => { active = false }
  }, [])
  return { profiles, setProfiles, error, setError }
}

// «СУР» на странице номенклатуры — стоимостные индикаторы риска КГД (приложения к профилям риска «24_прил CN» и т. п.),
// только просмотр: загружает администратор на странице «Обновление данных». button — в шапку, panel — под шапку.
// onOpen — чтобы страница закрыла соседнюю панель (антидемпинг): открыта только одна.
export function useRiskIndicatorsPanel({ onOpen }: { onOpen?: () => void }) {
  const [open, setOpen] = useState(false)
  const { profiles, error } = useProfiles()

  const total = profiles?.reduce((sum, profile) => sum + profile.codeCount, 0) ?? 0
  const button = <div className="dir-transfer">
    <button type="button" className={open ? 'secondary active' : 'secondary'} onClick={() => { if (!open) onOpen?.(); setOpen(!open) }} aria-expanded={open} title="Стоимостные индикаторы риска КГД: стоимость 1 кг товара в USD по кодам ТН ВЭД">СУР{profiles && profiles.length > 0 && ` · ${formatDisplayNumber(total)}`}</button>
  </div>

  const panel = open && <section className="dir-transfer-panel kz-panel">
    <div className="dir-transfer-row">
      <div>
        <strong>СУР — стоимостные индикаторы риска</strong>
        <p>Приложения к профилям риска КГД: стоимость 1 кг товара в USD по коду ТН ВЭД и группам стран. Смета в режиме «ТС по СУР» берёт индикатор кода для страны сметы (обычно CN). Если код есть в нескольких профилях, берётся больший индикатор.</p>
      </div>
      <button type="button" className="icon-button ghost dir-transfer-close" onClick={() => setOpen(false)} aria-label="Закрыть">×</button>
    </div>
    {profiles === null ? <p className="sir-empty">{error ? '' : 'Загрузка…'}</p>
      : profiles.length === 0 ? <p className="sir-empty">Профили ещё не загружены администратором.</p>
      : <ProfilesTable profiles={profiles} />}
    {error && <p className="dir-transfer-error" role="alert">{error}</p>}
  </section>

  return { button, panel, close: () => setOpen(false) }
}

// Загрузка и удаление профилей СУР (администратор, страница «Обновление данных»): meta — сводка и ссылка «Профили»,
// action — кнопка загрузки, extra — таблица профилей (по ссылке) и сообщения.
export function useRiskIndicatorsUpload() {
  const { profiles, setProfiles, error, setError } = useProfiles()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')
  const [rowErrors, setRowErrors] = useState<DirectoryImportResult['errors']>([])

  // Можно выбрать сразу несколько файлов (24, 25, 26): загружаются по очереди, один файл — один профиль.
  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const files = Array.from(event.target.files ?? [])
    event.target.value = ''
    if (files.length === 0) return
    setMessage(''); setError(''); setRowErrors([])
    const done: string[] = []
    try {
      for (const [index, file] of files.entries()) {
        setBusy(files.length > 1 ? `Загружаем ${index + 1} из ${files.length}…` : 'Загружаем…')
        try {
          const result = await riskIndicatorsApi.upload(file)
          done.push(`${result.replaced ? 'обновлён' : 'загружен'} профиль ${result.profile.number}: ${codes(result.profile.codeCount)}, для CN — ${formatDisplayNumber(result.defaultCountryCodes)}`)
        } catch (reason) {
          setError(`${file.name}: ${errorMessage(reason)}`)
          setRowErrors((reason as RiskImportError).result?.errors ?? [])
          break
        }
      }
      if (done.length > 0) setMessage(`${done.join('; ')}.`)
      setProfiles(await riskIndicatorsApi.list())
    } catch (reason) { setError(errorMessage(reason)) } finally { setBusy('') }
  }

  async function remove(profile: RiskProfile) {
    if (!window.confirm(`Удалить профиль ${profile.number} (${codes(profile.codeCount)})? Сметы по СУР перестанут брать из него индикаторы.`)) return
    setBusy('Удаляем…'); setMessage(''); setError(''); setRowErrors([])
    try {
      await riskIndicatorsApi.remove(profile.id)
      setProfiles((current) => current?.filter((item) => item.id !== profile.id) ?? null)
      setMessage(`Профиль ${profile.number} удалён.`)
    } catch (reason) { setError(errorMessage(reason)) } finally { setBusy('') }
  }

  // Сводка: сколько профилей и кодов, последняя загрузка.
  const latest = profiles?.reduce<RiskProfile | null>((last, profile) => !last || profile.importedAt > last.importedAt ? profile : last, null) ?? null
  const total = profiles?.reduce((sum, profile) => sum + profile.codeCount, 0) ?? 0
  const meta = profiles === null ? (error ? '' : 'Загрузка…') : !latest ? 'Ещё не загружено.' : <>
    <time dateTime={latest.importedAt}>{formatRuDate(latest.importedAt.slice(0, 10))}</time>{latest.importedByName && `, ${latest.importedByName}`} · {profiles.length} {plural(profiles.length, 'профиль', 'профиля', 'профилей')}, {codes(total)}
    {' · '}<button type="button" className="button-link" onClick={() => setOpen(!open)} aria-expanded={open}>{open ? 'Скрыть профили' : 'Профили'}</button>
  </>

  const action = <label className={busy ? 'kz-file disabled' : 'kz-file'} title="Excel КГД вида «24_прил CN.xlsx»: номер профиля в шапке, строка стран над таблицей. Профиль с тем же номером заменяется. Можно выбрать несколько файлов сразу">
    {busy || 'Загрузить .xlsx'}
    <input type="file" accept=".xlsx" multiple onChange={(event) => void upload(event)} disabled={busy !== ''} />
  </label>

  const showTable = open && profiles !== null && profiles.length > 0
  const extra = showTable || message || error || rowErrors.length > 0 ? <>
    {showTable && <ProfilesTable profiles={profiles} onRemove={(profile) => void remove(profile)} busy={busy !== ''} />}
    {message && <p className="dir-transfer-ok" role="status">{message}</p>}
    {error && <p className="dir-transfer-error" role="alert">{error}</p>}
    {rowErrors.length > 0 && <ul className="dir-transfer-errors">{rowErrors.map((item) => <li key={`${item.row}-${item.message}`}><b>Строка {item.row}</b> {item.message}</li>)}</ul>}
  </> : null

  return { meta, action, extra }
}
