package ui

import "strings"

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
	writeTableSeparator(&b, widths)
	writeTableRow(&b, headers, widths)
	writeTableSeparator(&b, widths)
	for _, row := range normalizedRows {
		writeTableRow(&b, row, widths)
	}
	writeTableSeparator(&b, widths)
	return b.String()
}

func writeTableSeparator(b *strings.Builder, widths []int) {
	b.WriteString("+")
	for _, width := range widths {
		b.WriteString(strings.Repeat("-", width+2))
		b.WriteString("+")
	}
	b.WriteString("\n")
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
