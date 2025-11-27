package ui

import (
	"fmt"
	"strings"

	"beads_viewer/pkg/analysis"

	"github.com/charmbracelet/lipgloss"
)

// ActionableModel represents the actionable items view grouped by tracks
type ActionableModel struct {
	plan          analysis.ExecutionPlan
	selectedTrack int
	selectedItem  int
	scrollOffset  int
	width         int
	height        int
	theme         Theme
}

// NewActionableModel creates a new actionable view from execution plan
func NewActionableModel(plan analysis.ExecutionPlan, theme Theme) ActionableModel {
	return ActionableModel{
		plan:          plan,
		selectedTrack: 0,
		selectedItem:  0,
		scrollOffset:  0,
		theme:         theme,
	}
}

// SetSize updates the view dimensions
func (m *ActionableModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// MoveUp moves selection up
func (m *ActionableModel) MoveUp() {
	if len(m.plan.Tracks) == 0 {
		return
	}

	if m.selectedItem > 0 {
		m.selectedItem--
	} else if m.selectedTrack > 0 {
		m.selectedTrack--
		m.selectedItem = len(m.plan.Tracks[m.selectedTrack].Items) - 1
	}
	m.ensureVisible()
}

// MoveDown moves selection down
func (m *ActionableModel) MoveDown() {
	if len(m.plan.Tracks) == 0 {
		return
	}

	track := m.plan.Tracks[m.selectedTrack]
	if m.selectedItem < len(track.Items)-1 {
		m.selectedItem++
	} else if m.selectedTrack < len(m.plan.Tracks)-1 {
		m.selectedTrack++
		m.selectedItem = 0
	}
	m.ensureVisible()
}

// SelectedIssueID returns the ID of the currently selected issue
func (m *ActionableModel) SelectedIssueID() string {
	if len(m.plan.Tracks) == 0 {
		return ""
	}
	if m.selectedTrack >= len(m.plan.Tracks) {
		return ""
	}
	track := m.plan.Tracks[m.selectedTrack]
	if m.selectedItem >= len(track.Items) {
		return ""
	}
	return track.Items[m.selectedItem].ID
}

// ensureVisible adjusts scroll to keep selection visible
func (m *ActionableModel) ensureVisible() {
	// Calculate the line number of the current selection
	lineNum := 0
	for i := 0; i < m.selectedTrack; i++ {
		lineNum += 1 + len(m.plan.Tracks[i].Items) + 1 // header + items + blank
	}
	lineNum += 1 + m.selectedItem // header + item position

	visibleLines := m.height - 4 // account for header and footer
	if visibleLines < 5 {
		visibleLines = 5
	}

	if lineNum < m.scrollOffset {
		m.scrollOffset = lineNum
	} else if lineNum >= m.scrollOffset+visibleLines {
		m.scrollOffset = lineNum - visibleLines + 1
	}
}

// Render renders the actionable view
func (m *ActionableModel) Render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	t := m.theme
	var lines []string

	// Header
	headerStyle := t.Renderer.NewStyle().
		Bold(true).
		Foreground(t.Primary).
		Padding(0, 1)

	totalItems := 0
	for _, track := range m.plan.Tracks {
		totalItems += len(track.Items)
	}

	header := fmt.Sprintf("ACTIONABLE (%d items in %d tracks)", totalItems, len(m.plan.Tracks))
	lines = append(lines, headerStyle.Render(header))
	lines = append(lines, "")

	if len(m.plan.Tracks) == 0 {
		emptyStyle := t.Renderer.NewStyle().
			Foreground(t.Secondary).
			Italic(true).
			Padding(1, 2)
		lines = append(lines, emptyStyle.Render("No actionable items. All tasks are either blocked or completed."))
		return strings.Join(lines, "\n")
	}

	// Summary if there's a high-impact item
	if m.plan.Summary.HighestImpact != "" && m.plan.Summary.UnblocksCount > 0 {
		summaryStyle := t.Renderer.NewStyle().
			Foreground(t.Highlight).
			Padding(0, 1)
		summary := fmt.Sprintf("⚡ Start with %s - %s (%d downstream)",
			m.plan.Summary.HighestImpact,
			m.plan.Summary.ImpactReason,
			m.plan.Summary.UnblocksCount)
		lines = append(lines, summaryStyle.Render(summary))
		lines = append(lines, "")
	}

	// Render tracks
	for trackIdx, track := range m.plan.Tracks {
		// Track header
		trackHeaderStyle := t.Renderer.NewStyle().
			Bold(true).
			Foreground(t.Secondary)
		trackHeader := fmt.Sprintf("Track %s: %s", track.TrackID[6:], track.Reason)
		lines = append(lines, trackHeaderStyle.Render(trackHeader))

		// Track items
		for itemIdx, item := range track.Items {
			isSelected := trackIdx == m.selectedTrack && itemIdx == m.selectedItem

			// Priority icon
			prioIcon := GetPriorityIcon(item.Priority)

			// Build item line
			var itemLine strings.Builder

			// Selection indicator
			if isSelected {
				itemLine.WriteString("▸ ")
			} else {
				itemLine.WriteString("  ")
			}

			// Tree connector
			if itemIdx < len(track.Items)-1 {
				itemLine.WriteString("├─ ")
			} else {
				itemLine.WriteString("└─ ")
			}

			// Priority and ID
			itemLine.WriteString(prioIcon)
			itemLine.WriteString(" ")
			itemLine.WriteString(fmt.Sprintf("P%d ", item.Priority))
			itemLine.WriteString(item.ID)
			itemLine.WriteString(" ")

			// Title (truncated) - use rune-based truncation for UTF-8 safety
			maxTitleLen := m.width - lipgloss.Width(itemLine.String()) - 20
			if maxTitleLen < 10 {
				maxTitleLen = 10
			}
			title := truncateRunesHelper(item.Title, maxTitleLen, "…")
			itemLine.WriteString(title)

			// Unblocks count
			if len(item.UnblocksIDs) > 0 {
				itemLine.WriteString(fmt.Sprintf(" (→%d)", len(item.UnblocksIDs)))
			}

			// Style the line
			lineStyle := t.Renderer.NewStyle()
			if isSelected {
				lineStyle = lineStyle.Background(t.Highlight).Bold(true)
			}

			lines = append(lines, lineStyle.Width(m.width-2).Render(itemLine.String()))

			// Show unblocks detail for selected item
			if isSelected && len(item.UnblocksIDs) > 0 {
				unblocksStyle := t.Renderer.NewStyle().
					Foreground(t.Secondary).
					Italic(true)
				unblocksText := "     └─ Unblocks: " + strings.Join(item.UnblocksIDs, ", ")
				unblocksText = truncateRunesHelper(unblocksText, m.width-4, "...")
				lines = append(lines, unblocksStyle.Render(unblocksText))
			}
		}

		lines = append(lines, "") // Blank line between tracks
	}

	// Apply scroll offset
	visibleLines := m.height - 2
	if visibleLines < 1 {
		visibleLines = 1
	}

	startLine := m.scrollOffset
	if startLine > len(lines)-visibleLines {
		startLine = len(lines) - visibleLines
	}
	if startLine < 0 {
		startLine = 0
	}

	endLine := startLine + visibleLines
	if endLine > len(lines) {
		endLine = len(lines)
	}

	return strings.Join(lines[startLine:endLine], "\n")
}
