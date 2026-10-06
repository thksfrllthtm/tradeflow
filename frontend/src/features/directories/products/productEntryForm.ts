import { ProductDirectoryEntry, ProductDirectoryInput } from './types'

// Значения формы карточки товара: пустая карточка и перенос существующей в форму.
export const emptyInput = (): ProductDirectoryInput => ({ category: 'Товары', name: '', nameZh: '', unit: 'шт.', companyCoefficient: '', synonymsRu: [], synonymsZh: [], synonymsEn: [], code: '', verificationStatus: 'needs_review' })

export function toInput(item: ProductDirectoryEntry): ProductDirectoryInput { return { category: item.category, name: item.name, nameZh: item.nameZh, unit: item.unit, companyCoefficient: item.companyCoefficient, synonymsRu: item.synonymsRu, synonymsZh: item.synonymsZh, synonymsEn: item.synonymsEn, code: item.code, verificationStatus: item.verificationStatus } }
