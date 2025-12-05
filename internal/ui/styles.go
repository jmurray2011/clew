package ui

import "github.com/charmbracelet/lipgloss"

// Color palette - using ANSI 256 colors for broad terminal support
var (
	ColorCyan    = lipgloss.Color("6")
	ColorYellow  = lipgloss.Color("3")
	ColorRed     = lipgloss.Color("1")
	ColorGreen   = lipgloss.Color("2")
	ColorBlue    = lipgloss.Color("4")
	ColorMagenta = lipgloss.Color("5")
	ColorGray    = lipgloss.Color("8")
	ColorWhite   = lipgloss.Color("15")
	ColorBlack   = lipgloss.Color("0")
)

// Text styles
var (
	// Timestamps in log output
	TimestampStyle = lipgloss.NewStyle().Foreground(ColorCyan)

	// Log stream names
	LogStreamStyle = lipgloss.NewStyle().Foreground(ColorYellow)

	// Status messages ("Querying...", "Estimating cost...")
	StatusStyle = lipgloss.NewStyle().Foreground(ColorGray).Italic(true)

	// Error messages
	ErrorStyle = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)

	// Warning messages
	WarningStyle = lipgloss.NewStyle().Foreground(ColorYellow)

	// Success messages
	SuccessStyle = lipgloss.NewStyle().Foreground(ColorGreen)

	// Muted/secondary text
	MutedStyle = lipgloss.NewStyle().Foreground(ColorGray)

	// Highlighted/matched text
	HighlightStyle = lipgloss.NewStyle().
			Background(ColorYellow).
			Foreground(ColorBlack).
			Bold(true)

	// Labels (field names, headers)
	LabelStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)

	// Values (field values)
	ValueStyle = lipgloss.NewStyle().Foreground(ColorWhite)

	// Context lines (before matches)
	ContextStyle = lipgloss.NewStyle().Foreground(ColorGray)

	// Match marker
	MatchMarkerStyle = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
)

// Table styles
var (
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorCyan).
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(ColorGray)

	TableCellStyle = lipgloss.NewStyle().
			PaddingRight(2)

	TableRowAltStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)
)

// Box styles for sections
var (
	SectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorCyan).
				MarginBottom(1)

	InfoBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorGray).
			Padding(0, 1)

	WarningBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorYellow).
			Padding(0, 1)

	ErrorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorRed).
			Padding(0, 1)
)

// Prefix styles for message types
var (
	ErrorPrefix   = ErrorStyle.Render("ERROR:")
	WarningPrefix = WarningStyle.Render("WARN:")
	InfoPrefix    = LabelStyle.Render("INFO:")
	DebugPrefix   = MutedStyle.Render("[DEBUG]")
)
