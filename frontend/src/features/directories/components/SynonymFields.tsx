import { useEffect, useState } from 'react'
import { splitLines } from '../directoryLabels'

type Synonyms = { synonymsRu: string[]; synonymsZh: string[]; synonymsEn: string[] }

// Текст поля хранится как ввели (с пробелами, запятыми и переводами строк), а наружу
// уходит уже разобранный список. Иначе разделитель пропадал бы сразу при вводе.
function SynonymTextarea({ label, lines, onChange }: { label: string; lines: string[]; onChange: (lines: string[]) => void }) {
  const [text, setText] = useState(lines.join('\n'))
  useEffect(() => {
    // Значение сменили снаружи (другая запись, сброс формы) — показываем его.
    if (splitLines(text).join('\n') !== lines.join('\n')) setText(lines.join('\n'))
  }, [lines])
  return <label className="drawer-field"><span>{label}</span><textarea rows={2} value={text} onChange={(event) => { setText(event.target.value); onChange(splitLines(event.target.value)) }} placeholder="Каждый вариант с новой строки или через запятую" /></label>
}

// Синонимы на трёх языках. Раздел свёрнут, пока синонимов нет; в заголовке — их количество.
export function SynonymFields({ value, hint, onChange }: { value: Synonyms; hint: string; onChange: (field: keyof Synonyms, lines: string[]) => void }) {
  const count = value.synonymsRu.length + value.synonymsZh.length + value.synonymsEn.length
  // Раскрыт ли раздел при открытии формы; дальше открывает и закрывает только пользователь.
  const [initiallyOpen] = useState(count > 0)
  return <details className="drawer-section drawer-more" open={initiallyOpen || undefined}>
    <summary>Синонимы <small>{count > 0 ? count : 'нет'}</small></summary>
    <div>
      <p className="drawer-hint">{hint}</p>
      <SynonymTextarea label="Русский" lines={value.synonymsRu} onChange={(lines) => onChange('synonymsRu', lines)} />
      <SynonymTextarea label="中文" lines={value.synonymsZh} onChange={(lines) => onChange('synonymsZh', lines)} />
      <SynonymTextarea label="English" lines={value.synonymsEn} onChange={(lines) => onChange('synonymsEn', lines)} />
    </div>
  </details>
}
