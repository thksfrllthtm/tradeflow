type Status = 'needs_review' | 'operator_verified'

// Статус проверки записи в форме: два варианта переключателем вместо выпадающего списка.
// canVerify = false — «Проверено» недоступно (ставит только администратор).
export function StatusToggle({ value, onChange, canVerify = true }: { value: Status; onChange: (value: Status) => void; canVerify?: boolean }) {
  return <div className="drawer-status" role="radiogroup" aria-label="Статус проверки">
    <button type="button" role="radio" aria-checked={value === 'needs_review'} className={value === 'needs_review' ? 'active review' : undefined} onClick={() => onChange('needs_review')}>Требует проверки</button>
    <button type="button" role="radio" aria-checked={value === 'operator_verified'} className={value === 'operator_verified' ? 'active verified' : undefined} onClick={() => onChange('operator_verified')} disabled={!canVerify} title={canVerify ? undefined : 'Отмечает только администратор'}>Проверено</button>
  </div>
}
