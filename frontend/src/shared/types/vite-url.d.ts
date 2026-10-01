// Импорт файла как адреса (Vite: `?url`), например воркер pdf.js.
declare module '*?url' {
  const url: string
  export default url
}

// Таблица стилей, загружаемая по требованию (import('…/x.css')), например стили Jspreadsheet.
declare module '*.css'
