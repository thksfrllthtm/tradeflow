import { useMemo, useState } from 'react'
import { TNVEDDirectoryEntry } from '../tnved/types'
import { errorMessage } from '@/shared/lib/errorMessage'
import { DirectoryPanel } from '../components/DirectoryPanel'
import { StatusToggle } from '../components/StatusToggle'
import { SynonymFields } from '../components/SynonymFields'
import { productDirectoryApi } from './productDirectoryApi'
import { emptyInput, toInput } from './productEntryForm'
import { TnvedPicker } from './TnvedPicker'
import { ProductDirectoryEntry, ProductDirectoryInput } from './types'
import './TnvedPicker.css'

// Создание и редактирование карточки товара: название, ИТС, привязка к коду ТН ВЭД, синонимы.
export function ProductEntryPanel({ item, tnved, onClose, onSaved, readOnly = false }: { readOnly?: boolean; item?: ProductDirectoryEntry; tnved: TNVEDDirectoryEntry[]; onClose: () => void; onSaved: () => void }) {
  const initial = useMemo(() => item ? toInput(item) : emptyInput(), [item])
  const [form, setForm] = useState<ProductDirectoryInput>(initial)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const dirty = JSON.stringify(form) !== JSON.stringify(initial)
  function setField<K extends keyof ProductDirectoryInput>(field: K, value: ProductDirectoryInput[K]) { setForm((previous) => ({ ...previous, [field]: value })) }
  async function save() {
    if (!form.tnvedDirectoryEntryId) { setError('Выберите код ТН ВЭД'); return }
    setSaving(true); setError('')
    try { if (item) await productDirectoryApi.update(item.id, form); else await productDirectoryApi.create(form); onSaved() } catch (reason) { setError(errorMessage(reason)) } finally { setSaving(false) }
  }

  return <DirectoryPanel title={item ? item.name : 'Новый товар'} subtitle={item ? 'Карточка товара' : 'При выборе карточки в смете код, ставки и ИТС подставятся сами'} dirty={dirty} saving={saving} error={error} readOnly={readOnly} onClose={onClose} onSubmit={() => void save()}>
    <section className="drawer-section">
      <h3>Товар</h3>
      <label className="drawer-field"><span>Наименование <b className="req">*</b></span><input value={form.name} onChange={(event) => setField('name', event.target.value)} maxLength={300} required placeholder="Например, перчатки рабочие трикотажные" autoFocus={!item} /></label>
      <label className="drawer-field"><span>Наименование на китайском</span><input value={form.nameZh} onChange={(event) => setField('nameZh', event.target.value)} maxLength={300} placeholder="Необязательно" /></label>
      <div className="drawer-row">
        <label className="drawer-field"><span>ИТС</span><span className="drawer-suffix wide"><input inputMode="decimal" value={form.companyCoefficient} onChange={(event) => setField('companyCoefficient', event.target.value.replace(',', '.'))} placeholder="0.65" /><i>USD/кг</i></span></label>
        <label className="drawer-field"><span>Единица</span><input value={form.unit} onChange={(event) => setField('unit', event.target.value)} placeholder="шт." maxLength={64} /></label>
        <label className="drawer-field"><span>Категория</span><input value={form.category} onChange={(event) => setField('category', event.target.value)} /></label>
      </div>
      <p className="drawer-hint">Стоимость в смете = ИТС × вес нетто. Если ИТС не указан, цену за единицу вводят в смете вручную.</p>
      <div className="drawer-field drawer-status-field"><span>Статус проверки</span><StatusToggle value={form.verificationStatus} onChange={(value) => setField('verificationStatus', value)} /></div>
    </section>

    <section className="drawer-section">
      <h3>Код ТН ВЭД <b className="req">*</b></h3>
      <TnvedPicker entries={tnved} value={form.tnvedDirectoryEntryId} onChange={(id) => setField('tnvedDirectoryEntryId', id)} />
      <p className="drawer-hint">Код, пошлина, НДС и антидемпинг берутся из справочника ТН ВЭД.</p>
    </section>

    <SynonymFields value={form} hint="Другие названия этого товара — по ним карточка найдётся в смете." onChange={(field, lines) => setField(field, lines)} />
  </DirectoryPanel>
}
