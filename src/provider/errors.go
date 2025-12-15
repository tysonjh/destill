package provider

import (
	"errors"
	"fmt"
)

var (
	ErrAuthFailed     = errors.New("authentication failed")
	ErrBuildNotFound  = errors.New("build not found")
	ErrRateLimited    = errors.New("rate limited")
	ErrNetworkTimeout = errors.New("network timeout")
)

// UserError wraps errors with user-friendly messages
type UserError struct {
	Message string
	Hint    string
	Err     error
}

func (e *UserError) Error() string {
	msg := e.Message
	if e.Hint != "" {
		msg += "\n\nHint: " + e.Hint
	}
	if e.Err != nil {
		msg += fmt.Sprintf("\n\nDetails: %v", e.Err)
	}
	return msg
}

func (e *UserError) Unwrap() error {
	return e.Err
}

// WrapError converts API errors to user-friendly messages
func WrapError(err error) error {
	if err == nil {
		return nil
	}

	// Check for common error patterns
	msg := err.Error()

	if errors.Is(err, ErrInvalidURL) {
		return &UserError{
			Message: "Invalid build URL",
			Hint:    "Supported formats:\n  - https://buildkite.com/org/pipeline/builds/123\n  - https://github.com/owner/repo/actions/runs/456",
			Err:     err,
		}
	}

	if msg == "401 Unauthorized" || errors.Is(err, ErrAuthFailed) {
		return &UserError{
			Message: "Authentication failed",
			Hint:    "Check that your API token is valid and has the correct permissions.\n  - Buildkite: Set BUILDKITE_API_TOKEN\n  - GitHub: Set GITHUB_TOKEN",
			Err:     err,
		}
	}

	if msg == "404 Not Found" || errors.Is(err, ErrBuildNotFound) {
		return &UserError{
			Message: "Build not found",
			Hint:    "Check that the build URL is correct and you have access to the repository.",
			Err:     err,
		}
	}

	return err
}
