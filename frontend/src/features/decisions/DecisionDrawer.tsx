import { useEffect, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { formatRuDate } from '@/shared/lib/dates'
import { decisionSourceLabels, decisionsApi } from './decisionsApi'
import '@/shared/styles/drawer.css'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { nameParts, formatCode } from '@/shared/tnved/codes'
import { CodeDecision, decisionStatusLabels } from '@/shared/tnved/decisions'

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
  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <aside className="drawer dc-drawer" role="dialog" aria-modal="true" aria-labelledby="dc-title">
      <header className="drawer-head">
        <div>
          <h2 id="dc-title">Решение · {decision ? <code>{formatCode(decision.code)}</code> : '…'}</h2>
          {decision && <p>
            <span className={`dc-status ${decision.status}`}>{decisionStatusLabels[decision.status]}</span>
            {' '}{decisionSourceLabels[decision.source]}{decision.createdByName && ` · ${decision.createdByName}`} · {formatRuDate(decision.createdAt.slice(0, 10))}
            {decision.verifiedByName && decision.verifiedAt && ` · проверил ${decision.verifiedByName} ${formatRuDate(decision.verifiedAt.slice(0, 10))}`}
          </p>}
        </div>
        <button type="button" className="drawer-close" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)">×</button>
      </header>
      <div className="drawer-body">
        {error && <p className="dc-error" role="alert">{error}</p>}
        {!decision && !error && <p className="dc-muted">Загрузка…</p>}
        {decision && <>
          {decision.photos.length > 0 && <section className="drawer-section"><div className="dc-photos">{decision.photos.map((photo, index) => <button key={index} type="button" className="dc-photo" onClick={() => setPhotoIndex(index)} title="Открыть фото крупно"><img src={photo} alt={`Фото ${index + 1}`} /></button>)}</div>
            <p className="dc-sub">{decision.photoDescription ? <>На фото (ИИ): {decision.photoDescription}</> : 'Описание фото для поиска похожих ИИ составит в течение нескольких минут.'}</p></section>}
          <section className="drawer-section">
            <h3>Как было в инвойсе</h3>
            <p className="dc-text">{decision.invoiceName || '—'}</p>
            {decision.supplierName && decision.supplierName !== decision.invoiceName && <p className="dc-sub">{decision.supplierName}</p>}
            {(decision.unitNetKg || decision.unitPriceUsd) && <p className="dc-sub">{decision.unitNetKg && `1 шт: ${decision.unitNetKg} кг`}{decision.unitNetKg && decision.unitPriceUsd && ' · '}{decision.unitPriceUsd && `${decision.unitPriceUsd} $`}</p>}
          </section>
          <section className="drawer-section">
            <h3>Итог</h3>
            {admin ? <>
              <label className="drawer-field"><span>Код ТН ВЭД</span><input className="mono" value={code} onChange={(event) => setCode(event.target.value)} inputMode="numeric" /></label>
              <label className="drawer-field"><span>Наименование для декларации</span><textarea rows={2} value={finalName} onChange={(event) => setFinalName(event.target.value)} /></label>
              <label className="drawer-field"><span>Почему этот код</span><textarea rows={3} value={reason} onChange={(event) => setReason(event.target.value)} /></label>
            </> : <>
              <p className="dc-code"><CopyCode code={decision.code} /></p>
              {decision.finalName && <p className="dc-text">{decision.finalName}</p>}
              {decision.reason && <p className="dc-sub">{decision.reason}</p>}
            </>}
            {decision.nomenclature ? <p className="dc-sub">{nameParts(decision.nomenclature.name).slice(-2).join(' › ')} · пошлина ЕТТ {decision.nomenclature.tariffText || 'не указана'} · НДС {decision.nomenclature.vatPercent}%{decision.nomenclature.detailsUrl && <> · <a href={decision.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a></>}</p>
              : <p className="dc-warn">Кода нет в текущей номенклатуре ТН ВЭД ЕАЭС.</p>}
            {decision.quote && <blockquote className="dc-quote">{decision.quote}</blockquote>}
            {decision.agentCode && <p className={decision.agentCorrect === false || agentChanged ? 'dc-warn' : 'dc-sub'}>
              ИИ предлагал <code>{formatCode(decision.agentCode)}</code>{decision.agentCorrect === true ? ' — верно' : decision.agentCorrect === false ? ' — администратор исправил' : agentChanged ? ' — при проверке будет отмечено, что ИИ ошибся' : ''}
            </p>}
          </section>
          {decision.rejectedCodes.length > 0 && <section className="drawer-section">
            <h3>Рассматривались и не подошли</h3>
            {decision.rejectedCodes.map((item) => <p key={item.code} className="dc-sub"><code>{formatCode(item.code)}</code> — {item.why}</p>)}
          </section>}
          {(admin || decision.note) && <section className="drawer-section">
            <h3>Заметка</h3>
            {admin ? <label className="drawer-field"><textarea rows={2} value={note} onChange={(event) => setNote(event.target.value)} placeholder="Например: подтверждено декларацией №…" /></label> : <p className="dc-sub">{decision.note}</p>}
          </section>}
        </>}
      </div>
      {admin && decision && <footer className="drawer-foot">
        <button type="button" onClick={() => void run(() => decisionsApi.verify(decision.id, edit()))} disabled={busy || !codeValid}>{decision.status === 'verified' ? 'Сохранить' : 'Проверено'}</button>
        {decision.status !== 'rejected' && <button type="button" className="secondary" onClick={() => void run(() => decisionsApi.reject(decision.id, note))} disabled={busy}>Отклонить</button>}
        {decision.status !== 'verified' && <button type="button" className="secondary" onClick={() => void run(() => decisionsApi.update(decision.id, edit()))} disabled={busy || !codeValid}>Сохранить без проверки</button>}
        <button type="button" className="dc-delete" onClick={() => { if (window.confirm('Удалить запись из базы решений?')) void run(() => decisionsApi.remove(decision.id), true) }} disabled={busy}>Удалить</button>
      </footer>}
    </aside>
    {photoIndex !== null && decision && decision.photos.length > 0 && <PhotoLightbox photos={decision.photos} startIndex={photoIndex} onClose={() => setPhotoIndex(null)} />}
  </div>
}
