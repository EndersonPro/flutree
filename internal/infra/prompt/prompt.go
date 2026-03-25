package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

type Adapter struct {
	in *bufio.Reader
}

func New() *Adapter { return &Adapter{in: bufio.NewReader(os.Stdin)} }

func (a *Adapter) Confirm(message string, nonInteractive, assumeYes bool) (bool, error) {
	if assumeYes {
		return true, nil
	}
	if nonInteractive {
		return false, domain.NewError(domain.CategoryInput, 2, "Confirmation required in non-interactive mode.", "Re-run interactively or provide --yes.", nil)
	}
	fmt.Printf("%s [y/N]: ", message)
	text, err := a.in.ReadString('\n')
	if err != nil {
		return false, domain.NewError(domain.CategoryInput, 2, "Prompt aborted by user.", "", err)
	}
	v := strings.ToLower(strings.TrimSpace(text))
	return v == "y" || v == "yes", nil
}

func (a *Adapter) ConfirmWithToken(message, token string, nonInteractive, assumeYes bool) (bool, error) {
	if assumeYes {
		return true, nil
	}
	if nonInteractive {
		return false, domain.NewError(domain.CategoryInput, 2, "Final confirmation token required in non-interactive mode.", "Re-run interactively or pass --yes.", nil)
	}
	fmt.Printf("%s Type '%s' to continue: ", message, token)
	text, err := a.in.ReadString('\n')
	if err != nil {
		return false, domain.NewError(domain.CategoryInput, 2, "Prompt aborted by user.", "", err)
	}
	return strings.TrimSpace(text) == token, nil
}

func (a *Adapter) SelectOne(message string, choices []string, nonInteractive bool) (string, error) {
	if len(choices) == 0 {
		return "", domain.NewError(domain.CategoryInput, 2, "No choices available.", "", nil)
	}
	if nonInteractive {
		return "", domain.NewError(domain.CategoryInput, 2, "Root repository selection is required in non-interactive mode.", "Provide --root-repo explicitly.", nil)
	}
	fmt.Println(message)
	for i, c := range choices {
		fmt.Printf("  %d) %s\n", i+1, c)
	}
	fmt.Printf("Select one [1-%d]: ", len(choices))
	text, err := a.in.ReadString('\n')
	if err != nil {
		return "", domain.NewError(domain.CategoryInput, 2, "Prompt aborted by user.", "", err)
	}
	i, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || i < 1 || i > len(choices) {
		return "", domain.NewError(domain.CategoryInput, 2, "Invalid selection.", "Pick one index from the listed choices.", err)
	}
	return choices[i-1], nil
}

func (a *Adapter) SelectPackages(message string, choices []string, nonInteractive bool) ([]string, error) {
	if len(choices) == 0 {
		return []string{}, nil
	}
	if nonInteractive {
		return nil, domain.NewError(domain.CategoryInput, 2, "Package selection is required in non-interactive mode.", "Provide one or more --package options.", nil)
	}
	fmt.Println(message)
	for i, c := range choices {
		fmt.Printf("  %d) %s\n", i+1, c)
	}
	fmt.Println("Select package indexes separated by comma (example: 1,2):")
	text, err := a.in.ReadString('\n')
	if err != nil {
		return nil, domain.NewError(domain.CategoryInput, 2, "Prompt aborted by user.", "", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, domain.NewError(domain.CategoryInput, 2, "At least one package must be selected.", "Select one package in the prompt or pass --package explicitly.", nil)
	}
	parts := strings.Split(text, ",")
	seen := map[int]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		i, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || i < 1 || i > len(choices) {
			return nil, domain.NewError(domain.CategoryInput, 2, "Invalid package selection.", "Pick indexes from listed choices.", err)
		}
		if _, ok := seen[i]; ok {
			continue
		}
		seen[i] = struct{}{}
		out = append(out, choices[i-1])
	}
	return out, nil
}

func (a *Adapter) AskText(message, defaultValue string, nonInteractive bool) (string, error) {
	if nonInteractive {
		return defaultValue, nil
	}
	fmt.Printf("%s [%s]: ", message, defaultValue)
	text, err := a.in.ReadString('\n')
	if err != nil {
		return "", domain.NewError(domain.CategoryInput, 2, "Prompt aborted by user.", "", err)
	}
	v := strings.TrimSpace(text)
	if v == "" {
		return defaultValue, nil
	}
	return v, nil
}
