package http_server

import (
	"context"
	"errors"
	"fmt"
	logger "github.com/wrzdx/Nero/internal/core/logger"
	"net/http"

	"go.uber.org/zap"
)

type HTTPServer struct {
	router http.Handler
	config Config
	log    *logger.Logger
}

func NewHTTPServer(config Config, log *logger.Logger, router http.Handler) *HTTPServer {
	return &HTTPServer{
		router: router,
		config: config,
		log:    log,
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

	select {
	case err := <-ch:
		if err != nil {
			return fmt.Errorf("listen and serve HTTP: %w", err)
		}
	case <-ctx.Done():
		s.log.Warn("shutdown HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			s.config.ShutdownTimeout,
		)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()

			return fmt.Errorf("shutdown HTTP server: %w", err)
		}

		s.log.Warn("HTTP server stopped")
	}

	return nil
}
