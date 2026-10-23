import { ReactNode, useEffect, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { formatRuDate } from '@/shared/lib/dates'
import { decisionSourceLabels, decisionsApi } from './decisionsApi'
import '@/shared/styles/drawer.css'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { nameParts, formatCode } from '@/shared/tnved/codes'
import { CodeDecision, PhotoFeatures, decisionStatusLabels } from '@/shared/tnved/decisions'
import { CopyButton } from '@/shared/components/CopyCode/CopyButton'

type Props = {
  id: number
  admin: boolean
  onClose: () => void
  onChanged: (decision: CodeDecision | null) => void
}

// Карточка записи базы решений читается сверху вниз как ответ на вопрос «что это за товар и какой у него
// код»: в шапке — наименование из инвойса и статус; «Товар» — фото, описание ИИ по полям, поставщик,
// цена; «Код ТН ВЭД» — код крупно, строка номенклатуры со ставками, наименование для декларации и
// обоснование; затем отвергнутые коды и заметка (откуда запись). Администратор правит поля прямо в
// карточке, остальные читают и копируют (права проверяет сервер: requireAdmin).
export function DecisionDrawer({ id, admin, onClose, onChanged }: Props) {
  const [decision, setDecision] = useState<CodeDecision | null>(null)
  const [code, setCode] = useState('')
  const [finalName, setFinalName] = useState('')
  const [reason, setReason] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  // Фото открываются здесь же: data:-адрес Chrome не даёт открыть в новой вкладке.
  const [photoIndex, setPhotoIndex] = useState<number | null>(null)

  useEffect(() => {
    let active = true
    decisionsApi.get(id).then((value) => {
      if (!active) return
      setDecision(value); setCode(formatCode(value.code)); setFinalName(value.finalName); setReason(value.reason); setNote(value.note)
    }).catch((reason) => { if (active) setError(errorMessage(reason)) })
    return () => { active = false }
  }, [id])

  useEffect(() => {
    // Пока открыто фото, Esc закрывает только его.
    if (photoIndex !== null) return
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose, photoIndex])

  const edit = () => ({ code: code.replace(/\D/g, ''), finalName, reason, note })
  const codeValid = /^\d{10}$/.test(code.replace(/\D/g, ''))

  async function run(action: () => Promise<CodeDecision | void>, closeAfter = false) {
    setBusy(true); setError('')
    try {
      const result = await action()
      if (result) {
        setDecision(result); setCode(formatCode(result.code))
      }
      onChanged(result ?? null)
      if (closeAfter) onClose()
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  const agentChanged = decision?.agentCode && decision.agentCode !== code.replace(/\D/g, '')
  const title = decision?.invoiceName || decision?.supplierName || decision?.finalName || ''
  const supplierExtra = decision?.supplierName && decision.invoiceName && decision.supplierName !== decision.invoiceName ? decision.supplierName : ''
  const hasFacts = Boolean(supplierExtra || decision?.unitNetKg || decision?.unitPriceUsd)

  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <aside className="drawer dc-drawer" role="dialog" aria-modal="true" aria-labelledby="dc-title">
      <header className="drawer-head">
        <div className="dc-head">
          <h2 id="dc-title" className="dc-title">{decision ? <><span>{title || 'Без наименования'}</span>{title && <CopyButton text={title} label="наименование" />}</> : 'Загрузка…'}</h2>
          {decision && <p className="dc-head-meta">
            <span className={`dc-status ${decision.status}`}>{decisionStatusLabels[decision.status]}</span>
            <span>{decisionSourceLabels[decision.source]}{decision.createdByName && ` · ${decision.createdByName}`} · {formatRuDate(decision.createdAt.slice(0, 10))}</span>
            {decision.verifiedByName && decision.verifiedAt && <span>проверил {decision.verifiedByName} · {formatRuDate(decision.verifiedAt.slice(0, 10))}</span>}
          </p>}
        </div>
        <button type="button" className="drawer-close" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)">×</button>
      </header>
      <div className="drawer-body">
        {error && <p className="dc-error" role="alert">{error}</p>}
        {!decision && !error && <p className="dc-muted">Загрузка…</p>}
        {decision && <>
          {(decision.photos.length > 0 || hasFacts) && <section className="drawer-section">
            <h3>Товар</h3>
            {decision.photos.length > 0 && <div className="dc-photos">{decision.photos.map((photo, index) => <button key={index} type="button" className="dc-photo" onClick={() => setPhotoIndex(index)} title="Открыть фото крупно"><img src={photo} alt={`Фото ${index + 1}`} /></button>)}</div>}
            {decision.photos.length > 0 && (decision.photoFeatures
              ? <PhotoDescription features={decision.photoFeatures} />
              : <p className="dc-muted">Описание фото для поиска похожих ИИ составит в течение нескольких минут.</p>)}
            {hasFacts && <dl className="dc-facts">
              {supplierExtra && <div><dt>У поставщика</dt><dd>{supplierExtra}<CopyButton text={supplierExtra} label="наименование поставщика" /></dd></div>}
              {decision.unitPriceUsd && <div><dt>Цена за 1 шт</dt><dd>{decision.unitPriceUsd} $</dd></div>}
              {decision.unitNetKg && <div><dt>Нетто 1 шт</dt><dd>{decision.unitNetKg} кг</dd></div>}
            </dl>}
          </section>}

          <section className="drawer-section">
            <h3>Код ТН ВЭД</h3>
            {admin
              ? <div className="dc-field">
                <input className="dc-code-input" value={code} onChange={(event) => setCode(event.target.value)} inputMode="numeric" aria-label="Код ТН ВЭД" />
                {!codeValid && <span className="dc-field-error">Код — 10 цифр</span>}
              </div>
              : <CopyCode code={decision.code} className="dc-code-big" />}
            {decision.nomenclature
              ? <div className="dc-nomen">
                <span className="dc-nomen-name">{nameParts(decision.nomenclature.name).slice(-2).join(' › ')}</span>
                <span className="dc-nomen-rates">Пошлина ЕТТ <b>{decision.nomenclature.tariffText || 'не указана'}</b> · НДС <b>{decision.nomenclature.vatPercent}%</b>
                  {decision.nomenclature.detailsUrl && <a href={decision.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}</span>
              </div>
              : <p className="dc-nomen warn">Кода нет в текущей номенклатуре ТН ВЭД ЕАЭС.</p>}
            {decision.agentCode && <p className={decision.agentCorrect === false || agentChanged ? 'dc-warn dc-agent' : 'dc-sub dc-agent'}>
              ИИ предлагал <code>{formatCode(decision.agentCode)}</code>{decision.agentCorrect === true ? ' — верно' : decision.agentCorrect === false ? ' — администратор исправил' : agentChanged ? ' — при проверке будет отмечено, что ИИ ошибся' : ''}
            </p>}
            <Field label="Наименование для декларации" hint={admin ? 'рус / кит, как пишется в графе 31' : ''} copy={admin ? finalName : decision.finalName} copyLabel="наименование" empty={!admin && !decision.finalName}>
              {admin ? <textarea rows={2} value={finalName} onChange={(event) => setFinalName(event.target.value)} placeholder="Не заполнено" aria-label="Наименование для декларации" /> : <p className="dc-value">{decision.finalName}</p>}
            </Field>
            {(admin || decision.reason || decision.quote) && <Field label="Почему этот код" hint={admin ? 'признак товара, который решил выбор' : ''} copy={admin ? reason : decision.reason} copyLabel="обоснование" empty={!admin && !decision.reason && !decision.quote}>
              {admin ? <textarea rows={2} value={reason} onChange={(event) => setReason(event.target.value)} placeholder="Не заполнено" aria-label="Почему этот код" /> : decision.reason ? <p className="dc-value">{decision.reason}</p> : null}
              {decision.quote && <blockquote className="dc-quote">{decision.quote}</blockquote>}
            </Field>}
          </section>

          {decision.rejectedCodes.length > 0 && <section className="drawer-section">
            <h3>Рассматривались и не подошли</h3>
            <ul className="dc-rejected">{decision.rejectedCodes.map((item) => <li key={item.code}><code>{formatCode(item.code)}</code><span>{item.why}</span></li>)}</ul>
          </section>}

          {(admin || decision.note) && <section className="drawer-section">
            <h3>Заметка</h3>
            <Field label="" hint={admin ? 'откуда запись, чем подтверждён код' : ''} copy={admin ? note : decision.note} copyLabel="заметку">
              {admin ? <textarea rows={4} value={note} onChange={(event) => setNote(event.target.value)} placeholder="Не заполнено" aria-label="Заметка" /> : <p className="dc-value dc-note">{decision.note}</p>}
            </Field>
          </section>}
        </>}
      </div>
      {admin && decision && <footer className="drawer-foot dc-foot">
        <button type="button" className="dc-btn primary" onClick={() => void run(() => decisionsApi.verify(decision.id, edit()))} disabled={busy || !codeValid}>{decision.status === 'verified' ? 'Сохранить' : 'Отметить «Проверено»'}</button>
        {decision.status !== 'verified' && <button type="button" className="dc-btn" onClick={() => void run(() => decisionsApi.update(decision.id, edit()))} disabled={busy || !codeValid}>Сохранить без проверки</button>}
        {decision.status !== 'rejected' && <button type="button" className="dc-btn" onClick={() => void run(() => decisionsApi.reject(decision.id, note))} disabled={busy}>Отклонить</button>}
        <button type="button" className="dc-btn danger" onClick={() => { if (window.confirm('Удалить запись из базы решений?')) void run(() => decisionsApi.remove(decision.id), true) }} disabled={busy}>Удалить</button>
      </footer>}
    </aside>
    {photoIndex !== null && decision && decision.photos.length > 0 && <PhotoLightbox photos={decision.photos} startIndex={photoIndex} onClose={() => setPhotoIndex(null)} />}
  </div>
}

// Описание фото от ИИ по полям: подпись слева, значение справа. Пустые поля не показываются.
function PhotoDescription({ features }: { features: PhotoFeatures }) {
  const list = (values?: string[]) => (values ?? []).filter(Boolean).join(', ')
  const rows: [string, string][] = [
    ['Вид', features.kind],
    ['Также', list(features.synonyms)],
    ['Назначение', features.purpose ?? ''],
    ['Материал', list(features.material)],
    ['Форма', list(features.shape)],
    ['Особенности', list(features.features)],
    ['Цвет', list(features.color)],
  ]
  return <div className="dc-desc-wrap">
    <dl className="dc-desc" aria-label="Описание фото от ИИ">
      {rows.filter(([, value]) => value).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}
    </dl>
    <p className="dc-desc-note">Составил ИИ по фото, для поиска похожих товаров</p>
  </div>
}

// Строка карточки: подпись (и подсказка администратору) с «копировать» справа, под ними значение —
// текст или поле. empty — значения нет: серый прочерк, копировать нечего.
function Field({ label, hint, copy, copyLabel, empty, children }: { label: string; hint?: string; copy: string; copyLabel: string; empty?: boolean; children: ReactNode }) {
  return <div className="dc-field">
    {(label || hint || (!empty && copy.trim())) && <div className="dc-field-head"><span>{label}{hint && <small>{hint}</small>}</span>{!empty && copy.trim() && <CopyButton text={copy} label={copyLabel} />}</div>}
    {empty ? <p className="dc-value dc-none">—</p> : children}
  </div>
}
