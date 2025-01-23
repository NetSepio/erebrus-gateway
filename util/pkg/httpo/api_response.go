package httpo

import (
	"github.com/gin-gonic/gin"
)

// ApiSuccessResponse defines struct used for http response
type ApiSuccessResponse[T any] struct {
	// Custom status code
	StatusCode int `json:"status,omitempty"`

	Error string `json:"error,omitempty"`

	Message string `json:"message,omitempty"`
	Payload T      `json:"payload,omitempty"`
}

// ApiErrorResponse defines struct used for http response
type ApiErrorResponse[T any] struct {
	// Custom status code
	StatusCode int `json:"status,omitempty"`

	ErrorStr string `json:"error,omitempty"`

	Message string `json:"message,omitempty"`
	Payload T      `json:"payload,omitempty"`
}

func (r *ApiErrorResponse[T]) Error() string {
	return r.ErrorStr
}

// Sends ApiResponse with gin context and standard statusCode
func (apiRes *ApiSuccessResponse[T]) Send(c *gin.Context, statusCode int) {
	c.JSON(statusCode, apiRes)
}

// Sends ApiResponse with gin context and with customStatusCode as standard one
func (apiRes *ApiSuccessResponse[T]) SendD(c *gin.Context) {
	c.JSON(apiRes.StatusCode, apiRes)
}

// Sends ApiResponse with gin context and standard statusCode
func (apiRes *ApiErrorResponse[T]) Send(c *gin.Context, statusCode int) {
	c.JSON(statusCode, apiRes)
}

// Sends ApiResponse with gin context and with customStatusCode as standard one
func (apiRes *ApiErrorResponse[T]) SendD(c *gin.Context) {
	c.JSON(apiRes.StatusCode, apiRes)
}

// NewSuccessResponse returns ApiResponse for success without payload
func NewSuccessResponse(customStatusCode int, message string) *ApiSuccessResponse[any] {
	return &ApiSuccessResponse[any]{
		StatusCode: customStatusCode,
		Message:    message,
	}
}

// NewSuccessResponse returns ApiResponse for success with payload
func NewSuccessResponseP[T any](customStatusCode int, message string, payload T) *ApiSuccessResponse[T] {
	return &ApiSuccessResponse[T]{
		StatusCode: customStatusCode,
		Message:    message,
		Payload:    payload,
	}
}

// NewSuccessResponse returns ApiResponse for error
func NewErrorResponse(customStatusCode int, errorStr string) *ApiErrorResponse[any] {
	return &ApiErrorResponse[any]{
		StatusCode: customStatusCode,
		ErrorStr:   errorStr,
	}
}
