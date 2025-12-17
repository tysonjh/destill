package provider

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestWrapError_InvalidURL(t *testing.T) {
	err := fmt.Errorf("%w: https://invalid.com", ErrInvalidURL)
	wrapped := WrapError(err)

	if wrapped == nil {
		t.Fatal("WrapError() returned nil, want error")
	}

	userErr, ok := wrapped.(*UserError)
	if !ok {
		t.Fatalf("WrapError() returned %T, want *UserError", wrapped)
	}

	if userErr.Message != "Invalid build URL" {
		t.Errorf("Message = %q, want %q", userErr.Message, "Invalid build URL")
	}

	if !strings.Contains(userErr.Hint, "Supported formats") {
		t.Errorf("Hint should contain 'Supported formats', got %q", userErr.Hint)
	}

	if !strings.Contains(userErr.Hint, "buildkite.com") {
		t.Errorf("Hint should contain 'buildkite.com', got %q", userErr.Hint)
	}

	if !strings.Contains(userErr.Hint, "github.com") {
		t.Errorf("Hint should contain 'github.com', got %q", userErr.Hint)
	}

	if !errors.Is(wrapped, ErrInvalidURL) {
		t.Error("errors.Is(wrapped, ErrInvalidURL) = false, want true")
	}
}

func TestWrapError_AuthFailed(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "401 Unauthorized message",
			err:  errors.New("401 Unauthorized"),
		},
		{
			name: "ErrAuthFailed sentinel",
			err:  ErrAuthFailed,
		},
		{
			name: "wrapped ErrAuthFailed",
			err:  fmt.Errorf("request failed: %w", ErrAuthFailed),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := WrapError(tt.err)

			if wrapped == nil {
				t.Fatal("WrapError() returned nil, want error")
			}

			userErr, ok := wrapped.(*UserError)
			if !ok {
				t.Fatalf("WrapError() returned %T, want *UserError", wrapped)
			}

			if userErr.Message != "Authentication failed" {
				t.Errorf("Message = %q, want %q", userErr.Message, "Authentication failed")
			}

			if !strings.Contains(userErr.Hint, "API token") {
				t.Errorf("Hint should contain 'API token', got %q", userErr.Hint)
			}

			if !strings.Contains(userErr.Hint, "BUILDKITE_API_TOKEN") {
				t.Errorf("Hint should contain 'BUILDKITE_API_TOKEN', got %q", userErr.Hint)
			}

			if !strings.Contains(userErr.Hint, "GITHUB_TOKEN") {
				t.Errorf("Hint should contain 'GITHUB_TOKEN', got %q", userErr.Hint)
			}
		})
	}
}

func TestWrapError_BuildNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "404 Not Found message",
			err:  errors.New("404 Not Found"),
		},
		{
			name: "ErrBuildNotFound sentinel",
			err:  ErrBuildNotFound,
		},
		{
			name: "wrapped ErrBuildNotFound",
			err:  fmt.Errorf("build fetch failed: %w", ErrBuildNotFound),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := WrapError(tt.err)

			if wrapped == nil {
				t.Fatal("WrapError() returned nil, want error")
			}

			userErr, ok := wrapped.(*UserError)
			if !ok {
				t.Fatalf("WrapError() returned %T, want *UserError", wrapped)
			}

			if userErr.Message != "Build not found" {
				t.Errorf("Message = %q, want %q", userErr.Message, "Build not found")
			}

			if !strings.Contains(userErr.Hint, "build URL is correct") {
				t.Errorf("Hint should contain 'build URL is correct', got %q", userErr.Hint)
			}

			if !strings.Contains(userErr.Hint, "you have access") {
				t.Errorf("Hint should contain 'you have access', got %q", userErr.Hint)
			}
		})
	}
}

func TestWrapError_OtherErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "rate limited",
			err:  ErrRateLimited,
		},
		{
			name: "network timeout",
			err:  ErrNetworkTimeout,
		},
		{
			name: "generic error",
			err:  errors.New("something went wrong"),
		},
		{
			name: "500 Internal Server Error",
			err:  errors.New("500 Internal Server Error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := WrapError(tt.err)

			if wrapped == nil {
				t.Fatal("WrapError() returned nil, want error")
			}

			// Should return the original error unchanged
			if wrapped != tt.err {
				t.Errorf("WrapError() = %v, want original error %v", wrapped, tt.err)
			}

			// Should not be wrapped in UserError
			if _, ok := wrapped.(*UserError); ok {
				t.Error("WrapError() returned *UserError, want original error unchanged")
			}
		})
	}
}

func TestWrapError_NilError(t *testing.T) {
	wrapped := WrapError(nil)

	if wrapped != nil {
		t.Errorf("WrapError(nil) = %v, want nil", wrapped)
	}
}

func TestUserError_Error(t *testing.T) {
	tests := []struct {
		name     string
		userErr  *UserError
		wantMsg  string
		wantHint string
		wantErr  string
	}{
		{
			name: "message only",
			userErr: &UserError{
				Message: "Something went wrong",
			},
			wantMsg: "Something went wrong",
		},
		{
			name: "message with hint",
			userErr: &UserError{
				Message: "Something went wrong",
				Hint:    "Try doing this instead",
			},
			wantMsg:  "Something went wrong",
			wantHint: "Hint: Try doing this instead",
		},
		{
			name: "message with underlying error",
			userErr: &UserError{
				Message: "Something went wrong",
				Err:     errors.New("original error"),
			},
			wantMsg: "Something went wrong",
			wantErr: "Details: original error",
		},
		{
			name: "message with hint and error",
			userErr: &UserError{
				Message: "Something went wrong",
				Hint:    "Try doing this instead",
				Err:     errors.New("original error"),
			},
			wantMsg:  "Something went wrong",
			wantHint: "Hint: Try doing this instead",
			wantErr:  "Details: original error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.userErr.Error()

			if !strings.Contains(got, tt.wantMsg) {
				t.Errorf("Error() should contain %q, got %q", tt.wantMsg, got)
			}

			if tt.wantHint != "" && !strings.Contains(got, tt.wantHint) {
				t.Errorf("Error() should contain %q, got %q", tt.wantHint, got)
			}

			if tt.wantErr != "" && !strings.Contains(got, tt.wantErr) {
				t.Errorf("Error() should contain %q, got %q", tt.wantErr, got)
			}

			// Check order: Message first
			msgIdx := strings.Index(got, tt.wantMsg)
			if msgIdx != 0 {
				t.Errorf("Message should be at start, found at index %d", msgIdx)
			}

			// If hint exists, it should come after message
			if tt.wantHint != "" {
				hintIdx := strings.Index(got, tt.wantHint)
				if hintIdx <= msgIdx {
					t.Errorf("Hint should come after Message, got hint at %d, msg at %d", hintIdx, msgIdx)
				}
			}

			// If error exists, it should come after hint (if present) or message
			if tt.wantErr != "" {
				errIdx := strings.Index(got, tt.wantErr)
				if tt.wantHint != "" {
					hintIdx := strings.Index(got, tt.wantHint)
					if errIdx <= hintIdx {
						t.Errorf("Details should come after Hint, got details at %d, hint at %d", errIdx, hintIdx)
					}
				} else if errIdx <= msgIdx {
					t.Errorf("Details should come after Message, got details at %d, msg at %d", errIdx, msgIdx)
				}
			}
		})
	}
}

func TestUserError_Unwrap(t *testing.T) {
	tests := []struct {
		name    string
		userErr *UserError
		want    error
	}{
		{
			name: "with underlying error",
			userErr: &UserError{
				Message: "Something went wrong",
				Err:     ErrAuthFailed,
			},
			want: ErrAuthFailed,
		},
		{
			name: "without underlying error",
			userErr: &UserError{
				Message: "Something went wrong",
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.userErr.Unwrap()

			if got != tt.want {
				t.Errorf("Unwrap() = %v, want %v", got, tt.want)
			}

			// Test that errors.Is works correctly
			if tt.want != nil {
				if !errors.Is(tt.userErr, tt.want) {
					t.Errorf("errors.Is(userErr, %v) = false, want true", tt.want)
				}
			}
		})
	}
}
