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

// «СИР» — стоимостные индикаторы риска КГД (приложения к профилям риска «24_прил CN» и т. п.).
// Список профилей видят все; загрузка файлов и удаление — администратор. button — в шапку, panel — под шапку.
export function useRiskIndicatorsPanel({ admin }: { admin: boolean }) {
  const [open, setOpen] = useState(false)
  const [profiles, setProfiles] = useState<RiskProfile[] | null>(null)
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [rowErrors, setRowErrors] = useState<DirectoryImportResult['errors']>([])

  useEffect(() => {
    let active = true
    riskIndicatorsApi.list().then((list) => { if (active) setProfiles(list) }).catch((reason) => { if (active) setError(errorMessage(reason)) })
    return () => { active = false }
  }, [])

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
    if (!window.confirm(`Удалить профиль ${profile.number} (${codes(profile.codeCount)})? Сметы по СИР перестанут брать из него индикаторы.`)) return
    setBusy('Удаляем…'); setMessage(''); setError(''); setRowErrors([])
    try {
      await riskIndicatorsApi.remove(profile.id)
      setProfiles((current) => current?.filter((item) => item.id !== profile.id) ?? null)
      setMessage(`Профиль ${profile.number} удалён.`)
    } catch (reason) { setError(errorMessage(reason)) } finally { setBusy('') }
  }

  const total = profiles?.reduce((sum, profile) => sum + profile.codeCount, 0) ?? 0
  const button = <div className="dir-transfer">
    <button type="button" className={open ? 'dir-transfer-button active' : 'dir-transfer-button'} onClick={() => setOpen(!open)} aria-expanded={open} title="Стоимостные индикаторы риска КГД: стоимость 1 кг товара в USD по кодам ТН ВЭД">СИР{profiles && profiles.length > 0 && ` · ${formatDisplayNumber(total)}`}</button>
  </div>

  const panel = open && <section className="dir-transfer-panel kz-panel">
    <div className="dir-transfer-row">
      <div>
        <strong>СИР — стоимостные индикаторы риска</strong>
        <p>Приложения к профилям риска КГД: стоимость 1 кг товара в USD по коду ТН ВЭД и группам стран. Смета в режиме «ТС по СИР» берёт индикатор кода для страны сметы (обычно CN). Если код есть в нескольких профилях, берётся больший индикатор.</p>
      </div>
      <button type="button" className="dir-transfer-close" onClick={() => setOpen(false)} aria-label="Закрыть">×</button>
    </div>
    {profiles === null ? <p className="sir-empty">{error ? '' : 'Загрузка…'}</p>
      : profiles.length === 0 ? <p className="sir-empty">{admin ? 'Профили ещё не загружены. Загрузите файлы «24_прил CN.xlsx», «25_прил CN.xlsx», «26_прил CN.xlsx».' : 'Профили ещё не загружены администратором.'}</p>
      : <table className="sir-profiles">
        <thead><tr><th>Профиль риска</th><th className="sir-num">Кодов</th><th>Группы стран (колонки файла)</th><th>Загружен</th>{admin && <th aria-label="Действия" />}</tr></thead>
        <tbody>{profiles.map((profile) => <tr key={profile.id}>
          <td><b>{profile.number}</b>{profile.fileName && <small>{profile.fileName}</small>}</td>
          <td className="sir-num">{formatDisplayNumber(profile.codeCount)}</td>
          <td><span className="sir-groups">{profile.groups.map((group, index) => <span key={`${index}-${group}`} className={group.split(', ').includes('CN') ? 'cn' : undefined}>{group}</span>)}</span></td>
          <td>{formatRuDate(profile.importedAt.slice(0, 10))}{profile.importedByName && <small>{profile.importedByName}</small>}</td>
          {admin && <td><button type="button" className="button-link" onClick={() => void remove(profile)} disabled={busy !== ''}>Удалить</button></td>}
        </tr>)}</tbody>
      </table>}
    {admin && <div className="kz-step">
      <span className="kz-step-number">↑</span>
      <div><b>Загрузить файлы профилей</b><p>Excel КГД вида «24_прил CN.xlsx»: номер профиля в шапке, строка стран над таблицей. Профиль с тем же номером заменяется. Можно выбрать несколько файлов сразу.</p></div>
      <label className={busy ? 'kz-file primary disabled' : 'kz-file primary'}>
        {busy || 'Выбрать .xlsx'}
        <input type="file" accept=".xlsx" multiple onChange={(event) => void upload(event)} disabled={busy !== ''} />
      </label>
    </div>}
    {message && <p className="dir-transfer-ok" role="status">{message}</p>}
    {error && <p className="dir-transfer-error" role="alert">{error}</p>}
    {rowErrors.length > 0 && <ul className="dir-transfer-errors">{rowErrors.map((item) => <li key={`${item.row}-${item.message}`}><b>Строка {item.row}</b> {item.message}</li>)}</ul>}
  </section>

  return { button, panel }
}
