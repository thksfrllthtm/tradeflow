package translator

// Правка перевода сотрудником: исправленный или подтверждённый сегмент блокируется от
// автоперевода, каждая версия текста пишется в историю.

import (
	"context"
	"encoding/json"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

// SegmentEdit — PATCH сегмента: новый текст перевода и/или статус (edited, confirmed).
type SegmentEdit struct {
	TargetText *string `json:"targetText"`
	Status     string  `json:"status"`
}

const maxTargetRunes = 20000

// EditSegment сохраняет правку. Новый текст без статуса — «исправлен»; «подтверждён» без текста
// подтверждает текущий перевод (машинный, если правки не было).
func (s *Service) EditSegment(ctx context.Context, userID, id int64, edit SegmentEdit) (SegmentView, error) {
	segment, err := s.repository.TranslatorSegmentByID(ctx, id)
	if err != nil {
		return SegmentView{}, err
	}
	document, err := s.repository.TranslatorDocumentByID(ctx, segment.DocumentID)
	if err != nil {
		return SegmentView{}, err
	}
	if edit.Status != "" && edit.Status != "edited" && edit.Status != "confirmed" {
		return SegmentView{}, &service.ValidationError{Field: "status", Message: "Неизвестный статус сегмента"}
	}
	current := segment.TargetText
	if current == "" {
		current = segment.MTText
	}
	target := current
	if edit.TargetText != nil {
		target = normalizeTagged(*edit.TargetText)
	}
	switch {
	case target == "":
		return SegmentView{}, &service.ValidationError{Field: "targetText", Message: "Перевод пустой: введите текст перевода"}
	case len([]rune(target)) > maxTargetRunes:
		return SegmentView{}, &service.ValidationError{Field: "targetText", Message: "Перевод сегмента слишком длинный"}
	}
	textChanged := target != current
	status := edit.Status
	if status == "" {
		if !textChanged {
			return segmentView(segment) // нечего сохранять
		}
		status = "edited"
	}
	segment.TargetText, segment.Status = target, status
	segment.QA, _ = json.Marshal(translationProblems(segment.SourceText, target, document.SourceLang, document.TargetLang))
	err = s.repository.SaveTranslatorSegmentEdit(ctx, repository.TranslatorSegmentEdit{
		Segment: segment, UserID: userID, TextChanged: textChanged,
	})
	if err != nil {
		return SegmentView{}, err
	}
	saved, err := s.repository.TranslatorSegmentByID(ctx, id)
	if err != nil {
		return SegmentView{}, err
	}
	return segmentView(saved)
}

// RetranslateSegment — «Перевести заново»: новый машинный перевод мимо кэша, правка
// снимается (на странице перед этим — подтверждение, если сегмент правили).
func (s *Service) RetranslateSegment(ctx context.Context, id int64) (SegmentView, error) {
	if !s.model.Configured() {
		return SegmentView{}, ai.ErrAINotConfigured
	}
	segment, err := s.repository.TranslatorSegmentByID(ctx, id)
	if err != nil {
		return SegmentView{}, err
	}
	document, err := s.repository.TranslatorDocumentByID(ctx, segment.DocumentID)
	if err != nil {
		return SegmentView{}, err
	}
	previous, err := s.repository.PreviousTranslatorSource(ctx, segment)
	if err != nil {
		return SegmentView{}, err
	}
	text, problems, err := machineTranslation(ctx, s.model, document.SourceLang, document.TargetLang, segment.SourceText, previous)
	if err != nil {
		return SegmentView{}, err
	}
	segment.MTText = text
	segment.QA, _ = json.Marshal(problems)
	if len(problems) == 0 {
		if err := s.repository.ReplaceTranslatorCache(ctx, hashText(s.model.Name(), document.SourceLang, document.TargetLang, segment.SourceText), text); err != nil {
			return SegmentView{}, err
		}
	}
	if err := s.repository.ResetTranslatorSegment(ctx, segment); err != nil {
		return SegmentView{}, err
	}
	saved, err := s.repository.TranslatorSegmentByID(ctx, id)
	if err != nil {
		return SegmentView{}, err
	}
	return segmentView(saved)
}

func (s *Service) SegmentHistory(ctx context.Context, id int64) ([]repository.TranslatorSegmentVersion, error) {
	if _, err := s.repository.TranslatorSegmentByID(ctx, id); err != nil {
		return nil, err
	}
	return s.repository.TranslatorSegmentHistory(ctx, id)
}
