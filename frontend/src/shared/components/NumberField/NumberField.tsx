import { InputHTMLAttributes, useState } from 'react'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'

type Props = Omit<InputHTMLAttributes<HTMLInputElement>, 'value' | 'onChange'> & {
  // Значение — как хранится: без пробелов, дробная часть через точку («66974.8»).
  value: string
  onChange: (value: string) => void
}

// Числовое поле: без фокуса — через общий форматтер («66 974,8»), при вводе — без пробелов-разрядов.
// Принимает и запятую, и точку; наружу отдаёт значение с точкой.
export function NumberField({ value, onChange, onFocus, onBlur, ...rest }: Props) {
  const [focused, setFocused] = useState(false)
  return <input
    {...rest}
    inputMode="decimal"
    value={focused ? value.replace('.', ',') : formatRuNumber(value)}
    onFocus={(event) => { setFocused(true); onFocus?.(event) }}
    onBlur={(event) => { setFocused(false); onBlur?.(event) }}
    onChange={(event) => onChange(normalizeNumberInput(event.target.value))}
  />
}

// Ввод «66 974,8» или «66974.8» → «66974.8».
export const normalizeNumberInput = (text: string) => text.replace(/[\s\u00a0]/g, '').replace(',', '.')
