import { useEffect, useState } from 'react'
import { splitLines } from '../common/directoryLabels'

type Synonyms = { synonymsRu: string[]; synonymsZh: string[]; synonymsEn: string[] }

// Текст поля хранится как ввели (с пробелами, запятыми и переводами строк), а наружу
// уходит уже разобранный список. Иначе разделитель пропадал бы сразу при вводе.
function SynonymTextarea({ label, lines, onChange }: { label: string; lines: string[]; onChange: (lines: string[]) => void }) {
  const [text, setText] = useState(lines.join('\n'))
  useEffect(() => {
    // Значение сменили снаружи (другая запись, сброс формы) — показываем его.
    if (splitLines(text).join('\n') !== lines.join('\n')) setText(lines.join('\n'))
  }, [lines])
  return <label className="drawer-field pe-row top"><span>{label}{lines.length > 0 && <em>{lines.length}</em>}</span><textarea rows={2} value={text} onChange={(event) => { setText(event.target.value); onChange(splitLines(event.target.value)) }} placeholder="Нет" /></label>
}

// Синонимы на трёх языках — карточка с тремя строками; в заголовке — сколько их всего.
export function SynonymFields({ value, hint, onChange }: { value: Synonyms; hint: string; onChange: (field: keyof Synonyms, lines: string[]) => void }) {
  const count = value.synonymsRu.length + value.synonymsZh.length + value.synonymsEn.length
  return <section className="pe-card">
    <h3>Синонимы{count > 0 && <span className="dir-count">{count}</span>}</h3>
    <SynonymTextarea label="Русский" lines={value.synonymsRu} onChange={(lines) => onChange('synonymsRu', lines)} />
    <SynonymTextarea label="中文" lines={value.synonymsZh} onChange={(lines) => onChange('synonymsZh', lines)} />
    <SynonymTextarea label="English" lines={value.synonymsEn} onChange={(lines) => onChange('synonymsEn', lines)} />
    <p className="pe-foot">{hint}</p>
  </section>
}
