import { useMemo, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { DirectoryPanel } from '../components/DirectoryPanel'
import { StatusToggle } from '../components/StatusToggle'
import { SynonymFields } from '../components/SynonymFields'
import { productDirectoryApi } from './productDirectoryApi'
import { emptyInput, toInput } from './productEntryForm'
import { TnvedPicker } from './TnvedPicker'
import { ProductDirectoryEntry, ProductDirectoryInput } from './types'
import './TnvedPicker.css'
import './ProductEntryPanel.css'

// Создание и редактирование карточки товара — белые карточки на сером фоне, как этапы сметы:
// сверху статус проверки, затем «Товар» (названия), «Код ТН ВЭД», «Цена в смете» и «Синонимы».
// В карточках строки «подпись слева — поле справа», как «Параметры расчёта» на этапе проверки.
export function ProductEntryPanel({ item, onClose, onSaved, readOnly = false }: { readOnly?: boolean; item?: ProductDirectoryEntry; onClose: () => void; onSaved: () => void }) {
  const initial = useMemo(() => item ? toInput(item) : emptyInput(), [item])
  const [form, setForm] = useState<ProductDirectoryInput>(initial)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const dirty = JSON.stringify(form) !== JSON.stringify(initial)
  function setField<K extends keyof ProductDirectoryInput>(field: K, value: ProductDirectoryInput[K]) { setForm((previous) => ({ ...previous, [field]: value })) }
  async function save() {
    if (!form.code) { setError('Выберите код ТН ВЭД'); return }
    setSaving(true); setError('')
    try { if (item) await productDirectoryApi.update(item.id, form); else await productDirectoryApi.create(form); onSaved() } catch (reason) { setError(errorMessage(reason)) } finally { setSaving(false) }
  }

  return <DirectoryPanel title={item ? item.name : 'Новый товар'} subtitle={item ? 'Карточка товара' : 'При выборе карточки в смете код, ставки и ИТС подставятся сами'} dirty={dirty} saving={saving} error={error} readOnly={readOnly} onClose={onClose} onSubmit={() => void save()}>
    <div className="pe-status">
      <span>Статус проверки</span>
      <StatusToggle value={form.verificationStatus} onChange={(value) => setField('verificationStatus', value)} />
    </div>

    <section className="pe-card">
      <h3>Товар</h3>
      <label className="drawer-field pe-row"><span>Наименование <b className="req">*</b></span><input value={form.name} onChange={(event) => setField('name', event.target.value)} maxLength={300} required placeholder="Например, перчатки рабочие трикотажные" autoFocus={!item} /></label>
      <label className="drawer-field pe-row"><span>На китайском</span><input value={form.nameZh} onChange={(event) => setField('nameZh', event.target.value)} maxLength={300} placeholder="Необязательно" /></label>
    </section>

    <section className="pe-card">
      <h3>Код ТН ВЭД <b className="req">*</b></h3>
      <div className="pe-pad">
        <TnvedPicker value={form.code} onChange={(code) => setField('code', code)} disabled={readOnly} />
      </div>
      <p className="pe-foot">Пошлина, НДС и антидемпинг в смете берутся по коду из номенклатуры ТН ВЭД ЕАЭС и реестра ЕЭК.</p>
    </section>

    <section className="pe-card">
      <h3>Цена в смете</h3>
      <label className="drawer-field pe-row"><span>ИТС</span><span className="drawer-suffix wide"><input inputMode="decimal" value={form.companyCoefficient} onChange={(event) => setField('companyCoefficient', event.target.value.replace(',', '.'))} placeholder="0.65" /><i>USD/кг</i></span></label>
      <label className="drawer-field pe-row"><span>Единица</span><input value={form.unit} onChange={(event) => setField('unit', event.target.value)} placeholder="шт." maxLength={64} /></label>
      <label className="drawer-field pe-row"><span>Категория</span><input value={form.category} onChange={(event) => setField('category', event.target.value)} /></label>
      <p className="pe-foot"><b>Стоимость = ИТС × вес нетто.</b> Без ИТС цену за единицу вводят в смете вручную.</p>
    </section>

    <SynonymFields value={form} hint="По ним карточка найдётся в смете. Каждый вариант — с новой строки или через запятую." onChange={(field, lines) => setField(field, lines)} />
  </DirectoryPanel>
}
