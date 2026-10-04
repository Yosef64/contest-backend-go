// Package httpx holds the shared error type, JSON responses and request
// binding/validation used by every handler.
package httpx

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
)

// Error is an API error rendered as {"error": {code, message, fields}}.
type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func NewError(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

func BadRequest(msg string) *Error    { return NewError(http.StatusBadRequest, "bad_request", msg) }
func Unauthorized(msg string) *Error  { return NewError(http.StatusUnauthorized, "unauthorized", msg) }
func Forbidden(msg string) *Error     { return NewError(http.StatusForbidden, "forbidden", msg) }
func NotFound(what string) *Error     { return NewError(http.StatusNotFound, "not_found", what+" not found") }
func Conflict(code, msg string) *Error { return NewError(http.StatusConflict, code, msg) }

// Invalid reports field-level validation problems.
func Invalid(fields map[string]string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Please fix the highlighted fields.", Fields: fields}
}

// Field is a shorthand for a single invalid field.
func Field(name, msg string) *Error { return Invalid(map[string]string{name: msg}) }

// Handler is a gin handler that returns an error instead of writing it.
type Handler func(c *gin.Context) error

// H adapts a Handler, rendering returned errors consistently.
func H(fn Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := fn(c); err != nil {
			Fail(c, err)
		}
	}
}

// Fail writes err as an API error. Unknown errors become 500s and are logged
// without leaking details to the client.
func Fail(c *gin.Context, err error) {
	var apiErr *Error
	switch {
	case errors.As(err, &apiErr):
	case errors.Is(err, pgx.ErrNoRows):
		apiErr = NotFound("Resource")
	default:
		slog.Error("request failed", "method", c.Request.Method, "path", c.FullPath(), "err", err)
		apiErr = NewError(http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
	}
	c.AbortWithStatusJSON(apiErr.Status, gin.H{"error": apiErr})
}

func OK(c *gin.Context, body any) error {
	c.JSON(http.StatusOK, body)
	return nil
}

func Created(c *gin.Context, body any) error {
	c.JSON(http.StatusCreated, body)
	return nil
}

func NoContent(c *gin.Context) error {
	c.Status(http.StatusNoContent)
	return nil
}

func init() {
	// Report validation errors using JSON field names.
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
			if name == "-" || name == "" {
				return f.Name
			}
			return name
		})
	}
}

// Bind decodes the JSON body into dst and validates its `binding` tags.
func Bind(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		var verrs validator.ValidationErrors
		if errors.As(err, &verrs) {
			fields := map[string]string{}
			for _, fe := range verrs {
				fields[fe.Field()] = message(fe)
			}
			return Invalid(fields)
		}
		return BadRequest("The request body is not valid JSON.")
	}
	return nil
}

func message(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "This field is required."
	case "email":
		return "Enter a valid email address."
	case "min":
		if fe.Kind() == reflect.String {
			return fmt.Sprintf("Must be at least %s characters.", fe.Param())
		}
		if fe.Kind() == reflect.Slice {
			return fmt.Sprintf("Add at least %s.", fe.Param())
		}
		return fmt.Sprintf("Must be at least %s.", fe.Param())
	case "max":
		if fe.Kind() == reflect.String {
			return fmt.Sprintf("Must be at most %s characters.", fe.Param())
		}
		return fmt.Sprintf("Must be at most %s.", fe.Param())
	case "oneof":
		return "Choose one of: " + strings.ReplaceAll(fe.Param(), " ", ", ") + "."
	case "gtfield":
		return "Must be after " + strings.ToLower(fe.Param()) + "."
	case "url":
		return "Enter a valid URL."
	default:
		return "This value is not valid."
	}
}
