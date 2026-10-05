package web_handlers

import (
	"errors"
	"github.com/wrzdx/Nero/internal/core/domain"
	core_types "github.com/wrzdx/Nero/internal/core/types"
	chats_service "github.com/wrzdx/Nero/internal/features/chats/service"
	users_service "github.com/wrzdx/Nero/internal/features/users/service"
	web_views "github.com/wrzdx/Nero/internal/web/views"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

func (h *AppHandler) Profile(w http.ResponseWriter, r *http.Request) {
	h.profile(w, r, web_views.ProfileData{}, 200)
}
func (h *AppHandler) profile(w http.ResponseWriter, r *http.Request, data web_views.ProfileData, status int) {
	app, err := h.base(r, "profile")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	app.Title = "Профиль"
	if data.Person.ID == "" {
		data.Person = app.Me
	}
	if data.Saved {
		app.Me = data.Person
	}
	h.page(w, r, status, web_views.ProfilePage(app, data), web_views.ProfileRoot(app, data))
}
func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
func (h *AppHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	if err := parseAppForm(w, r); err != nil {
		h.failure(w, r, err)
		return
	}
	username, first := strings.TrimSpace(r.PostForm.Get("username")), strings.TrimSpace(r.PostForm.Get("first_name"))
	last, bio := optionalString(r.PostForm.Get("last_name")), optionalString(r.PostForm.Get("bio"))
	u, err := h.users.UpdateProfile(r.Context(), requester(r), users_service.UpdateProfileCommand{Username: &username, FirstName: &first, LastName: core_types.Nullable[string]{Set: true, Value: last}, Bio: core_types.Nullable[string]{Set: true, Value: bio}})
	data := web_views.ProfileData{Saved: err == nil, Errors: map[string]string{}}
	status := 200
	if err == nil {
		data.Person = person(u)
	} else {
		status, data.Message = publicError(err)
		data.Person = web_views.Person{ID: requester(r).String(), Username: username, FirstName: first, Name: first, Initials: web_views.Initials(first)}
		if last != nil {
			data.Person.LastName = *last
			data.Person.Name += " " + *last
		}
		if bio != nil {
			data.Person.Bio = *bio
		}
		if errors.Is(err, domain.ErrAlreadyExists) {
			data.Errors["username"] = "Это имя уже занято."
		}
		if detailed, ok := errors.AsType[domain.DetailedError](err); ok {
			for key := range detailed.Details {
				switch key {
				case "username":
					data.Errors[key] = "5–32 символа: латиница, цифры и _."
				case "first_name":
					data.Errors[key] = "От 1 до 64 символов."
				case "last_name":
					data.Errors[key] = "Не более 64 символов."
				case "bio":
					data.Errors[key] = "Не более 70 символов."
				}
			}
		}
	}
	h.profile(w, r, data, status)
}
func (h *AppHandler) User(w http.ResponseWriter, r *http.Request) {
	id, err := routeID(r, "user_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if id == requester(r) {
		redirectPage(w, r, "/app/profile")
		return
	}
	u, err := h.users.GetUser(r.Context(), id)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	app, err := h.base(r, "user")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	p := person(u)
	app.Title = p.Name
	h.page(w, r, 200, web_views.UserPage(app, p), web_views.UserRoot(app, p))
}
func (h *AppHandler) Password(w http.ResponseWriter, r *http.Request) { h.password(w, r, "", 200) }
func (h *AppHandler) password(w http.ResponseWriter, r *http.Request, message string, status int) {
	app, err := h.base(r, "password")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	app.Title = "Пароль"
	h.page(w, r, status, web_views.PasswordPage(app, message), web_views.PasswordRoot(app, message))
}
func (h *AppHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if err := parseAppForm(w, r); err != nil {
		h.failure(w, r, err)
		return
	}
	newPassword := r.PostForm.Get("new_password")
	if newPassword != r.PostForm.Get("confirm_password") {
		h.password(w, r, "Новые пароли не совпадают.", 422)
		return
	}
	if err := h.passwords.ChangePassword(r.Context(), requester(r), r.PostForm.Get("current_password"), newPassword); err != nil {
		status, message := publicError(err)
		h.password(w, r, message, status)
		return
	}
	h.sessions.invalidateUser(requester(r))
	h.sessions.clearCookies(w)
	redirectPage(w, r, "/login?changed=1")
}
func (h *AppHandler) DeleteAccountPage(w http.ResponseWriter, r *http.Request) {
	h.deleteAccount(w, r, "", 200)
}
func (h *AppHandler) deleteAccount(w http.ResponseWriter, r *http.Request, message string, status int) {
	app, err := h.base(r, "delete")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	app.Title = "Удалить аккаунт"
	h.page(w, r, status, web_views.DeleteAccountPage(app, message), web_views.DeleteAccountRoot(app, message))
}
func (h *AppHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := parseAppForm(w, r); err != nil {
		h.failure(w, r, err)
		return
	}
	u, err := h.users.GetUser(r.Context(), requester(r))
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if strings.TrimSpace(r.PostForm.Get("confirm_username")) != u.Profile.Username {
		h.deleteAccount(w, r, "Имя пользователя не совпадает.", 422)
		return
	}
	if err := h.users.DeleteAccount(r.Context(), u.ID); err != nil {
		h.failure(w, r, err)
		return
	}
	h.sessions.invalidateUser(requester(r))
	h.sessions.clearCookies(w)
	redirectPage(w, r, "/")
}

func (h *AppHandler) NewChat(w http.ResponseWriter, r *http.Request) {
	h.newChat(w, r, r.URL.Query().Get("type") == "group", "", "", "", 200)
}
func (h *AppHandler) newChat(w http.ResponseWriter, r *http.Request, group bool, message, title, usernames string, status int) {
	app, err := h.base(r, "new")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	app.Title = "Новый разговор"
	h.page(w, r, status, web_views.NewChatPage(app, group, message, title, usernames), web_views.NewChatRoot(app, group, message, title, usernames))
}
func parseUsernames(raw string) ([]string, error) {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' ' || r == '\t' })
	return chats_service.NormalizeUsernames(parts)
}

func participantFormValue(r *http.Request) string {
	if names, ok := r.PostForm["participant_username"]; ok {
		return strings.Join(names, ",")
	}
	// Accept forms opened before the participant picker was introduced.
	return r.PostForm.Get("participants")
}
func (h *AppHandler) CreateChat(w http.ResponseWriter, r *http.Request) {
	if err := parseAppForm(w, r); err != nil {
		h.failure(w, r, err)
		return
	}
	group := r.PostForm.Get("type") == "group"
	title := r.PostForm.Get("title")
	raw := strings.Join(r.PostForm["peer_username"], ",")
	if group {
		raw = participantFormValue(r)
	}
	names, err := parseUsernames(raw)
	if err != nil {
		h.newChat(w, r, group, "Проверь username: 5–32 символа, латиница, цифры и _. Не более 100 имён.", title, raw, 422)
		return
	}
	var id uuid.UUID
	if group {
		g, createErr := h.chats.CreateGroupByUsernames(r.Context(), requester(r), chats_service.CreateGroupByUsernamesCommand{Title: title, ParticipantUsernames: names})
		id, err = g.Chat.ID, createErr
	} else {
		if len(names) != 1 {
			h.newChat(w, r, false, "Укажи username собеседника.", title, raw, 422)
			return
		}
		direct, _, createErr := h.chats.CreateDirectByUsername(r.Context(), requester(r), names[0])
		id, err = direct.Chat.ID, createErr
	}
	if err != nil {
		status, message := publicError(err)
		if errors.Is(err, domain.ErrNotFound) {
			message = "Пользователь не найден. Проверь username участников."
		}
		h.newChat(w, r, group, message, title, raw, status)
		return
	}
	redirectPage(w, r, "/app/chats/"+id.String())
}

func (h *AppHandler) Group(w http.ResponseWriter, r *http.Request) { h.group(w, r, "", 200) }
func (h *AppHandler) group(w http.ResponseWriter, r *http.Request, message string, status int) {
	h.groupSelection(w, r, message, status, nil)
}
func (h *AppHandler) groupSelection(w http.ResponseWriter, r *http.Request, message string, status int, selected []string) {
	id, err := routeID(r, "chat_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	item, err := h.findChat(r, id)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if item.Chat.Type != domain.ChatTypeGroup {
		h.failure(w, r, domain.ErrNotFound)
		return
	}
	app, err := h.base(r, "group")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	app.Title = row(item).Title
	// Read all member pages so management controls use the actual current role.
	var before *chats_service.GroupParticipantCursor
	members := []chats_service.ParticipantInfo{}
	for {
		page, err := h.chats.ListGroupParticipants(r.Context(), requester(r), chats_service.ListGroupParticipantsQuery{ChatID: id, Before: before, Limit: 100})
		if err != nil {
			h.failure(w, r, err)
			return
		}
		members = append(members, page.Participants...)
		if page.NextCursor == nil {
			break
		}
		if before != nil && *page.NextCursor == *before {
			h.failure(w, r, errors.New("participant cursor did not advance"))
			return
		}
		before = page.NextCursor
	}
	role := "member"
	for _, member := range members {
		if member.ID == requester(r) {
			role = member.Role
		}
	}
	data := web_views.GroupData{ID: id.String(), Title: app.Title, Message: message, Manage: role == "owner" || role == "admin", CanLeave: role != "owner", PendingUsernames: selected}
	for _, member := range members {
		name := member.FirstName
		if member.LastName != nil {
			name += " " + *member.LastName
		}
		label := map[string]string{"owner": "Владелец", "admin": "Администратор", "member": "Участник"}[member.Role]
		canRemove := member.ID != requester(r) && member.Role != "owner" && (role == "owner" || (role == "admin" && member.Role == "member"))
		data.Members = append(data.Members, web_views.GroupMember{ID: member.ID.String(), Username: member.Username, Name: name, Initials: web_views.Initials(name), Role: label, CanRemove: canRemove})
	}
	h.page(w, r, status, web_views.GroupPage(app, data), web_views.GroupRoot(app, data))
}
func (h *AppHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := routeID(r, "chat_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if err := parseAppForm(w, r); err != nil {
		h.failure(w, r, err)
		return
	}
	_, err = h.chats.UpdateGroup(r.Context(), chats_service.UpdateGroupCommand{RequesterID: requester(r), GroupID: id, Title: r.PostForm.Get("title")})
	if err != nil {
		status, message := publicError(err)
		h.group(w, r, message, status)
		return
	}
	redirectPage(w, r, "/app/groups/"+id.String())
}
func (h *AppHandler) AddMembers(w http.ResponseWriter, r *http.Request) {
	id, err := routeID(r, "chat_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if err := parseAppForm(w, r); err != nil {
		h.failure(w, r, err)
		return
	}
	raw := participantFormValue(r)
	names, err := parseUsernames(raw)
	if err != nil || len(names) == 0 {
		h.groupSelection(w, r, "Укажи username участников: латиница, цифры и _, от 5 до 32 символов. Не более 100 имён.", 422, web_views.PickerNames(raw))
		return
	}
	result, err := h.chats.AddGroupParticipantsByUsernames(r.Context(), chats_service.AddGroupParticipantsByUsernamesCommand{GroupID: id, RequesterID: requester(r), ParticipantUsernames: names})
	if err != nil {
		status, message := publicError(err)
		h.groupSelection(w, r, message, status, names)
		return
	}
	var unavailable []string
	for _, added := range result {
		if added.Status == chats_service.Unavailable {
			unavailable = append(unavailable, "@"+added.Username)
		}
	}
	if len(unavailable) > 0 {
		h.groupSelection(w, r, "Пользователи недоступны: "+strings.Join(unavailable, ", ")+". Остальные участники добавлены или уже состоят в группе.", 200, unavailable)
		return
	}
	redirectPage(w, r, "/app/groups/"+id.String())
}
func (h *AppHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	id, err := routeID(r, "chat_id")
	if err != nil {
		h.failure(w, r, err)
		return
	}
	if err := parseAppForm(w, r); err != nil {
		h.failure(w, r, err)
		return
	}
	target, err := uuid.Parse(r.PostForm.Get("user_id"))
	if err != nil {
		h.group(w, r, "Проверь ID участника.", 422)
		return
	}
	if err := h.chats.RemoveGroupParticipant(r.Context(), chats_service.RemoveGroupParticipantCommand{GroupID: id, RequesterID: requester(r), TargetID: target}); err != nil {
		status, message := publicError(err)
		h.group(w, r, message, status)
		return
	}
	if target == requester(r) {
		redirectPage(w, r, "/app")
		return
	}
	redirectPage(w, r, "/app/groups/"+id.String())
}
