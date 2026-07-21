package service

import (
	"errors"
	"net/http"
	"time"
)

type UpstreamErrorKind string

const (
	UpstreamErrorAddressForbidden           UpstreamErrorKind = "address_forbidden"
	UpstreamErrorUnavailable                UpstreamErrorKind = "unavailable"
	UpstreamErrorAuthExpired                UpstreamErrorKind = "auth_expired"
	UpstreamErrorPermissionDenied           UpstreamErrorKind = "permission_denied"
	UpstreamErrorRateLimited                UpstreamErrorKind = "rate_limited"
	UpstreamErrorRemote                     UpstreamErrorKind = "upstream_error"
	UpstreamErrorResponseInvalid            UpstreamErrorKind = "response_invalid"
	UpstreamErrorEnvelopeInvalid            UpstreamErrorKind = "envelope_invalid"
	UpstreamErrorResponseTooLarge           UpstreamErrorKind = "response_too_large"
	UpstreamErrorExportDisabled             UpstreamErrorKind = "export_disabled"
	UpstreamErrorCredentialOriginMismatch   UpstreamErrorKind = "credential_origin_mismatch"
	UpstreamErrorTokenRotationResultUnknown UpstreamErrorKind = "token_rotation_result_unknown"
	UpstreamErrorDataMismatch               UpstreamErrorKind = "data_mismatch"
)

var (
	ErrUpstreamUnavailable                = errors.New("upstream is unavailable")
	ErrUpstreamAuthExpired                = errors.New("upstream authorization has expired")
	ErrUpstreamPermissionDenied           = errors.New("upstream permission denied")
	ErrUpstreamRateLimited                = errors.New("upstream rate limited")
	ErrUpstreamRemote                     = errors.New("upstream server error")
	ErrUpstreamResponseInvalid            = errors.New("upstream response is invalid")
	ErrUpstreamEnvelopeInvalid            = errors.New("upstream response envelope is invalid")
	ErrUpstreamResponseTooLarge           = errors.New("upstream response is too large")
	ErrUpstreamExportDisabled             = errors.New("upstream data export is disabled")
	ErrUpstreamCredentialOriginMismatch   = errors.New("upstream credentials are bound to another origin")
	ErrUpstreamTokenRotationResultUnknown = errors.New("upstream token rotation result is unknown")
	ErrUpstreamDataMismatch               = errors.New("upstream flow and data totals do not match")
	ErrUpstreamUserNotFound               = errors.New("upstream user was not found")
	ErrUpstreamUserIdentityConflict       = errors.New("upstream user identity conflicts with the requested ID")
)

type UpstreamUserIdentityConflictError struct {
	ExpectedID int64
	ActualID   int64
}

func (err *UpstreamUserIdentityConflictError) Error() string {
	return ErrUpstreamUserIdentityConflict.Error()
}

func (err *UpstreamUserIdentityConflictError) Unwrap() error {
	return ErrUpstreamUserIdentityConflict
}

// UpstreamRequestError deliberately exposes only classification data. It never
// retains an access token, request URL, response body, or raw transport error.
type UpstreamRequestError struct {
	Kind          UpstreamErrorKind
	Detail        string
	Method        string
	Endpoint      string
	StatusCode    int
	ContentType   string
	PayloadBytes  int64
	RetryAfter    time.Duration
	HasRetryAfter bool
	ResponseBytes int64
	LimitBytes    int64
}

func annotateUpstreamRequestError(err error, method, endpoint, contentType string, statusCode int, payloadBytes int64) error {
	var requestError *UpstreamRequestError
	if !errors.As(err, &requestError) || requestError == nil {
		return err
	}
	requestError.Method = method
	requestError.Endpoint = endpoint
	if statusCode > 0 {
		requestError.StatusCode = statusCode
	}
	if contentType != "" {
		requestError.ContentType = contentType
	}
	if payloadBytes > 0 {
		requestError.PayloadBytes = payloadBytes
	}
	return err
}

func (err *UpstreamRequestError) Error() string {
	if err == nil {
		return "upstream request failed"
	}
	if sentinel := err.sentinel(); sentinel != nil {
		return sentinel.Error()
	}
	return "upstream request failed"
}

func (err *UpstreamRequestError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.sentinel()
}

func (err *UpstreamRequestError) sentinel() error {
	switch err.Kind {
	case UpstreamErrorAddressForbidden:
		return ErrUpstreamAddressForbidden
	case UpstreamErrorUnavailable:
		return ErrUpstreamUnavailable
	case UpstreamErrorAuthExpired:
		return ErrUpstreamAuthExpired
	case UpstreamErrorPermissionDenied:
		return ErrUpstreamPermissionDenied
	case UpstreamErrorRateLimited:
		return ErrUpstreamRateLimited
	case UpstreamErrorRemote:
		return ErrUpstreamRemote
	case UpstreamErrorResponseInvalid:
		return ErrUpstreamResponseInvalid
	case UpstreamErrorEnvelopeInvalid:
		return ErrUpstreamEnvelopeInvalid
	case UpstreamErrorResponseTooLarge:
		return ErrUpstreamResponseTooLarge
	case UpstreamErrorExportDisabled:
		return ErrUpstreamExportDisabled
	case UpstreamErrorCredentialOriginMismatch:
		return ErrUpstreamCredentialOriginMismatch
	case UpstreamErrorTokenRotationResultUnknown:
		return ErrUpstreamTokenRotationResultUnknown
	case UpstreamErrorDataMismatch:
		return ErrUpstreamDataMismatch
	default:
		return nil
	}
}

func newUpstreamRequestError(kind UpstreamErrorKind) *UpstreamRequestError {
	return &UpstreamRequestError{Kind: kind}
}

func newUpstreamRequestErrorWithDetail(kind UpstreamErrorKind, detail string) *UpstreamRequestError {
	return &UpstreamRequestError{Kind: kind, Detail: detail}
}

func newUpstreamHTTPError(
	kind UpstreamErrorKind,
	status int,
	retryAfter time.Duration,
	hasRetryAfter bool,
) *UpstreamRequestError {
	return &UpstreamRequestError{
		Kind: kind, StatusCode: status, RetryAfter: retryAfter, HasRetryAfter: hasRetryAfter,
	}
}

func newUpstreamResponseTooLargeError(responseBytes, limitBytes int64) *UpstreamRequestError {
	return &UpstreamRequestError{
		Kind: UpstreamErrorResponseTooLarge, ResponseBytes: responseBytes, LimitBytes: limitBytes,
	}
}

func classifyUpstreamHTTPStatus(status int, retryAfter time.Duration, hasRetryAfter bool) error {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return nil
	case status == http.StatusUnauthorized:
		return newUpstreamHTTPError(UpstreamErrorAuthExpired, status, 0, false)
	case status == http.StatusForbidden:
		return newUpstreamHTTPError(UpstreamErrorPermissionDenied, status, 0, false)
	case status == http.StatusTooManyRequests:
		return newUpstreamHTTPError(UpstreamErrorRateLimited, status, retryAfter, hasRetryAfter)
	case status >= http.StatusInternalServerError:
		return newUpstreamHTTPError(UpstreamErrorRemote, status, 0, false)
	default:
		return newUpstreamHTTPError(UpstreamErrorResponseInvalid, status, 0, false)
	}
}
