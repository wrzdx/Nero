package web_handlers

import (
	"bytes"
	"context"
	"errors"
	"messenger/internal/core/auth"
	"messenger/internal/core/domain"
	auth_service "messenger/internal/features/auth/service"
	web_views "messenger/internal/web/views"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"go.uber.org/zap"
)

const accessCookieName = "nero_access"

type AuthService interface {
	Register(context.Context, auth_service.RegisterCommand) (domain.User, auth.TokenPair, error)
	Login(context.Context, string, string) (auth.TokenPair, error)
}

type RefreshCookieManager interface {
	SetRefreshToken(http.ResponseWriter, string)
}

type AccessTokenParser interface {
	ParseAccessToken(string) (auth.ParsedAccessToken, error)
}

type AuthHandler struct {
	service       AuthService
	refreshCookie RefreshCookieManager
	tokens        AccessTokenParser
	accessTTL     time.Duration
	secure        bool
	log           *zap.Logger
}

func NewAuthHandler(service AuthService, refreshCookie RefreshCookieManager, tokens AccessTokenParser, accessTTL time.Duration, secure bool, log *zap.Logger) *AuthHandler {
	return &AuthHandler{service: service, refreshCookie: refreshCookie, tokens: tokens, accessTTL: accessTTL, secure: secure, log: log}
}

func (h *AuthHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, web_views.Register())
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, web_views.Login())
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	data, password, status := readForm(w, r)
	if status != http.StatusOK {
		h.form(w, r, status, true, data)
		return
	}

	// Reuse domain validation; collect errors before running bcrypt or writing to DB.
	_, profileErr := domain.NewUserProfile(data.Username, data.FirstName, nil, nil)
	if profileErr != nil {
		data.Errors = profileErrors(profileErr)
	}
	if auth.ValidatePassword(password) != nil {
		data.Errors["password"] = "Пароль: от 15 символов и не более 72 байт. Кириллица занимает больше одного байта."
	}
	if len(data.Errors) > 0 {
		h.form(w, r, http.StatusUnprocessableEntity, true, data)
		return
	}

	_, tokens, err := h.service.Register(r.Context(), auth_service.RegisterCommand{
		Username: data.Username, FirstName: data.FirstName, Password: password,
	})
	if err != nil {
		status = h.serviceError(&data, err)
		h.form(w, r, status, true, data)
		return
	}
	h.signedIn(w, r, tokens)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	data, password, status := readForm(w, r)
	if status != http.StatusOK {
		h.form(w, r, status, false, data)
		return
	}
	if data.Username == "" {
		data.Errors["username"] = "Введите имя пользователя."
	}
	if password == "" {
		data.Errors["password"] = "Введите пароль."
	}
	if len(data.Errors) > 0 {
		h.form(w, r, http.StatusUnprocessableEntity, false, data)
		return
	}

	tokens, err := h.service.Login(r.Context(), data.Username, password)
	if err != nil {
		status = h.serviceError(&data, err)
		h.form(w, r, status, false, data)
		return
	}
	h.signedIn(w, r, tokens)
}

func (h *AuthHandler) Welcome(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(accessCookieName)
	if err == nil {
		_, err = h.tokens.ParseAccessToken(cookie.Value)
	}
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	h.render(w, r, http.StatusOK, web_views.Welcome())
}

func readForm(w http.ResponseWriter, r *http.Request) (web_views.AuthFormData, string, int) {
	data := web_views.AuthFormData{Errors: make(map[string]string)}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		data.Message = "Не удалось прочитать форму. Обновите страницу и попробуйте ещё раз."
		return data, "", http.StatusUnsupportedMediaType
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		data.Message = "Не удалось прочитать форму. Обновите страницу и попробуйте ещё раз."
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return data, "", http.StatusRequestEntityTooLarge
		}
		return data, "", http.StatusBadRequest
	}
	// Query parameters must not supply credentials that are absent from the body.
	data.Username = strings.TrimSpace(r.PostForm.Get("username"))
	data.FirstName = strings.TrimSpace(r.PostForm.Get("first_name"))
	return data, r.PostForm.Get("password"), http.StatusOK
}

func profileErrors(err error) map[string]string {
	fields := make(map[string]string)
	if detailed, ok := errors.AsType[domain.DetailedError](err); ok {
		for key := range detailed.Details {
			switch key {
			case "username":
				fields[key] = "От 5 до 32 символов: латиница, цифры и _."
			case "first_name":
				fields[key] = "Введите имя длиной от 1 до 64 символов."
			}
		}
	}
	return fields
}

func (h *AuthHandler) serviceError(data *web_views.AuthFormData, err error) int {
	switch {
	case errors.Is(err, domain.ErrAlreadyExists):
		data.Errors["username"] = "Это имя пользователя уже занято."
		return http.StatusConflict
	case errors.Is(err, domain.ErrInvalidUserProfile):
		data.Errors = profileErrors(err)
		data.Message = "Проверьте данные профиля."
		return http.StatusUnprocessableEntity
	case errors.Is(err, auth.ErrInvalidPassword):
		data.Errors["password"] = "Пароль должен содержать от 15 символов и не более 72 байт."
		return http.StatusUnprocessableEntity
	case errors.Is(err, auth.ErrInvalidCredentials):
		data.Message = "Неверное имя пользователя или пароль."
		return http.StatusUnauthorized
	default:
		h.log.Error("web authentication failed", zap.Error(err))
		data.Message = "Сейчас не удалось выполнить запрос. Попробуйте позже."
		return http.StatusInternalServerError
	}
}

func (h *AuthHandler) signedIn(w http.ResponseWriter, r *http.Request, tokens auth.TokenPair) {
	h.refreshCookie.SetRefreshToken(w, tokens.Refresh)
	// The HTML UI keeps the access token in an HttpOnly cookie, never in localStorage.
	// JSON API clients still use the existing Authorization: Bearer contract.
	http.SetCookie(w, &http.Cookie{
		Name: accessCookieName, Value: tokens.Access, Path: "/", HttpOnly: true,
		Secure: h.secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(h.accessTTL.Seconds()), Expires: time.Now().Add(h.accessTTL),
	})
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/welcome")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/welcome", http.StatusSeeOther)
}

func (h *AuthHandler) form(w http.ResponseWriter, r *http.Request, status int, register bool, data web_views.AuthFormData) {
	var component templ.Component
	if register {
		component = web_views.RegisterPage(data)
		if r.Header.Get("HX-Request") == "true" {
			component = web_views.RegisterForm(data)
		}
	} else {
		component = web_views.LoginPage(data)
		if r.Header.Get("HX-Request") == "true" {
			component = web_views.LoginForm(data)
		}
	}
	h.render(w, r, status, component)
}

func (h *AuthHandler) render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	var output bytes.Buffer
	if err := component.Render(r.Context(), &output); err != nil {
		h.log.Error("render web page", zap.Error(err))
		http.Error(w, "Не удалось открыть страницу.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "HX-Request")
	w.WriteHeader(status)
	if _, err := w.Write(output.Bytes()); err != nil {
		h.log.Error("write web page", zap.Error(err))
	}
}
