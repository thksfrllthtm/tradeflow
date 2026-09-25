import { useMemo, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { DirectoryPanel } from '../components/DirectoryPanel'
import { StatusToggle } from '../components/StatusToggle'
import { SynonymFields } from '../components/SynonymFields'
import { RateFields } from './RateFields'
import { tnvedDirectoryApi } from './tnvedDirectoryApi'
import { emptyInput, entryToInput } from './tnvedEntryForm'
import { TNVEDDirectoryEntry, TNVEDDirectoryInput, TNVEDRate } from './types'

// Создание и редактирование записи ТН ВЭД: код и описание, ставки, синонимы.
export function TNVEDEntryPanel({ item, onClose, onSaved, readOnly = false }: { readOnly?: boolean; item?: TNVEDDirectoryEntry; onClose: () => void; onSaved: () => void }) {
  const initial = useMemo(() => item ? entryToInput(item) : emptyInput(), [item])
  const [form, setForm] = useState<TNVEDDirectoryInput>(initial)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const dirty = JSON.stringify(form) !== JSON.stringify(initial)
  function setField<K extends keyof TNVEDDirectoryInput>(field: K, value: TNVEDDirectoryInput[K]) { setForm((previous) => ({ ...previous, [field]: value })) }
  function setRate(field: 'duty' | 'vat', rate: Partial<TNVEDRate>) { setForm((previous) => ({ ...previous, [field]: { ...previous[field], ...rate } })) }
  async function save() {
    setSaving(true); setError('')
    try { if (item) await tnvedDirectoryApi.update(item.id, form); else await tnvedDirectoryApi.create(form); onSaved() } catch (reason) { setError(errorMessage(reason)) } finally { setSaving(false) }
  }

  return <DirectoryPanel title={item ? `Код ${item.code}` : 'Новый код ТН ВЭД'} subtitle={item ? 'Изменения попадут в новые подборы кода; уже сохранённые сметы не меняются' : 'Код, описание и ставки для предварительного расчёта'} dirty={dirty} saving={saving} error={error} readOnly={readOnly} onClose={onClose} onSubmit={() => void save()}>
    <section className="drawer-section">
      <h3>Код и описание</h3>
      <div className="drawer-row">
        <label className="drawer-field"><span>Код ТН ВЭД <b className="req">*</b><em>{form.code.length}/10</em></span><input className="mono" value={form.code} onChange={(event) => setField('code', event.target.value.replace(/\D/g, '').slice(0, 10))} inputMode="numeric" maxLength={10} placeholder="0000000000" required pattern="\d{10}" title="10 цифр" autoFocus={!item} /></label>
        <div className="drawer-field drawer-status-field"><span>Статус проверки</span><StatusToggle value={form.verificationStatus} onChange={(value) => setField('verificationStatus', value)} /></div>
      </div>
      <label className="drawer-field"><span>Описание на русском</span><textarea rows={3} value={form.name} onChange={(event) => setField('name', event.target.value)} maxLength={300} placeholder="Как в тарифе: например, «Перчатки трикотажные…»" /></label>
      <label className="drawer-field"><span>Описание на китайском</span><input value={form.nameZh} onChange={(event) => setField('nameZh', event.target.value)} maxLength={300} placeholder="Необязательно" /></label>
      <p className="drawer-hint">Код проверяется только по формату. Существование кода и применимость к товару нужно проверить отдельно.</p>
    </section>

    <section className="drawer-section">
      <h3>Ставки</h3>
      <RateFields label="Пошлина" rate={form.duty} onChange={(rate) => setRate('duty', rate)} />
      <RateFields label="НДС" rate={form.vat} onChange={(rate) => setRate('vat', rate)} vat />
      <label className="drawer-field"><span>Антидемпинговая пошлина<em>необязательно</em></span><span className="drawer-suffix"><input inputMode="decimal" value={form.antiDumpingPercent} onChange={(event) => setField('antiDumpingPercent', event.target.value.replace(',', '.'))} placeholder="0" /><i>%</i></span></label>
      <p className="drawer-hint">Антидемпинг добавляется к пошлине и входит в базу НДС. ИТС задаётся в карточке товара.</p>
    </section>

    <SynonymFields value={form} hint="Помогают найти код по названию из инвойса." onChange={(field, lines) => setField(field, lines)} />
  </DirectoryPanel>
}
