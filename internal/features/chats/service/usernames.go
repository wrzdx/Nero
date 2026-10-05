package chats_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/wrzdx/Nero/internal/core/domain"
	"strings"

	"github.com/google/uuid"
)

// NormalizeUsernames keeps input order and duplicates for positional add results.
func NormalizeUsernames(names []string) ([]string, error) {
	if len(names) > 100 {
		return nil, domain.DetailedError{Err: ErrInvalidInput, Details: map[string]string{"usernames": "at most 100 usernames per request"}}
	}
	result := make([]string, 0, len(names))
	for i, name := range names {
		name = strings.TrimPrefix(strings.TrimSpace(name), "@")
		if !domain.UsernamePattern.MatchString(name) {
			return nil, domain.DetailedError{Err: ErrInvalidInput, Details: map[string]string{fmt.Sprintf("usernames[%d]", i): "must contain 5–32 ASCII letters, digits or underscores"}}
		}
		result = append(result, strings.ToLower(name))
	}
	return result, nil
}

type usernameTarget struct {
	name    string
	id      uuid.UUID
	deleted bool
}

func (s *ChatsService) resolveUsernames(ctx context.Context, names []string) ([]usernameTarget, error) {
	names, err := NormalizeUsernames(names)
	if err != nil {
		return nil, err
	}
	result := make([]usernameTarget, 0, len(names))
	cache := make(map[string]usernameTarget, len(names))
	for _, name := range names {
		target, cached := cache[name]
		if !cached {
			user, err := s.usersRepo.GetUserByUsername(ctx, name)
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				return nil, fmt.Errorf("resolve username: %w", err)
			}
			target = usernameTarget{name: name}
			if err == nil {
				target.id = user.ID
				target.deleted = user.DeletedAt != nil
			}
			cache[name] = target
		}
		result = append(result, target)
	}
	return result, nil
}

func (s *ChatsService) CreateDirectByUsername(ctx context.Context, requesterID uuid.UUID, username string) (domain.DirectChat, bool, error) {
	targets, err := s.resolveUsernames(ctx, []string{username})
	if err != nil {
		return domain.DirectChat{}, false, err
	}
	if targets[0].id == uuid.Nil || targets[0].deleted {
		return domain.DirectChat{}, false, domain.ErrNotFound
	}
	return s.CreateDirect(ctx, requesterID, targets[0].id)
}

type CreateGroupByUsernamesCommand struct {
	Title                string
	ParticipantUsernames []string
}

func (s *ChatsService) CreateGroupByUsernames(ctx context.Context, creatorID uuid.UUID, command CreateGroupByUsernamesCommand) (domain.GroupChat, error) {
	targets, err := s.resolveUsernames(ctx, command.ParticipantUsernames)
	if err != nil {
		return domain.GroupChat{}, err
	}
	ids := make([]uuid.UUID, 0, len(targets))
	seen := map[uuid.UUID]bool{creatorID: true}
	missing := map[string]string{}
	for _, target := range targets {
		if target.id == uuid.Nil || target.deleted {
			missing[target.name] = "user unavailable"
			continue
		}
		if !seen[target.id] {
			ids = append(ids, target.id)
			seen[target.id] = true
		}
	}
	if len(missing) > 0 {
		return domain.GroupChat{}, domain.DetailedError{Err: domain.ErrNotFound, Details: missing}
	}
	return s.CreateGroup(ctx, creatorID, CreateGroupCommand{Title: command.Title, ParticipantIDs: ids})
}

type AddGroupParticipantsByUsernamesCommand struct {
	GroupID, RequesterID uuid.UUID
	ParticipantUsernames []string
}

type UsernameParticipantResult struct {
	Username string
	Status   AddGroupParticipantStatus
}

func (s *ChatsService) AddGroupParticipantsByUsernames(ctx context.Context, command AddGroupParticipantsByUsernamesCommand) ([]UsernameParticipantResult, error) {
	targets, err := s.resolveUsernames(ctx, command.ParticipantUsernames)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(targets))
	positions := make([]int, 0, len(targets))
	result := make([]UsernameParticipantResult, len(targets))
	for i, target := range targets {
		result[i] = UsernameParticipantResult{Username: target.name, Status: Unavailable}
		if target.id != uuid.Nil && !target.deleted {
			ids = append(ids, target.id)
			positions = append(positions, i)
		}
	}
	// Call even for an empty resolved list: authorization still applies.
	added, err := s.AddGroupParticipants(ctx, AddGroupParticipantsCommand{GroupID: command.GroupID, RequesterID: command.RequesterID, ParticipantIDs: ids})
	if err != nil {
		return nil, err
	}
	if len(added) != len(positions) {
		return nil, errors.New("invalid participant result length")
	}
	for i, item := range added {
		result[positions[i]].Status = item.Status
	}
	return result, nil
}

type RemoveGroupParticipantByUsernameCommand struct {
	GroupID, RequesterID uuid.UUID
	TargetUsername       string
}

func (s *ChatsService) RemoveGroupParticipantByUsername(ctx context.Context, command RemoveGroupParticipantByUsernameCommand) error {
	targets, err := s.resolveUsernames(ctx, []string{command.TargetUsername})
	if err != nil {
		return err
	}
	if targets[0].id == uuid.Nil {
		return domain.ErrNotFound
	}
	return s.RemoveGroupParticipant(ctx, RemoveGroupParticipantCommand{GroupID: command.GroupID, RequesterID: command.RequesterID, TargetID: targets[0].id})
}
