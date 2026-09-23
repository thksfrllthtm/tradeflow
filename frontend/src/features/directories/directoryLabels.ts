// Подписи и разбор полей, общие для справочников ТН ВЭД и товаров.
type VerificationStatus = 'needs_review' | 'operator_verified'

export function verificationStatusLabel(status: VerificationStatus) { return status === 'operator_verified' ? 'Проверено оператором' : 'Требует проверки' }

// Синонимы в форме вводятся через перевод строки, запятую или точку с запятой.
export function splitLines(value: string) { return value.split(/[\n,;]/).map((part) => part.trim()).filter(Boolean) }
