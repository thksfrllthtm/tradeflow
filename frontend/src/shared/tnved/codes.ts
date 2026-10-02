// «6116102000» → «6116 10 200 0».
export function formatCode(code: string) {
  return /^\d{10}$/.test(code) ? `${code.slice(0, 4)} ${code.slice(4, 6)} ${code.slice(6, 9)} ${code.slice(9)}` : code
}

// Наименование в выгрузке TWS — последние уровни дерева через «🠺»:
// «перчатки, рукавицы и митенки: 🠺 прочие».
export function nameParts(name: string) {
  return name.split('🠺').map((part) => part.trim()).filter(Boolean)
}
