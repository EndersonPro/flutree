package ui

import "github.com/charmbracelet/lipgloss"

// ── Color Palette ────────────────────────────────────────────────────────────

var (
	// Core palette
	uiAccentColor  = lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"}
	uiMutedColor   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"}
	uiErrorColor   = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#FCA5A5"}
	uiSuccessColor = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#86EFAC"}
	uiWarningColor = lipgloss.AdaptiveColor{Light: "#A16207", Dark: "#FDE047"}
	uiInfoColor    = lipgloss.AdaptiveColor{Light: "#1D4ED8", Dark: "#93C5FD"}

	// Border and surface colors
	uiBorderColor = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#4B5563"}
	uiCardBgColor = lipgloss.AdaptiveColor{Light: "#F9FAFB", Dark: "#1F2937"}
)

// ── Reusable Style Components ────────────────────────────────────────────────

var (
	// Header styles
	uiHeaderStyle    = lipgloss.NewStyle().Bold(true).Foreground(uiAccentColor).Padding(0, 1)
	uiSubheaderStyle = lipgloss.NewStyle().Bold(true).Foreground(uiMutedColor).Padding(0, 1)
	uiSectionStyle   = lipgloss.NewStyle().Bold(true).Foreground(uiAccentColor)
	uiSuccessHeader  = lipgloss.NewStyle().Bold(true).Foreground(uiSuccessColor).Padding(0, 1)
	uiInfoHeader     = lipgloss.NewStyle().Bold(true).Foreground(uiInfoColor).Padding(0, 1)
	uiWarningHeader  = lipgloss.NewStyle().Bold(true).Foreground(uiWarningColor).Padding(0, 1)

	// Body and text styles
	uiBodyStyle     = lipgloss.NewStyle().PaddingLeft(2)
	uiMutedStyle    = lipgloss.NewStyle().PaddingLeft(2).Foreground(uiMutedColor)
	uiSuccessBody   = lipgloss.NewStyle().PaddingLeft(2).Foreground(uiSuccessColor)
	uiKeyValueStyle = lipgloss.NewStyle().PaddingLeft(2)
	uiKeyValueLabel = lipgloss.NewStyle().Foreground(uiMutedColor)
	uiKeyValueValue = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#111827", Dark: "#F9FAFB"})

	// Table styles
	uiTableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(uiAccentColor)
	uiTableRowStyle    = lipgloss.NewStyle()
	uiTableRowAltStyle = lipgloss.NewStyle()

	// Badge and icon styles
	uiBadgeStyle = lipgloss.NewStyle().Bold(true).Padding(0, 1).MarginRight(1)
	uiIconStyle  = lipgloss.NewStyle().MarginRight(1)
)

// ── Box Helper ────────────────────────────────────────────────────────────────

// Box wraps content in a bordered card with optional style overrides.
func Box(content string, opts ...lipgloss.Style) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(uiBorderColor).
		Padding(1, 2).
		Background(uiCardBgColor)

	for _, opt := range opts {
		style = style.Inherit(opt)
	}

	return style.Render(content)
}

// SuccessBox wraps content in a green-bordered success card.
func SuccessBox(content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(uiSuccessColor).
		Padding(1, 2).
		Background(uiCardBgColor).
		Render(content)
}

// InfoBox wraps content in a blue-bordered info card.
func InfoBox(content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(uiInfoColor).
		Padding(1, 2).
		Background(uiCardBgColor).
		Render(content)
}

// KeyValue formats a key-value pair with consistent styling.
func KeyValue(key, value string) string {
	return uiKeyValueStyle.Render(
		uiKeyValueLabel.Render(key+":") + " " + uiKeyValueValue.Render(value),
	)
}
