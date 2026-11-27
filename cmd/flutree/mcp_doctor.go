package main

import (
	"fmt"
	"time"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/EndersonPro/flutree/internal/infra/mcpinstall"
	"github.com/spf13/cobra"
)

type doctorReport struct {
	OK        bool                       `json:"ok"`
	Binary    string                     `json:"binary"`
	Warning   string                     `json:"binary_warning,omitempty"`
	Handshake mcpinstall.HandshakeResult `json:"handshake"`
	Clients   []mcpinstall.ClientCheck   `json:"clients"`
}

// newMCPDoctorCmd diagnoses the binary, each client's config entry and a live
// `mcp serve` handshake. A detected client without a flutree entry is informational; a broken
// command path or a failed handshake is an error (exit 1).
func newMCPDoctorCmd() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the flutree MCP installation (binary, client entries, server handshake)",
		Long: "Report the resolved flutree binary, per client whether it was detected, its config path, " +
			"whether the flutree entry exists and its command is an existing executable, and run a live " +
			"`mcp serve` initialize handshake (5s timeout).\n\n" +
			"A detected client without a flutree entry is informational ([-]); a [WARN] is reserved for real warnings such as a temporary binary path; a broken command path or a failed handshake is an [ERR] and exits 1.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			installer := mcpinstall.DefaultInstaller()
			opts, err := mcpOptions(installer, "", scope, false)
			if err != nil {
				return err
			}
			bin, err := mcpinstall.RealExecResolver()()
			if err != nil {
				return domain.NewError(domain.CategoryUnexpected, 1, err.Error(), "", err)
			}
			checks, err := installer.Diagnose(opts)
			if err != nil {
				return domain.NewError(domain.CategoryUnexpected, 1, err.Error(), "", err)
			}
			rep := doctorReport{
				Binary:    bin,
				Warning:   mcpinstall.BinaryWarning(bin),
				Handshake: mcpinstall.Handshake(bin, 5*time.Second),
				Clients:   checks,
			}
			failed := !rep.Handshake.OK
			for _, c := range checks {
				// A detected client whose config cannot be read, or whose
				// entry points at a missing executable, is a failure.
				if (c.Detected && c.Message != "") || (c.EntryPresent && !c.CommandOK) {
					failed = true
				}
			}
			rep.OK = !failed

			out := cmd.OutOrStdout()
			if jsonOutput(cmd) {
				printJSON(out, rep)
			} else {
				fmt.Fprintf(out, "[OK]   binary: %s\n", bin)
				if rep.Warning != "" {
					fmt.Fprintf(out, "[WARN] binary: %s\n", rep.Warning)
				}
				for _, c := range checks {
					switch {
					case !c.Detected:
						fmt.Fprintf(out, "[-]    %s: not detected\n", c.Client)
					case c.Message != "":
						fmt.Fprintf(out, "[ERR]  %s: %s\n", c.Client, c.Message)
					case !c.EntryPresent:
						fmt.Fprintf(out, "[-]    %s: no flutree entry (%s)\n", c.Client, c.ConfigPath)
					case !c.CommandOK:
						fmt.Fprintf(out, "[ERR]  %s: command %q is not an existing executable (%s)\n", c.Client, c.Command, c.ConfigPath)
					default:
						fmt.Fprintf(out, "[OK]   %s: entry ok (%s)\n", c.Client, c.ConfigPath)
					}
				}
				if rep.Handshake.OK {
					fmt.Fprintf(out, "[OK]   handshake: %s %s\n", rep.Handshake.ServerName, rep.Handshake.ServerVersion)
				} else {
					fmt.Fprintf(out, "[ERR]  handshake: %s\n", rep.Handshake.Error)
				}
			}
			if failed {
				return domain.NewError(domain.CategoryUnexpected, 1, "flutree mcp doctor found problems.", "", nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "user", "Config scope to inspect: user (default) or project")
	return cmd
}
