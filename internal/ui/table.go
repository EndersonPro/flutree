package ui

import (
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

// renderStyledTable builds a styled table with lipgloss-colored headers,
// status-aware cell rendering, and Unicode separator lines.
// Cell delimiters use ASCII | to maintain test compatibility.
func renderStyledTable(headers []string, rows [][]string, listRows []domain.ListRow) string {
	if len(headers) == 0 {
		return ""
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}

	normalizedRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		normalized := make([]string, len(headers))
		for i := range headers {
			if i < len(row) {
				normalized[i] = row[i]
			}
			if len(normalized[i]) > widths[i] {
				widths[i] = len(normalized[i])
			}
		}
		normalizedRows = append(normalizedRows, normalized)
	}

	var b strings.Builder
	writeTableSeparatorUnicode(&b, widths)
	writeTableStyledHeader(&b, headers, widths)
	writeTableSeparatorUnicode(&b, widths)
	for i, row := range normalizedRows {
		if i < len(listRows) {
			writeTableStyledRow(&b, row, widths, listRows[i].Status, i%2 == 0)
		} else {
			writeTableStyledRow(&b, row, widths, "", i%2 == 0)
		}
	}
	writeTableSeparatorUnicode(&b, widths)
	return b.String()
}

// renderTable builds a plain table with ASCII delimiters.
// This is the base table function used by render.go and the wizard.
func renderTable(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}

	normalizedRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		normalized := make([]string, len(headers))
		for i := range headers {
			if i < len(row) {
				normalized[i] = row[i]
			}
			if len(normalized[i]) > widths[i] {
				widths[i] = len(normalized[i])
			}
		}
		normalizedRows = append(normalizedRows, normalized)
	}

	var b strings.Builder
	writeTableSeparatorASCII(&b, widths)
	writeTableRow(&b, headers, widths)
	writeTableSeparatorASCII(&b, widths)
	for _, row := range normalizedRows {
		writeTableRow(&b, row, widths)
	}
	writeTableSeparatorASCII(&b, widths)
	return b.String()
}

func writeTableSeparatorUnicode(b *strings.Builder, widths []int) {
	b.WriteString("├")
	for _, width := range widths {
		b.WriteString(strings.Repeat("─", width+2))
		b.WriteString("┼")
	}
	result := b.String()
	b.Reset()
	b.WriteString(result[:len(result)-1])
	b.WriteString("┤\n")
}

func writeTableSeparatorASCII(b *strings.Builder, widths []int) {
	b.WriteString("+")
	for _, width := range widths {
		b.WriteString(strings.Repeat("-", width+2))
		b.WriteString("+")
	}
	b.WriteString("\n")
}

func writeTableStyledHeader(b *strings.Builder, headers []string, widths []int) {
	b.WriteString("|")
	for i, header := range headers {
		cell := uiTableHeaderStyle.Render(header)
		b.WriteString(" ")
		b.WriteString(cell)
		cellWidth := lipgloss.Width(cell)
		if cellWidth < widths[i] {
			b.WriteString(strings.Repeat(" ", widths[i]-cellWidth))
		}
		if i < len(headers)-1 {
			b.WriteString(" |")
		} else {
			b.WriteString(" |")
		}
	}
	b.WriteString("\n")
}

func writeTableStyledRow(b *strings.Builder, values []string, widths []int, status string, isEven bool) {
	b.WriteString("|")
	for i := range widths {
		value := ""
		if i < len(values) {
			value = values[i]
		}

		// Apply status-based coloring for the Status column
		if i == 2 && status != "" {
			value = statusCell(value, status)
		}

		cellWidth := lipgloss.Width(value)
		b.WriteString(" ")
		b.WriteString(value)
		if cellWidth < widths[i] {
			b.WriteString(strings.Repeat(" ", widths[i]-cellWidth))
		}
		b.WriteString(" |")
	}
	b.WriteString("\n")
}

func statusCell(display, status string) string {
	switch status {
	case "active":
		return lipgloss.NewStyle().Foreground(uiSuccessColor).Render("● " + display)
	case "completed":
		return lipgloss.NewStyle().Foreground(uiMutedColor).Render("○ " + display)
	case "error":
		return lipgloss.NewStyle().Foreground(uiErrorColor).Render("✖ " + display)
	default:
		return display
	}
}

func writeTableRow(b *strings.Builder, values []string, widths []int) {
	b.WriteString("|")
	for i, width := range widths {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		b.WriteString(" ")
		b.WriteString(value)
		if len(value) < width {
			b.WriteString(strings.Repeat(" ", width-len(value)))
		}
		b.WriteString(" |")
	}
	b.WriteString("\n")
}
