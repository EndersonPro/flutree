package ui

import (
	"os"
	"strconv"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/term"
	"github.com/mattn/go-runewidth"
)

var defaultTerminalWidth = 120

func terminalWidth() int {
	// Respect COLUMNS env var (standard way to override terminal size in CI/non-TTY).
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if w, err := strconv.Atoi(cols); err == nil && w > 0 {
			return w
		}
	}
	width, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil {
		return defaultTerminalWidth
	}
	return width
}

// renderStyledTable builds a styled table using lipgloss/table with adaptive
// colors, status-aware cell rendering, zebra striping, and terminal-width
// awareness.
func renderStyledTable(headers []string, rows [][]string, listRows []domain.ListRow) string {
	if len(headers) == 0 {
		return ""
	}

	targetWidth := terminalWidth()

	// Apply status-based styling to the Status column (index 2)
	styledRows := make([][]string, len(rows))
	for i, row := range rows {
		styledRow := make([]string, len(row))
		copy(styledRow, row)
		if len(styledRow) > 2 && i < len(listRows) {
			styledRow[2] = statusCell(styledRow[2], listRows[i].Status)
		}
		styledRows[i] = styledRow
	}

	// Truncate Path column first when terminal width is insufficient
	styledRows = truncatePathFirst(headers, styledRows, targetWidth)

	t := table.New().
		Headers(headers...).
		Rows(styledRows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return uiTableHeaderStyle
			}
			if row%2 == 0 {
				return uiTableRowStyle
			}
			return uiTableRowAltStyle
		})

	return t.Render()
}

// truncatePathFirst reduces the Path column content when the estimated total
// table width exceeds the target width. Other columns are preserved.
func truncatePathFirst(headers []string, rows [][]string, targetWidth int) [][]string {
	if targetWidth <= 0 {
		return rows
	}

	// Overhead: left border (1) + right border (1) + internal separators (cols-1) + cell padding (cols*2)
	overhead := 2 + (len(headers) - 1) + (len(headers) * 2)

	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = lipgloss.Width(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) {
				if w := lipgloss.Width(cell); w > colWidths[i] {
					colWidths[i] = w
				}
			}
		}
	}

	totalWidth := overhead
	for _, w := range colWidths {
		totalWidth += w
	}

	if totalWidth <= targetWidth {
		return rows
	}

	excess := totalWidth - targetWidth
	pathIdx := 3 // Path is the 4th column

	newRows := make([][]string, len(rows))
	for i, row := range rows {
		newRow := make([]string, len(row))
		copy(newRow, row)
		if pathIdx < len(row) {
			pathWidth := lipgloss.Width(row[pathIdx])
			if pathWidth > excess {
				newWidth := pathWidth - excess
				if newWidth < 1 {
					newWidth = 1
				}
				// Truncate from the left so the basename (end of path) is preserved.
				newRow[pathIdx] = runewidth.TruncateLeft(row[pathIdx], newWidth, "…")
			} else {
				newRow[pathIdx] = "…"
			}
		}
		newRows[i] = newRow
	}

	return newRows
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

func writeTableSeparatorASCII(b *strings.Builder, widths []int) {
	b.WriteString("+")
	for _, width := range widths {
		b.WriteString(strings.Repeat("-", width+2))
		b.WriteString("+")
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
