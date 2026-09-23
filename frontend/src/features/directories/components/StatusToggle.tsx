type Status = 'needs_review' | 'operator_verified'

// Статус проверки записи в форме: два варианта переключателем вместо выпадающего списка.
export function StatusToggle({ value, onChange }: { value: Status; onChange: (value: Status) => void }) {
  return <div className="drawer-status" role="radiogroup" aria-label="Статус проверки">
    <button type="button" role="radio" aria-checked={value === 'needs_review'} className={value === 'needs_review' ? 'active review' : undefined} onClick={() => onChange('needs_review')}>Требует проверки</button>
    <button type="button" role="radio" aria-checked={value === 'operator_verified'} className={value === 'operator_verified' ? 'active verified' : undefined} onClick={() => onChange('operator_verified')}>Проверено</button>
  </div>
}
