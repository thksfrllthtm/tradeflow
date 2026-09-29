// Импорт файла как адреса (Vite: `?url`), например воркер pdf.js.
declare module '*?url' {
  const url: string
  export default url
}
