type Value = string | number | undefined | null

// Колонка для вставки в Excel столбцом: по числу на строку, пустая строка — пустая ячейка.
// Без пробелов-разрядов, дробная часть через запятую — так ждёт русский/казахстанский Excel сотрудников.
export const excelColumn = (values: Value[]) =>
  values.map((value) => String(value ?? '').replace(/\s/g, '').replace('.', ',')).join('\n')

// Та же колонка как HTML для Excel: число берётся из x:num (с точкой) при любых разделителях в настройках Excel,
// иначе «1,3272» при запятой-разделителе тысяч становится 13272. Без xmlns:x Excel атрибут игнорирует.
export const excelColumnHtml = (values: Value[]) => {
  const cells = values.map((value) => {
    const number = String(value ?? '').replace(/\s/g, '')
    return /^-?\d+(\.\d+)?$/.test(number) ? `<td x:num="${number}">${number.replace('.', ',')}</td>` : '<td></td>'
  })
  return `<html xmlns:x="urn:schemas-microsoft-com:office:excel"><body><table>${cells.map((cell) => `<tr>${cell}</tr>`).join('')}</table></body></html>`
}
