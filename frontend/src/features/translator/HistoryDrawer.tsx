import { useEffect, useState } from 'react'
import { Drawer } from '@/shared/components/Drawer/Drawer'
import { errorMessage } from '@/shared/lib/errorMessage'
import { TaggedText } from './TaggedText'
import { Segment, SegmentVersion, translatorApi } from './translatorApi'

const formatTime = (value: string) => new Intl.DateTimeFormat('ru-RU', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value))

// История версий перевода сегмента: кто и когда менял; любую версию можно вернуть (это новая правка).
export function HistoryDrawer({ segment, onClose, onRestore }: { segment: Segment; onClose: () => void; onRestore: (text: string) => Promise<void> }) {
  const [versions, setVersions] = useState<SegmentVersion[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    translatorApi.history(segment.id).then(setVersions).catch((reason) => setError(errorMessage(reason)))
  }, [segment.id])

  async function restore(text: string) {
    setBusy(true)
    try {
      await onRestore(text)
      onClose()
    } catch (reason) {
      setError(errorMessage(reason))
      setBusy(false)
    }
  }

  const current = segment.targetText || segment.mtText
  const restoreButton = (text: string) => text === current
    ? <small className="tr-history-current">Текущая</small>
    : <button type="button" className="secondary sm" onClick={() => void restore(text)} disabled={busy}>Вернуть</button>

  return <Drawer title="История перевода" subtitle={<TaggedText text={segment.sourceText} tags={segment.tags} />} onClose={() => { if (!busy) onClose() }}>
    {error && <p className="dir-transfer-error" role="alert">{error}</p>}
    {versions === null && !error && <p className="drawer-hint">Загрузка…</p>}
    <ol className="tr-history">
      {versions?.map((version) => <li key={version.id}>
        <div><TaggedText text={version.targetText} tags={segment.tags} /></div>
        <footer><small>{version.changedByName || '—'} · {formatTime(version.changedAt)}</small>{restoreButton(version.targetText)}</footer>
      </li>)}
      {segment.mtText && <li>
        <div><TaggedText text={segment.mtText} tags={segment.tags} /></div>
        <footer><small>Машинный перевод</small>{restoreButton(segment.mtText)}</footer>
      </li>}
    </ol>
    {versions?.length === 0 && !segment.mtText && <p className="drawer-hint">Правок пока не было.</p>}
  </Drawer>
}
