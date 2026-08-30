package devicesapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"
	tsclient "tailscale.com/client/tailscale/v2"
)

const (
	OAuthScope     = "devices:core:read"
	RequestTimeout = 15 * time.Second
)

type Config struct {
	Tailnet      string
	ClientID     string
	ClientSecret string
	BaseURL      *url.URL
	Transport    http.RoundTripper
}

type Device struct {
	Addresses          []string
	Name               string
	NodeID             string
	Tags               []string
	Hostname           string
	ConnectedToControl bool
	LastSeen           *time.Time
	NodeKey            string
	OS                 string
}

type Lister interface {
	List(context.Context) ([]Device, error)
}

type ErrorKind string

const (
	ErrorCanceled        ErrorKind = "canceled"
	ErrorTimeout         ErrorKind = "timeout"
	ErrorUnauthorized    ErrorKind = "unauthorized"
	ErrorForbidden       ErrorKind = "forbidden"
	ErrorRateLimited     ErrorKind = "rate-limited"
	ErrorUnavailable     ErrorKind = "unavailable"
	ErrorInvalidResponse ErrorKind = "invalid-response"
)

type RequestError struct {
	StatusCode int
	Kind       ErrorKind
	cause      error
}

func (e *RequestError) Error() string {
	return "devices API request failed"
}

func (e *RequestError) Unwrap() error {
	return e.cause
}

type Client struct {
	upstream *tsclient.Client
}

func New(config Config) *Client {
	httpClient := &http.Client{
		Transport: config.Transport,
		Timeout:   RequestTimeout,
	}
	return &Client{upstream: &tsclient.Client{
		BaseURL: config.BaseURL,
		Tailnet: config.Tailnet,
		Auth: &tsclient.OAuth{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			Scopes:       []string{OAuthScope},
		},
		HTTP: httpClient,
	}}
}

func (c *Client) List(ctx context.Context) ([]Device, error) {
	devices, err := c.upstream.Devices().List(ctx)
	if err != nil {
		return nil, sanitizeError(err)
	}

	result := make([]Device, 0, len(devices))
	for _, device := range devices {
		var lastSeen *time.Time
		if device.LastSeen != nil {
			value := device.LastSeen.Time
			lastSeen = &value
		}
		result = append(result, Device{
			Addresses:          append([]string(nil), device.Addresses...),
			Name:               device.Name,
			NodeID:             device.NodeID,
			Tags:               append([]string(nil), device.Tags...),
			Hostname:           device.Hostname,
			ConnectedToControl: device.ConnectedToControl,
			LastSeen:           lastSeen,
			NodeKey:            device.NodeKey,
			OS:                 device.OS,
		})
	}
	return result, nil
}

func sanitizeError(err error) error {
	requestError := &RequestError{Kind: ErrorUnavailable}
	switch {
	case errors.Is(err, context.Canceled):
		requestError.Kind = ErrorCanceled
		requestError.cause = context.Canceled
		return requestError
	case errors.Is(err, context.DeadlineExceeded):
		requestError.Kind = ErrorTimeout
		requestError.cause = context.DeadlineExceeded
		return requestError
	}

	var apiError tsclient.APIError
	if errors.As(err, &apiError) {
		requestError.StatusCode = apiError.Status
		requestError.Kind = kindForStatus(apiError.Status)
		return requestError
	}
	var oauthError *oauth2.RetrieveError
	if errors.As(err, &oauthError) && oauthError.Response != nil {
		requestError.StatusCode = oauthError.Response.StatusCode
		requestError.Kind = kindForStatus(oauthError.Response.StatusCode)
		return requestError
	}
	var syntaxError *json.SyntaxError
	var typeError *json.UnmarshalTypeError
	if errors.As(err, &syntaxError) || errors.As(err, &typeError) {
		requestError.Kind = ErrorInvalidResponse
	}
	return requestError
}

func kindForStatus(statusCode int) ErrorKind {
	switch statusCode {
	case http.StatusUnauthorized:
		return ErrorUnauthorized
	case http.StatusForbidden:
		return ErrorForbidden
	case http.StatusTooManyRequests:
		return ErrorRateLimited
	default:
		if statusCode >= http.StatusInternalServerError {
			return ErrorUnavailable
		}
		return ErrorInvalidResponse
	}
}
