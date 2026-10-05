package main

import (
	"context"
	"fmt"
	auth_bcrypt "messenger/internal/core/auth/bcrypt"
	auth_cookie "messenger/internal/core/auth/cookie"
	auth_jwt "messenger/internal/core/auth/jwt"
	config "messenger/internal/core/config"
	logger "messenger/internal/core/logger"
	"messenger/internal/core/postgres"
	http_middleware "messenger/internal/core/transport/http/middleware"
	http_server "messenger/internal/core/transport/http/server"
	web_handlers "messenger/internal/web/handlers"
	"net/http"

	auth_postgres_repository "messenger/internal/features/auth/repository/postgres"
	auth_service "messenger/internal/features/auth/service"
	auth_transport_http "messenger/internal/features/auth/transport/http"
	chats_postgres_repository "messenger/internal/features/chats/repository/postgres"
	chats_service "messenger/internal/features/chats/service"
	chats_transport_http "messenger/internal/features/chats/transport/http"
	messages_postgres_repository "messenger/internal/features/messages/repository/postgres"
	messages_service "messenger/internal/features/messages/service"
	messages_transport_http "messenger/internal/features/messages/transport/http"
	realtime_transport_ws "messenger/internal/features/realtime/transport/ws"
	users_postgres_repository "messenger/internal/features/users/repository/postgres"
	users_service "messenger/internal/features/users/service"
	users_transport_http "messenger/internal/features/users/transport/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func main() {
	cfg := config.NewConfigMust()
	time.Local = cfg.TimeZone

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
		os.Interrupt,
	)
	defer cancel()

	logger, err := logger.NewLogger(logger.NewConfigMust())
	if err != nil {
		fmt.Println("failed to init application logger:", err)
		os.Exit(1)
	}

	defer logger.Close()

	logger.Debug("application time zone", zap.Any("zone", time.Local))
	logger.Debug("initializing postgres connection pool")
	postgresConfig := postgres.NewConfigMust()
	pool, err := postgres.NewPool(
		ctx,
		postgresConfig,
	)
	if err != nil {
		logger.Fatal("failed to init postgres connection pool", zap.Error(err))
	}

	defer pool.Close()

	logger.Debug("initializing feature", zap.String("feature", "auth"))
	usersRepository := users_postgres_repository.NewUsersRepository(
		pool,
		postgresConfig.Timeout,
	)
	sessionsRepository := auth_postgres_repository.NewSessionsRepository(
		pool,
		postgresConfig.Timeout,
	)
	hasher := auth_bcrypt.NewBcryptHasher()
	jwtProvider := auth_jwt.NewTokenProvider(auth_jwt.NewConfigMust())
	txManager := postgres.NewTransactionManager(pool)
	authConfig := auth_service.AuthConfig{
		AccessTokenTTL: cfg.AccessTokenTTL,
		SessionTTL:     cfg.SessionTTL,
	}
	cookieManager := auth_cookie.NewCookieManager(
		authConfig.SessionTTL,
		cfg.Environment.IsProduction(),
		"/api/v1/auth",
	)

	authService, err := auth_service.NewAuthService(
		usersRepository,
		sessionsRepository,
		hasher,
		jwtProvider,
		txManager,
		authConfig,
	)

	if err != nil {
		logger.Fatal("failed create auth service", zap.Error(err))
	}

	authHTTP := auth_transport_http.NewAuthHTTPHandler(
		authService, cookieManager,
	)

	logger.Debug("initializing feature", zap.String("feature", "realtime"))
	realtimeHub := realtime_transport_ws.NewHub()
	realtimeWS := realtime_transport_ws.NewWSHandler(ctx, jwtProvider, realtimeHub)

	logger.Debug("initializing feature", zap.String("feature", "users"))
	usersService := users_service.NewUsersService(
		usersRepository,
		sessionsRepository,
		txManager,
	)
	usersHTTP := users_transport_http.NewUsersHandler(usersService, cookieManager)

	logger.Debug("initializing feature", zap.String("feature", "chats"))
	chatsRepository := chats_postgres_repository.NewChatsRepository(
		pool,
		postgresConfig.Timeout,
	)
	chatsService := chats_service.NewChatsService(
		chatsRepository,
		usersRepository,
		txManager,
	)
	chatsHTTP := chats_transport_http.NewChatsHandler(chatsService)

	logger.Debug("initializing feature", zap.String("feature", "messages"))
	messagesRepository := messages_postgres_repository.NewRepository(
		pool,
		postgresConfig.Timeout,
	)
	notifier := realtime_transport_ws.NewNotifier(realtimeHub, messagesRepository, logger)
	messagesService := messages_service.NewMessagesService(
		messagesRepository,
		messagesRepository,
		txManager,
		notifier,
	)
	messagesHTTP := messages_transport_http.NewMessagesHandler(messagesService)

	logger.Debug("initializing HTTP server")
	httpConfig := http_server.NewConfigMust()
	authMW := http_middleware.Auth(jwtProvider)
	router := chi.NewRouter()
	router.Use(
		http_middleware.CORS(httpConfig.AllowedOrigins),
		http_middleware.RequestID(),
		http_middleware.Logging(logger),
		http_middleware.Trace(),
		http_middleware.Recovery(),
	)

	routerV1 := chi.NewRouter()
	routerV1.Mount("/auth", authHTTP.Router(authMW))
	routerV1.Mount("/users", usersHTTP.Router(authMW))
	routerV1.Mount("/chats", chatsHTTP.Router(authMW))
	routerV1.Mount(
		"/chats/{chat_id}/messages",
		messagesHTTP.Router(authMW),
	)

	routerV1.Get("/ws", realtimeWS.ServeHTTP)

	router.Mount("/api/v1", routerV1)
	webAuth := web_handlers.NewAuthHandler(authService, cookieManager, jwtProvider, cfg.AccessTokenTTL, cfg.Environment.IsProduction(), logger.Logger)
	router.Group(func(pages chi.Router) {
		pages.Use(http.NewCrossOriginProtection().Handler)
		pages.Get("/register", webAuth.RegisterPage)
		pages.Post("/register", webAuth.Register)
		pages.Get("/login", webAuth.LoginPage)
		pages.Post("/login", webAuth.Login)
		pages.Get("/welcome", webAuth.Welcome)
	})
	router.Handle(
		"/static/*",
		http.StripPrefix(
			"/static/",
			http.FileServer(http.Dir(cfg.StaticDir)),
		),
	)
	httpServer := http_server.NewHTTPServer(
		httpConfig,
		logger,
		router,
	)

	if err := httpServer.Run(ctx); err != nil {
		logger.Error("HTTP server run error", zap.Error(err))
	}
}
