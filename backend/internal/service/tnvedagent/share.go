package tnvedagent

import (
	"context"
	"errors"
	"slices"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

// Доступ к вопросу чата на главной для коллеги: открыть может автор или администратор, получатель только
// читает. Все действия, которые меняют вопрос (уточнение, перезапуск, документы, удаление), по-прежнему идут
// через ownedSession, то есть только у автора; расход ИИ — на авторе.

// canShare — открывать и закрывать доступ к вопросу может автор или администратор, и только к чату на главной:
// подбор по строке сметы привязан к смете, которую видит только её владелец.
func canShare(session repository.TNVEDAgentSession, user repository.User) bool {
	return session.Mode == modeReference && (session.UserID == user.ID || user.IsAdmin())
}

// canUnshare — закрыть доступ может тот, кто открывает, а получатель — себе (убрать вопрос из своей истории).
func canUnshare(session repository.TNVEDAgentSession, user repository.User, targetID int64) bool {
	return canShare(session, user) || (session.Mode == modeReference && targetID == user.ID)
}

// ShareUsers — кому можно открыть вопрос: работающие сотрудники, кроме себя.
func (s *TNVEDAgentService) ShareUsers(ctx context.Context, user repository.User) ([]repository.AgentShareUser, error) {
	return s.repository.ShareUsers(ctx, user.ID)
}

func (s *TNVEDAgentService) shareableSession(ctx context.Context, user repository.User, id int64) (repository.TNVEDAgentSession, error) {
	session, err := s.repository.AgentSessionByID(ctx, id)
	if err != nil {
		return repository.TNVEDAgentSession{}, err
	}
	if !canShare(session, user) {
		return repository.TNVEDAgentSession{}, ErrAgentForbidden
	}
	return session, nil
}

// Shares — кому открыт вопрос (видит автор или администратор).
func (s *TNVEDAgentService) Shares(ctx context.Context, user repository.User, id int64) ([]repository.AgentShareUser, error) {
	if _, err := s.shareableSession(ctx, user, id); err != nil {
		return nil, err
	}
	return s.repository.AgentSessionShares(ctx, id)
}

// Share открывает вопрос сотрудникам: автор и несуществующие или отключённые сотрудники отбрасываются.
func (s *TNVEDAgentService) Share(ctx context.Context, user repository.User, id int64, userIDs []int64) ([]repository.AgentShareUser, error) {
	session, err := s.shareableSession(ctx, user, id)
	if err != nil {
		return nil, err
	}
	wanted := []int64{}
	for _, userID := range userIDs {
		if userID > 0 && userID != session.UserID && !slices.Contains(wanted, userID) {
			wanted = append(wanted, userID)
		}
	}
	valid, err := s.repository.ExistingShareUsers(ctx, wanted)
	if err != nil {
		return nil, err
	}
	if len(valid) == 0 {
		return nil, &service.ValidationError{Field: "userIds", Message: "Выберите сотрудника, которому открыть вопрос"}
	}
	if err := s.repository.ShareAgentSession(ctx, id, user.ID, valid); err != nil {
		return nil, err
	}
	return s.repository.AgentSessionShares(ctx, id)
}

// Unshare закрывает вопрос сотруднику (автор или администратор — любому, получатель — себе).
func (s *TNVEDAgentService) Unshare(ctx context.Context, user repository.User, id, targetID int64) error {
	session, err := s.repository.AgentSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if !canUnshare(session, user, targetID) {
		return ErrAgentForbidden
	}
	return s.repository.UnshareAgentSession(ctx, id, targetID)
}

// readableSession — вопрос для просмотра: свой (как ownedSession) или открытый сотруднику коллегой.
// sharedBy — имя автора, если вопрос чужой и открыт только для чтения ("" — свой).
func (s *TNVEDAgentService) readableSession(ctx context.Context, user repository.User, id int64) (repository.TNVEDAgentSession, string, error) {
	session, err := s.ownedSession(ctx, user, id)
	if !errors.Is(err, ErrAgentForbidden) {
		return session, "", err
	}
	session, err = s.repository.AgentSessionByID(ctx, id)
	if err != nil {
		return repository.TNVEDAgentSession{}, "", err
	}
	if session.Mode != modeReference {
		return repository.TNVEDAgentSession{}, "", ErrAgentForbidden
	}
	shared, err := s.repository.AgentSessionSharedWith(ctx, id, user.ID)
	if err != nil {
		return repository.TNVEDAgentSession{}, "", err
	}
	if !shared {
		return repository.TNVEDAgentSession{}, "", ErrAgentForbidden
	}
	author, err := s.repository.UserByID(ctx, session.UserID)
	name := author.Name
	if err != nil || name == "" {
		name = "коллеги"
	}
	return session, name, nil
}
