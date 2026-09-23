import { ArchiveIcon, CopyIcon, EditIcon, RestoreIcon } from './icons'

// Действия строки справочника. Клик по самой строке тоже открывает редактирование,
// поэтому здесь клики не всплывают до строки.
export function RowActions({ name, archived, onEdit, onCopy, onArchive, onRestore }: { name: string; archived: boolean; onEdit: () => void; onCopy: () => void; onArchive: () => void; onRestore: () => void }) {
  return <div className="dir-actions" onClick={(event) => event.stopPropagation()}>
    <button type="button" className="dir-act" onClick={onEdit} title="Изменить" aria-label={`Изменить «${name}»`}><EditIcon /></button>
    <button type="button" className="dir-act" onClick={onCopy} title="Создать копию" aria-label={`Копировать «${name}»`}><CopyIcon /></button>
    {archived
      ? <button type="button" className="dir-act" onClick={onRestore} title="Вернуть из архива" aria-label={`Вернуть из архива «${name}»`}><RestoreIcon /></button>
      : <button type="button" className="dir-act danger" onClick={onArchive} title="В архив" aria-label={`В архив «${name}»`}><ArchiveIcon /></button>}
  </div>
}
