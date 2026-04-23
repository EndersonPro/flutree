package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/EndersonPro/flutree/internal/domain"
)

type ErrorJSONOutput struct {
	Error ErrorInfo `json:"error"`
}

type ErrorInfo struct {
	Category string `json:"category"`
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

func ExitOnError(err error) {
	if err == nil {
		return
	}
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		fmt.Fprintf(os.Stderr, "[%s] %s\n", appErr.Category, appErr.Message)
		if appErr.Hint != "" {
			fmt.Fprintf(os.Stderr, "Hint: %s\n", appErr.Hint)
		}
		if appErr.Category == domain.CategoryInput {
			os.Exit(2)
		}
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "[unexpected] Command failed.")
	os.Exit(1)
}

func ExitOnErrorJSON(err error, jsonFlag bool) {
	if err == nil {
		return
	}
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		if jsonFlag {
			jsonErr := ErrorJSONOutput{
				Error: ErrorInfo{
					Category: string(appErr.Category),
					Code:    appErr.Code,
					Message: appErr.Message,
					Hint:    appErr.Hint,
				},
			}
			data, _ := json.MarshalIndent(jsonErr, "", "  ")
			fmt.Fprintln(os.Stderr, string(data))
		} else {
			fmt.Fprintf(os.Stderr, "[%s] %s\n", appErr.Category, appErr.Message)
			if appErr.Hint != "" {
				fmt.Fprintf(os.Stderr, "Hint: %s\n", appErr.Hint)
			}
		}
		if appErr.Category == domain.CategoryInput {
			os.Exit(2)
		}
		os.Exit(1)
	}
	if jsonFlag {
		jsonErr := ErrorJSONOutput{
			Error: ErrorInfo{
				Category: string(domain.CategoryUnexpected),
				Code:    1,
				Message: "Command failed.",
			},
		}
		data, _ := json.MarshalIndent(jsonErr, "", "  ")
		fmt.Fprintln(os.Stderr, string(data))
	} else {
		fmt.Fprintln(os.Stderr, "[unexpected] Command failed.")
	}
	os.Exit(1)
}
