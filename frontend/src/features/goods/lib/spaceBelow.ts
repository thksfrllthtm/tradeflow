// Сколько места под элементом до низа страницы сметы (строки под ним, подпись, отступ) — для подгонки
// листа или таблицы к окну. Пустоту над подписью (её margin-top: auto, когда смета короче окна) не
// считаем: иначе лист, однажды ужавшись, держал бы эту пустоту сам и больше не рос.
export function spaceBelow(element: Element) {
  const page = element.closest('.goods-page')
  if (!page) return 0
  const disclaimer = page.querySelector('.goods-disclaimer')
  const slack = disclaimer ? parseFloat(getComputedStyle(disclaimer).marginTop) || 0 : 0
  return Math.max(0, page.getBoundingClientRect().bottom - element.getBoundingClientRect().bottom - slack)
}
