package runtime

import (
	"errors"
	"fmt"
	"os"

	"github.com/EndersonPro/flutree/internal/domain"
)

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
		os.Exit(appErr.Code)
	}
	fmt.Fprintln(os.Stderr, "[unexpected] Command failed.")
	os.Exit(1)
}
