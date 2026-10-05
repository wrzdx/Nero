package http_response

import (
	"encoding/json"
	logger "github.com/wrzdx/Nero/internal/core/logger"
	"net/http"

	"go.uber.org/zap"
)

type HTTPSender struct {
	log         *logger.Logger
	rw          http.ResponseWriter
	errorMapper ErrorMapper
}

func NewHTTPSender(
	log *logger.Logger,
	rw http.ResponseWriter,
	errorMapper ErrorMapper,
) *HTTPSender {
	return &HTTPSender{
		log:         log,
		rw:          rw,
		errorMapper: errorMapper,
	}
}

type APIResponse struct {
	Data  any             `json:"data,omitempty"`
	Error *APIErrorDetail `json:"error,omitempty"`
}

type APIErrorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (s *HTTPSender) Error(err error) {
	errResp := s.errorMapper(err)
	if errResp.StatusCode >= 500 {
		s.log.Error("CRITICAL_SERVER_ERROR", zap.Error(err))
	} else {
		s.log.Warn(err.Error())
	}

	response := APIResponse{
		Error: &APIErrorDetail{
			Code:    errResp.Code,
			Message: errResp.Message,
			Fields:  errResp.Fields,
		},
	}
	s.write(errResp.StatusCode, response)
}

func (s *HTTPSender) OK(statusCode int, data any) {
	response := APIResponse{
		Data: data,
	}
	s.write(statusCode, response)
}

func (s *HTTPSender) write(statusCode int, response APIResponse) {
	if statusCode == http.StatusNoContent {
		s.rw.WriteHeader(http.StatusNoContent)
		return
	}

	s.rw.Header().Set("Content-Type", "application/json")
	s.rw.WriteHeader(statusCode)
	if err := json.NewEncoder(s.rw).Encode(response); err != nil {
		s.log.Error("write HTTP response", zap.Error(err))
	}
}
