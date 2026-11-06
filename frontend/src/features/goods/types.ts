// Какие колонки Excel сопоставлены полям строки (значение — заголовок колонки).
export type GoodsMapping = {
  name: string
  // «Данные» — колонки со свойствами товара (материал, модель, назначение…): уходят ИИ при подборе кода
  // с подписью из заголовка колонки («Материал: сталь»).
  data1: string
  data2: string
  data3: string
  data4: string
  data5: string
  code: string
  pairs: string
  places: string
  pairsPerPlace: string
  grossKg: string
  netKg: string
  // Цена за единицу из файла (USD); у строк без цены стоимость считается как обычно.
  pricePerPair: string
}

// Что сейчас ждёт ответа сервера: кнопки показывают «Подождите…» только у своего действия.
export type CalculatorActivity = '' | 'calculate' | 'save' | 'export'
