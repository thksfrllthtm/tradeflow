package httpapi

// Раздача собранного фронтенда тем же сервером, что и API (STATIC_DIR). Так сайт и API
// живут на одном адресе и cookie сессии работает без CORS. Локально (docker-compose)
// STATIC_DIR не задан и сайт отдаёт Vite.

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) serveFrontend(router *gin.Engine) {
	dir := s.config.StaticDir
	if dir == "" {
		return
	}
	index := filepath.Join(dir, "index.html")
	router.NoRoute(func(c *gin.Context) {
		urlPath := c.Request.URL.Path
		if urlPath == "/api" || strings.HasPrefix(urlPath, "/api/") || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Не найдено", "code": "not_found"}})
			return
		}
		// Clean от корня не даёт выйти за пределы папки сборки через "..".
		clean := path.Clean("/" + urlPath)
		file := filepath.Join(dir, filepath.FromSlash(clean))
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			if strings.HasPrefix(clean, "/assets/") {
				// Имена файлов сборки содержат хеш — их можно кэшировать надолго.
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			}
			c.File(file)
			return
		}
		if strings.HasPrefix(clean, "/assets/") {
			c.Status(http.StatusNotFound)
			return
		}
		// Остальные адреса (/estimates/goods/12, /directory/tnved…) — страницы приложения.
		c.Header("Cache-Control", "no-cache")
		c.File(index)
	})
}
