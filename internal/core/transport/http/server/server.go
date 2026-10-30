package http_server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	logger "github.com/wrzdx/Nero/internal/core/logger"

	"go.uber.org/zap"
)

type HTTPServer struct {
	router     http.Handler
	config     Config
	log        *logger.Logger
	shutdownWS func(context.Context) error
}

func NewHTTPServer(
	config Config,
	log *logger.Logger,
	router http.Handler,
	shutdownWS func(context.Context) error,
) *HTTPServer {
	return &HTTPServer{
		router:     router,
		config:     config,
		log:        log,
		shutdownWS: shutdownWS,
	}
}

func (s *HTTPServer) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:    s.config.Addr,
		Handler: s.router,
	}
	s.log.Warn(
		"start HTTP server",
		zap.String("addr", s.config.Addr),
		zap.Strings("allowed_origins", s.config.AllowedOrigins),
	)
	ch := make(chan error, 1)

	go func() {
		defer close(ch)
		err := server.ListenAndServe()

		if !errors.Is(err, http.ErrServerClosed) {
			ch <- err
		}
	}()

	var serveErr error

	select {
	case err := <-ch:
		if err != nil {
			serveErr = fmt.Errorf("listen and serve HTTP: %w", err)
		}
	case <-ctx.Done():
		s.log.Warn("shutdown HTTP server...")
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		s.config.ShutdownTimeout,
	)
	defer cancel()
	httpErr := server.Shutdown(shutdownCtx)
	if httpErr != nil {
		_ = server.Close()
		httpErr = fmt.Errorf("shutdown HTTP server: %w", httpErr)
	}

	wsErr := s.shutdownWS(shutdownCtx)
	if wsErr != nil {
		wsErr = fmt.Errorf("shutdown WebSocket handlers: %w", wsErr)
	}

	if err := errors.Join(serveErr, httpErr, wsErr); err != nil {
		return err
	}

	s.log.Warn("HTTP server stopped")

	return nil
}
