import { ReactNode, useEffect, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { formatRuDate } from '@/shared/lib/dates'
import { decisionSourceLabels, decisionsApi } from './decisionsApi'
import '@/shared/styles/drawer.css'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { nameParts, formatCode } from '@/shared/tnved/codes'
import { CodeDecision, decisionStatusLabels } from '@/shared/tnved/decisions'
import { CopyButton } from '@/shared/components/CopyCode/CopyButton'

type Props = {
  id: number
  admin: boolean
  onClose: () => void
  onChanged: (decision: CodeDecision | null) => void
}

// Карточка записи базы решений: фото, как товар был в инвойсе, итоговый код и обоснование.
// Администратор правит код и наименование, отмечает «Проверено» или «Отклонено», удаляет.
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
  const invoiceMain = decision?.invoiceName || decision?.supplierName || ''
  const supplierExtra = decision?.supplierName && decision.invoiceName && decision.supplierName !== decision.invoiceName ? decision.supplierName : ''
  // Как этапы сметы: шапка — код крупно (копируется) и статус; ниже — фото, «Итог» и «Как было в инвойсе»
  // строками «подпись — значение — копировать». Администратор правит значения в полях; остальные только
  // читают и копируют (права проверяет сервер: правка, проверка и удаление — requireAdmin).
  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <aside className="drawer dc-drawer" role="dialog" aria-modal="true" aria-labelledby="dc-title">
      <header className="drawer-head">
        <div className="dc-head">
          <h2 id="dc-title"><span className="dc-head-label">Решение</span>{decision ? <CopyCode code={decision.code} /> : '…'}</h2>
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
          {decision.photos.length > 0 && <section className="drawer-section dc-photo-section">
            <div className="dc-photos">{decision.photos.map((photo, index) => <button key={index} type="button" className="dc-photo" onClick={() => setPhotoIndex(index)} title="Открыть фото крупно"><img src={photo} alt={`Фото ${index + 1}`} /></button>)}</div>
            <p className="dc-sub">{decision.photoDescription ? <><b>На фото (ИИ):</b> {decision.photoDescription}</> : 'Описание фото для поиска похожих ИИ составит в течение нескольких минут.'}</p>
          </section>}

          <section className="drawer-section">
            <h3>Итог</h3>
            {admin && <Field label="Код ТН ВЭД" copy={code.replace(/\D/g, '')} copyLabel="код">
              <input className="mono" value={code} onChange={(event) => setCode(event.target.value)} inputMode="numeric" aria-label="Код ТН ВЭД" />
              {!codeValid && <span className="dc-field-error">Код — 10 цифр</span>}
            </Field>}
            <Field label="Наименование для декларации" copy={admin ? finalName : decision.finalName} copyLabel="наименование" empty={!admin && !decision.finalName}>
              {admin ? <textarea rows={2} value={finalName} onChange={(event) => setFinalName(event.target.value)} aria-label="Наименование для декларации" /> : <p className="dc-value">{decision.finalName}</p>}
            </Field>
            {(admin || decision.reason) && <Field label="Почему этот код" copy={admin ? reason : decision.reason} copyLabel="обоснование">
              {admin ? <textarea rows={3} value={reason} onChange={(event) => setReason(event.target.value)} placeholder="Например: металлическая канистра для топлива, стенка 0,6 мм" aria-label="Почему этот код" /> : <p className="dc-value">{decision.reason}</p>}
            </Field>}
            {decision.nomenclature
              ? <div className="dc-nomen">
                <span className="dc-nomen-name">{nameParts(decision.nomenclature.name).slice(-2).join(' › ')}</span>
                <span className="dc-nomen-rates">Пошлина ЕТТ <b>{decision.nomenclature.tariffText || 'не указана'}</b> · НДС <b>{decision.nomenclature.vatPercent}%</b>
                  {decision.nomenclature.detailsUrl && <a href={decision.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}</span>
              </div>
              : <p className="dc-nomen warn">Кода нет в текущей номенклатуре ТН ВЭД ЕАЭС.</p>}
            {decision.quote && <blockquote className="dc-quote">{decision.quote}</blockquote>}
            {decision.agentCode && <p className={decision.agentCorrect === false || agentChanged ? 'dc-warn dc-agent' : 'dc-sub dc-agent'}>
              ИИ предлагал <code>{formatCode(decision.agentCode)}</code>{decision.agentCorrect === true ? ' — верно' : decision.agentCorrect === false ? ' — администратор исправил' : agentChanged ? ' — при проверке будет отмечено, что ИИ ошибся' : ''}
            </p>}
          </section>

          <section className="drawer-section">
            <h3>Как было в инвойсе</h3>
            <Field label="Наименование в инвойсе" copy={invoiceMain} copyLabel="наименование из инвойса" empty={!invoiceMain}>
              <p className="dc-value">{invoiceMain}</p>
            </Field>
            {supplierExtra && <Field label="У поставщика" copy={supplierExtra} copyLabel="наименование поставщика"><p className="dc-value">{supplierExtra}</p></Field>}
            {(decision.unitNetKg || decision.unitPriceUsd) && <div className="dc-facts">
              {decision.unitNetKg && <span>Нетто 1 шт <b>{decision.unitNetKg} кг</b></span>}
              {decision.unitPriceUsd && <span>Цена 1 шт <b>{decision.unitPriceUsd} $</b></span>}
            </div>}
          </section>

          {decision.rejectedCodes.length > 0 && <section className="drawer-section">
            <h3>Рассматривались и не подошли</h3>
            <ul className="dc-rejected">{decision.rejectedCodes.map((item) => <li key={item.code}><code>{formatCode(item.code)}</code><span>{item.why}</span></li>)}</ul>
          </section>}

          {(admin || decision.note) && <section className="drawer-section">
            <Field label="Заметка" copy={admin ? note : decision.note} copyLabel="заметку">
              {admin ? <textarea rows={2} value={note} onChange={(event) => setNote(event.target.value)} placeholder="Например: подтверждено декларацией №…" aria-label="Заметка" /> : <p className="dc-value">{decision.note}</p>}
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

// Строка карточки: подпись и «копировать» сверху, под ними значение (текст или поле у администратора).
// empty — значения нет: вместо него серый прочерк, копировать нечего.
function Field({ label, copy, copyLabel, empty, children }: { label: string; copy: string; copyLabel: string; empty?: boolean; children: ReactNode }) {
  return <div className="dc-field">
    <div className="dc-field-head"><span>{label}</span>{!empty && copy.trim() && <CopyButton text={copy} label={copyLabel} />}</div>
    {empty ? <p className="dc-value dc-none">—</p> : children}
  </div>
}
