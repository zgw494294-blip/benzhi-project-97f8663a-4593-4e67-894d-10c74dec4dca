package domain

import (
	"fmt"
	"strings"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func invalid(field, message string) error {
	return ValidationError{Field: field, Message: message}
}

type ValidationErrors struct {
	Issues []ValidationError `json:"issues"`
}

func (e ValidationErrors) Error() string {
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, issue.Error())
	}
	return strings.Join(parts, "；")
}

type ConflictError struct {
	Expected int64 `json:"expected"`
	Current  int64 `json:"current"`
}

func (e ConflictError) Error() string {
	return fmt.Sprintf("页面修订号已过期：期望 %d，当前 %d", e.Expected, e.Current)
}
