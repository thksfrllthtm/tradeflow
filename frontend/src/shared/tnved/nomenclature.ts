// Код номенклатуры ТН ВЭД ЕАЭС с пошлиной ЕТТ, НДС, ставкой перечня РК и ссылкой на страницу
// кода — общий для справочников, смет, базы решений и подбора кода ИИ.

// combined_plus — «процент плюс сумма за единицу», такого типа в справочнике нет.
export type NomenclatureDuty = { type: 'unknown' | 'percentage' | 'specific' | 'combined_minimum' | 'combined_plus'; percent: string; amount: string; currency: string; unit: string }
// detailsUrl — страница кода в источнике («Подробнее» в выгрузке TWS), пусто, если ссылки нет.
// Ставка по перечню РК (ниже ЕТТ) и её действие на сегодня; other — форма, которой нет в справочнике.
export type KzRate = { rateText: string; duty: Omit<NomenclatureDuty, 'type'> & { type: 'percentage' | 'specific' | 'combined_minimum' | 'other' }; validFrom: string; validTo: string; active: boolean; status: string; notesText: string }
export type NomenclatureCode = { code: string; name: string; tariffText: string; duty: NomenclatureDuty; vatPercent: string; detailsUrl: string; inDirectory: boolean; kz: KzRate | null }
export type NomenclatureImport = { source: string; fileName: string; actualDate: string; codeCount: number; addedCount: number; removedCount: number; changedCount: number; importedAt: string; importedByName: string }
