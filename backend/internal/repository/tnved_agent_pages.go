package repository

// Сохранённые страницы для агента ТН ВЭД: страницы кодов TWS.BY и веб-страницы (справки tnved.info, акты).

import (
	"context"
	"time"
)

type TNVEDDetailsPage struct {
	Code      string    `gorm:"primaryKey;size:10"`
	URL       string    `gorm:"size:500;not null"`
	Text      string    `gorm:"not null"`
	FetchedAt time.Time `gorm:"not null"`
}

func (TNVEDDetailsPage) TableName() string { return "tnved_details_pages" }

// WebPageCache — сохранённая веб-страница для агента (справка tnved.info, акты): текст и ссылки.
type WebPageCache struct {
	URL       string    `gorm:"primaryKey;size:1000"`
	Title     string    `gorm:"not null"`
	Text      string    `gorm:"not null"`
	Links     JSON      `gorm:"type:jsonb;not null"`
	FetchedAt time.Time `gorm:"not null"`
}

func (WebPageCache) TableName() string { return "web_pages" }

// DetailsPage — сохранённая страница кода; ErrNotFound, если её ещё не скачивали (Find, а не Take:
// отсутствие страницы — обычный случай, и GORM не пишет его в лог как ошибку).
func (r *Repository) DetailsPage(ctx context.Context, code string) (TNVEDDetailsPage, error) {
	var pages []TNVEDDetailsPage
	if err := r.db.WithContext(ctx).Where("code = ?", code).Limit(1).Find(&pages).Error; err != nil {
		return TNVEDDetailsPage{}, err
	}
	if len(pages) == 0 {
		return TNVEDDetailsPage{}, ErrNotFound
	}
	return pages[0], nil
}

func (r *Repository) DetailsPages(ctx context.Context, codes []string) ([]TNVEDDetailsPage, error) {
	var pages []TNVEDDetailsPage
	if len(codes) == 0 {
		return pages, nil
	}
	return pages, r.db.WithContext(ctx).Where("code IN ?", codes).Find(&pages).Error
}

func (r *Repository) SaveDetailsPage(ctx context.Context, page TNVEDDetailsPage) error {
	return r.db.WithContext(ctx).Save(&page).Error
}

// WebPage — сохранённая веб-страница; ErrNotFound, если её ещё не скачивали.
func (r *Repository) WebPage(ctx context.Context, url string) (WebPageCache, error) {
	var pages []WebPageCache
	if err := r.db.WithContext(ctx).Where("url = ?", url).Limit(1).Find(&pages).Error; err != nil {
		return WebPageCache{}, err
	}
	if len(pages) == 0 {
		return WebPageCache{}, ErrNotFound
	}
	return pages[0], nil
}

func (r *Repository) SaveWebPage(ctx context.Context, page WebPageCache) error {
	return r.db.WithContext(ctx).Save(&page).Error
}
