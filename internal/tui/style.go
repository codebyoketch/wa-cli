package tui

import "github.com/charmbracelet/lipgloss"

var (
	sidebarWidth = 28

	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	chatListHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205"))

	selectedChatStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("205")).
				Bold(true)

	chatItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	unreadStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	myMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86"))

	theirMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	inputBoxFocusedStyle = inputBoxStyle.
				BorderForeground(lipgloss.Color("205"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)
