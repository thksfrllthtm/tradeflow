import { DragEvent, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { languages, maxFileMb, translatorApi } from './translatorApi'
import '@/shared/styles/drawer.css'

const pairKey = 'tradeflow.translator.langs'

// Последняя выбранная пара языков (по умолчанию китайский → русский).
function savedPair() {
  try {
    const pair = JSON.parse(localStorage.getItem(pairKey) ?? '') as { source?: string; target?: string }
    const known = (code?: string) => languages.some((item) => item.code === code)
    if (known(pair.source) && known(pair.target) && pair.source !== pair.target) return { source: pair.source!, target: pair.target! }
  } catch { /* нет сохранённой пары или хранилище недоступно */ }
  return { source: 'zh', target: 'ru' }
}

function checkFile(file: File) {
  const extension = /\.(\w+)$/.exec(file.name)?.[1].toLowerCase()
  if (extension === 'doc') return 'Файлы .doc не поддерживаются: откройте документ в Word и сохраните его как .docx'
  if (extension === 'xls') return 'Файлы .xls не поддерживаются: откройте книгу в Excel и сохраните её как .xlsx'
  if (extension !== 'docx' && extension !== 'xlsx') return 'Выберите документ Word (.docx) или книгу Excel (.xlsx)'
  const limit = maxFileMb[extension]
  if (file.size > limit << 20) return `Файл больше ${limit} МБ. Уменьшите его (например, сожмите картинки) или разделите на части`
  return ''
}

// Загрузка .docx или .xlsx: перетащить файл или выбрать, указать языки. После загрузки открывается документ.
export function UploadDrawer({ onClose, onUploaded }: { onClose: () => void; onUploaded: (id: number) => void }) {
  const [file, setFile] = useState<File | null>(null)
  const [pair, setPair] = useState(savedPair)
  const [dragging, setDragging] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  function pick(next: File | undefined) {
    if (!next) return
    const problem = checkFile(next)
    setError(problem)
    setFile(problem ? null : next)
  }

  function onDrop(event: DragEvent<HTMLElement>) {
    event.preventDefault()
    setDragging(false)
    if (!busy) pick(event.dataTransfer.files[0])
  }

  async function upload() {
    if (!file) return
    setBusy(true); setError('')
    try {
      try { localStorage.setItem(pairKey, JSON.stringify(pair)) } catch { /* пара просто не запомнится */ }
      onUploaded(await translatorApi.upload(file, pair.source, pair.target))
    } catch (reason) {
      setError(errorMessage(reason))
      setBusy(false)
    }
  }

  const select = (side: 'source' | 'target', label: string) => <label className="drawer-field">
    <span>{label}</span>
    <select value={pair[side]} disabled={busy} onChange={(event) => {
      const code = event.target.value
      const other = side === 'source' ? 'target' : 'source'
      // Тот же язык с другой стороны — меняем языки местами.
      setPair((current) => ({ ...current, [side]: code, ...(current[other] === code ? { [other]: current[side] } : {}) }))
    }}>
      {languages.map((item) => <option key={item.code} value={item.code}>{item.label}</option>)}
    </select>
  </label>

  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !busy) onClose() }}>
    <aside className="drawer" role="dialog" aria-modal="true" aria-labelledby="tr-upload-title">
      <header className="drawer-head">
        <div><h2 id="tr-upload-title">Загрузить документ</h2><p>Word (.docx) до 20 МБ или Excel (.xlsx) до 100 МБ, до 3 000 абзацев или строк с текстом</p></div>
        <button type="button" className="drawer-close" onClick={onClose} disabled={busy} aria-label="Закрыть">×</button>
      </header>
      <div className="drawer-body">
        <section className="drawer-section">
          <label className={dragging ? 'tr-drop dragging' : 'tr-drop'}
            onDragOver={(event) => { if (event.dataTransfer.types.includes('Files')) { event.preventDefault(); setDragging(true) } }}
            onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false) }} onDrop={onDrop}>
            <input type="file" accept=".docx,.xlsx,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" disabled={busy} onChange={(event) => { pick(event.target.files?.[0]); event.target.value = '' }} />
            {file ? <><b>{file.name}</b><small>{(file.size / 1024 / 1024).toFixed(1)} МБ · выбрать другой</small></>
              : <><b>Перетащите .docx или .xlsx сюда</b><small>или нажмите, чтобы выбрать файл</small></>}
          </label>
        </section>
        <section className="drawer-section">
          <h3>Языки</h3>
          <div className="drawer-row">{select('source', 'Язык оригинала')}{select('target', 'Перевести на')}</div>
        </section>
        {error && <p className="dir-transfer-error" role="alert">{error}</p>}
      </div>
      <footer className="drawer-foot">
        <button type="button" onClick={() => void upload()} disabled={!file || busy}>{busy ? 'Загружаем…' : 'Загрузить'}</button>
        <button type="button" className="secondary" onClick={onClose} disabled={busy}>Отмена</button>
      </footer>
    </aside>
  </div>
}
