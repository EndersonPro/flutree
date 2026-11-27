package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/spf13/cobra"
)

var version = "dev"

var branchNameSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

// cliState records whether Cobra finished parsing and validation and reached a
// command's run phase. Errors returned before that point are usage errors.
type cliState struct {
	started bool
}

func newRootCmd(state *cliState) *cobra.Command {
	root := &cobra.Command{
		Use:   "flutree",
		Short: "Manage Git worktree lifecycle for Flutter-oriented flows.",
		// --version is a regular flag instead of Cobra's built-in one so it shares
		// the version command's argument validation and --json support.
		Args: func(cmd *cobra.Command, args []string) error {
			if showVersion, _ := cmd.Flags().GetBool("version"); showVersion {
				return validateVersionArgs(cmd, args)
			}
			return noUnknownSubcommand(cmd, args)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion, _ := cmd.Flags().GetBool("version"); showVersion {
				return printVersion(cmd)
			}
			return cmd.Help()
		},
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			state.started = true
		},
		SilenceUsage:               true,
		SilenceErrors:              true,
		SuggestionsMinimumDistance: 2,
	}
	root.Flags().Bool("version", false, "Show version")
	root.PersistentFlags().Bool("json", false, "Output results as JSON.")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageError(cmd, err.Error())
	})

	root.AddCommand(
		newCreateCmd(),
		newAddRepoCmd(),
		newConfigCmd(),
		newListCmd(),
		newCompleteCmd(),
		newPubGetCmd(),
		newCleanCmd(),
		newUpdateCmd(),
		newMCPCmd(),
		newVersionCmd(),
	)
	return root
}

// toAppError maps any error returned by Cobra execution to a domain error so
// the runtime boundary can render it and choose the exit code.
func toAppError(err error, state *cliState) error {
	if err == nil {
		return nil
	}
	var appErr *domain.AppError
	if errors.As(err, &appErr) || state.started {
		return err
	}
	// Cobra rejected the invocation before any command ran (bad args, unknown
	// subcommand of a built-in command, ...): that is a usage error.
	return domain.NewError(domain.CategoryInput, 2, err.Error(), "Run 'flutree --help' for usage.", err)
}

func usageError(cmd *cobra.Command, message string) error {
	return domain.NewError(domain.CategoryInput, 2, message, fmt.Sprintf("Run '%s --help' for usage.", cmd.CommandPath()), nil)
}

// noUnknownSubcommand is the Args validator for group commands (root, config,
// mcp). Cobra only reports unknown subcommands on the root by default; this
// extends the same message and suggestions to nested groups.
func noUnknownSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	message := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		message += "\n\nDid you mean this?\n\t" + strings.Join(suggestions, "\n\t")
	}
	return usageError(cmd, message)
}

// requireName validates the single positional argument of commands that act
// on a named worktree or workspace.
func requireName(missingMessage, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
			return domain.NewError(domain.CategoryInput, 2, missingMessage, usage, nil)
		}
		if len(args) > 1 {
			return usageError(cmd, fmt.Sprintf("accepts 1 arg(s), received %d", len(args)))
		}
		return nil
	}
}

func jsonOutput(cmd *cobra.Command) bool {
	enabled, _ := cmd.Flags().GetBool("json")
	return enabled
}

// jsonRequested reports whether --json was passed, even when flag parsing
// failed before Cobra could record it.
func jsonRequested(root *cobra.Command, args []string) bool {
	if flag := root.PersistentFlags().Lookup("json"); flag != nil && flag.Value.String() == "true" {
		return true
	}
	for _, arg := range args {
		if arg == "--json" || arg == "--json=true" {
			return true
		}
	}
	return false
}

func printJSON(w io.Writer, data interface{}) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}

func displayVersion() string {
	v := strings.TrimSpace(version)
	if v == "" {
		return "dev"
	}
	return strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
}

func defaultBranchForName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = branchNameSanitizer.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		normalized = "workspace"
	}
	return "feature/" + normalized
}

func safeVersion(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

// parseAssignments parses repeatable <selector>=<branch> flag values.
func parseAssignments(entries []string, flagName string) (map[string]string, error) {
	out := map[string]string{}
	for _, entry := range entries {
		parts := strings.SplitN(strings.TrimSpace(entry), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, domain.NewError(domain.CategoryInput, 2,
				fmt.Sprintf("Invalid --%s format.", flagName),
				fmt.Sprintf("Use --%s <selector>=<branch>.", flagName), nil)
		}
		out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return out, nil
}
