package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/jmurray2011/clew/internal/cases"
	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/ui"
	"github.com/jmurray2011/clew/pkg/timeutil"

	"github.com/spf13/cobra"
)

var (
	caseCustomID string
	caseForce    bool
	caseStatus   string
)

var caseCmd = &cobra.Command{
	Use:   "case",
	Short: "Manage investigation cases",
	Long: `Manage investigation cases for tracking log investigations.

Cases help you organize your investigation workflow by:
- Tracking queries you've run
- Saving important notes
- Collecting evidence (specific log entries)
- Generating reports

Examples:
  # Create a new case
  clew case new "API outage 2024-01-15"

  # List all cases
  clew case list

  # Show current case status
  clew case status

  # Switch to an existing case
  clew case open api-outage-2024-01-15

  # Close the current case
  clew case close`,
}

var caseNewCmd = &cobra.Command{
	Use:   "new <title>",
	Short: "Create a new investigation case",
	Long: `Create a new investigation case.

A slug is automatically generated from the title (e.g., "API outage 2024-01-15"
becomes "api-outage-2024-01-15"). Use --id to override the generated slug.

The new case is automatically set as the active case.

Examples:
  # Create a new case
  clew case new "API outage investigation"

  # Create with a custom ID
  clew case new "API outage" --id api-outage-prod`,
	Args: cobra.ExactArgs(1),
	RunE: runCaseNew,
}

var caseOpenCmd = &cobra.Command{
	Use:   "open <id>",
	Short: "Switch to an existing case",
	Long: `Switch to an existing case, making it the active case.

Use 'clew case list' to see available cases.

Examples:
  clew case open api-outage-2024-01-15`,
	Args:              cobra.ExactArgs(1),
	RunE:              runCaseOpen,
	ValidArgsFunction: completeCaseIDs,
}

var caseCloseCmd = &cobra.Command{
	Use:   "close",
	Short: "Close the active case",
	Long: `Close the currently active case.

Sets the case status to 'closed'. You will be prompted for a summary
if one hasn't been set yet.

Examples:
  clew case close`,
	RunE: runCaseClose,
}

var caseListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all cases",
	Long: `List all investigation cases.

Examples:
  # List all cases
  clew case list

  # List only active cases
  clew case list --status active

  # List only closed cases
  clew case list --status closed`,
	RunE: runCaseList,
}

var caseStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show active case summary",
	Long: `Show a summary of the currently active case.

Displays title, creation time, query count, evidence count, and notes count.

Examples:
  clew case status`,
	RunE: runCaseStatus,
}

var caseDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a case",
	Long: `Delete an investigation case.

Requires confirmation unless --force is used.

Examples:
  clew case delete api-outage-2024-01-15
  clew case delete api-outage-2024-01-15 --force`,
	Args:              cobra.ExactArgs(1),
	RunE:              runCaseDelete,
	ValidArgsFunction: completeCaseIDs,
}

var caseMarkCmd = &cobra.Command{
	Use:   "mark",
	Short: "Mark the last query as significant",
	Long: `Mark the most recent query in the timeline as significant evidence.

Marked queries are highlighted in the timeline and included in reports.

Examples:
  # Run a query, then mark it as significant
  clew query -g api -s 1h -f "error"
  clew case mark`,
	RunE: runCaseMark,
}

var caseTimelineCmd = &cobra.Command{
	Use:   "timeline",
	Short: "Show investigation timeline",
	Long: `Show the chronological timeline of the active investigation.

Displays queries, notes, and evidence collection events.

Examples:
  # Show full timeline
  clew case timeline

  # Show only marked (significant) entries
  clew case timeline --marked

  # Show only queries
  clew case timeline --type query

  # Show only notes
  clew case timeline --type note`,
	RunE: runCaseTimeline,
}

var (
	timelineMarkedOnly bool
	timelineType       string
	noteFile           string
	noteEditor         bool
	summaryFile        string
	summaryEditor      bool
	keepAnnotation     string
)

var caseNoteCmd = &cobra.Command{
	Use:   "note [text]",
	Short: "Add a note to the active case",
	Long: `Add a note to the active case timeline.

Notes help document your findings, hypotheses, and observations during an investigation.

Examples:
  # Add inline note
  clew case note "Correlates with deploy at 03:13 UTC"

  # Add note from file
  clew case note -f findings.md

  # Open editor to write note
  clew case note -e

  # Read from stdin
  echo "Quick note" | clew case note`,
	RunE: runCaseNote,
}

var caseSummaryCmd = &cobra.Command{
	Use:   "summary [text]",
	Short: "Set or update the case summary",
	Long: `Set or update the summary for the active case.

The summary is shown in case status and included in reports.

Examples:
  # Set summary inline
  clew case summary "Root cause: connection pool exhaustion after deploy"

  # Set from file
  clew case summary -f summary.md

  # Open editor
  clew case summary -e`,
	RunE: runCaseSummary,
}

var caseKeepCmd = &cobra.Command{
	Use:   "keep <@ptr>",
	Short: "Save a log entry as evidence",
	Long: `Save a specific log entry as evidence in the active case.

The @ptr (log pointer) is obtained from query results. This fetches the full
log record and stores it in the case.

Examples:
  # Keep a log entry
  clew case keep CpMBCmQK...

  # Keep with annotation
  clew case keep CpMBCmQK... -a "First error after deploy"`,
	Args: cobra.ExactArgs(1),
	RunE: runCaseKeep,
}

var caseAnnotateCmd = &cobra.Command{
	Use:   "annotate <@ptr> <text>",
	Short: "Annotate evidence",
	Long: `Add or update annotation on existing evidence.

Examples:
  clew case annotate CpMBCmQK... "This correlates with the deploy"`,
	Args: cobra.ExactArgs(2),
	RunE: runCaseAnnotate,
}

var caseEvidenceCmd = &cobra.Command{
	Use:   "evidence",
	Short: "List collected evidence",
	Long: `List all evidence collected in the active case.

Examples:
  clew case evidence`,
	RunE: runCaseEvidence,
}

var caseReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate investigation report",
	Long: `Generate a report of the investigation.

Outputs markdown by default. Use -o to write to a file (format auto-detected
from extension) or --format to specify format explicitly.

PDF output requires Typst to be installed (https://typst.app).

Examples:
  # Print markdown report to stdout
  clew case report

  # Save to file (format from extension)
  clew case report -o report.md
  clew case report -o report.json
  clew case report -o report.pdf

  # Include all queries (not just marked)
  clew case report --full`,
	RunE: runCaseReport,
}

var (
	reportOutput string
	reportFormat string
	reportFull   bool
)

func init() {
	rootCmd.AddCommand(caseCmd)

	caseCmd.AddCommand(caseNewCmd)
	caseCmd.AddCommand(caseOpenCmd)
	caseCmd.AddCommand(caseCloseCmd)
	caseCmd.AddCommand(caseListCmd)
	caseCmd.AddCommand(caseStatusCmd)
	caseCmd.AddCommand(caseDeleteCmd)
	caseCmd.AddCommand(caseMarkCmd)
	caseCmd.AddCommand(caseTimelineCmd)
	caseCmd.AddCommand(caseNoteCmd)
	caseCmd.AddCommand(caseSummaryCmd)
	caseCmd.AddCommand(caseKeepCmd)
	caseCmd.AddCommand(caseAnnotateCmd)
	caseCmd.AddCommand(caseEvidenceCmd)

	caseNewCmd.Flags().StringVar(&caseCustomID, "id", "", "Custom case ID (slug)")
	caseListCmd.Flags().StringVar(&caseStatus, "status", "all", "Filter by status: active, closed, all")
	caseDeleteCmd.Flags().BoolVarP(&caseForce, "force", "f", false, "Skip confirmation")
	caseTimelineCmd.Flags().BoolVar(&timelineMarkedOnly, "marked", false, "Show only marked (significant) entries")
	caseTimelineCmd.Flags().StringVar(&timelineType, "type", "", "Filter by type: query, note, evidence")

	caseNoteCmd.Flags().StringVarP(&noteFile, "file", "f", "", "Read note from file")
	caseNoteCmd.Flags().BoolVarP(&noteEditor, "editor", "e", false, "Open editor to write note")

	caseSummaryCmd.Flags().StringVarP(&summaryFile, "file", "f", "", "Read summary from file")
	caseSummaryCmd.Flags().BoolVarP(&summaryEditor, "editor", "e", false, "Open editor to write summary")

	caseKeepCmd.Flags().StringVarP(&keepAnnotation, "annotation", "a", "", "Add annotation to evidence")

	caseCmd.AddCommand(caseReportCmd)
	caseReportCmd.Flags().StringVarP(&reportOutput, "output", "o", "", "Output file (format from extension)")
	caseReportCmd.Flags().StringVar(&reportFormat, "format", "md", "Output format: md, json, pdf")
	caseReportCmd.Flags().BoolVar(&reportFull, "full", false, "Include all queries (not just marked)")

	// Export subcommand is defined in case_export.go
	initCaseExport()
}

func runCaseNew(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	title := args[0]

	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	c, err := mgr.CreateCase(ctx, title, caseCustomID)
	if err != nil {
		return err
	}

	app.Render.Success("Created case: %s", c.ID)
	app.Render.Info("Title: %s", c.Title)
	app.Render.Info("Set as active case")

	return nil
}

func runCaseOpen(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	id := args[0]

	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	c, err := mgr.OpenCase(ctx, id)
	if err != nil {
		return err
	}

	app.Render.Success("Switched to case: %s", c.ID)
	app.Render.Info("Title: %s", c.Title)
	app.Render.Info("Status: %s", c.Status)

	return nil
}

func runCaseClose(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	c, err := mgr.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no active case")
	}

	// Prompt for summary if not set
	summary := c.Summary
	if summary == "" {
		app.Render.Info("Case: %s", c.Title)
		fmt.Print("Enter summary (optional, press Enter to skip): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		summary = strings.TrimSpace(input)
	}

	if err := mgr.CloseCase(ctx, summary); err != nil {
		return err
	}

	app.Render.Success("Closed case: %s", c.ID)

	return nil
}

func runCaseList(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	allCases, err := mgr.ListCases(ctx)
	if err != nil {
		return err
	}

	// Get active case ID
	state, _ := mgr.LoadState(ctx)
	activeID := ""
	if state != nil {
		activeID = state.ActiveCase
	}

	// Filter by status
	var filtered []*cases.Case
	for _, c := range allCases {
		switch caseStatus {
		case "active":
			if c.Status == cases.StatusActive {
				filtered = append(filtered, c)
			}
		case "closed":
			if c.Status == cases.StatusClosed {
				filtered = append(filtered, c)
			}
		default:
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == 0 {
		app.Render.Info("No cases found.")
		return nil
	}

	// Sort by updated time (most recent first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Updated.After(filtered[j].Updated)
	})

	// Display cases
	for _, c := range filtered {
		// Show active indicator
		activeMarker := "  "
		if c.ID == activeID {
			activeMarker = ui.SuccessStyle.Render("* ")
		}

		// Status color
		statusStr := string(c.Status)
		if c.Status == cases.StatusActive {
			statusStr = ui.SuccessStyle.Render(statusStr)
		} else {
			statusStr = ui.MutedStyle.Render(statusStr)
		}

		// Count timeline entries and evidence
		queryCount := 0
		noteCount := 0
		for _, t := range c.Timeline {
			switch t.Type {
			case "query":
				queryCount++
			case "note":
				noteCount++
			}
		}
		evidenceCount := len(c.Evidence)

		fmt.Printf("%s%s  [%s]\n", activeMarker, ui.LabelStyle.Render(c.ID), statusStr)
		fmt.Printf("    %s\n", c.Title)
		fmt.Printf("    %s  queries: %d  notes: %d  evidence: %d\n",
			ui.MutedStyle.Render(c.Updated.Format("2006-01-02 15:04")),
			queryCount, noteCount, evidenceCount)
		fmt.Println()
	}

	return nil
}

func runCaseStatus(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	c, err := mgr.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		app.Render.Info("No active case. Use 'clew case new' or 'clew case open' to start.")
		return nil
	}

	// Count timeline entries
	queryCount := 0
	noteCount := 0
	markedCount := 0
	for _, t := range c.Timeline {
		switch t.Type {
		case "query":
			queryCount++
			if t.Marked {
				markedCount++
			}
		case "note":
			noteCount++
		}
	}

	// Calculate time spent
	var firstAction, lastAction time.Time
	if len(c.Timeline) > 0 {
		firstAction = c.Timeline[0].Timestamp
		lastAction = c.Timeline[len(c.Timeline)-1].Timestamp
	}

	app.Render.Section("Active Case")
	fmt.Println()
	fmt.Printf("  %s  %s\n", ui.LabelStyle.Render("ID:"), c.ID)
	fmt.Printf("  %s  %s\n", ui.LabelStyle.Render("Title:"), c.Title)
	fmt.Printf("  %s  %s\n", ui.LabelStyle.Render("Status:"), c.Status)
	fmt.Printf("  %s  %s\n", ui.LabelStyle.Render("Created:"), c.Created.Format("2006-01-02 15:04:05"))
	fmt.Printf("  %s  %s\n", ui.LabelStyle.Render("Updated:"), c.Updated.Format("2006-01-02 15:04:05"))

	if c.Summary != "" {
		fmt.Println()
		fmt.Printf("  %s\n", ui.LabelStyle.Render("Summary:"))
		fmt.Printf("  %s\n", c.Summary)
	}

	fmt.Println()
	app.Render.Divider()
	app.Render.Section("Statistics")
	fmt.Println()
	fmt.Printf("  Queries:    %d (%d marked)\n", queryCount, markedCount)
	fmt.Printf("  Notes:      %d\n", noteCount)
	fmt.Printf("  Evidence:   %d\n", len(c.Evidence))

	if !firstAction.IsZero() && !lastAction.IsZero() {
		duration := lastAction.Sub(firstAction)
		fmt.Printf("  Time span:  %s\n", timeutil.FormatDuration(duration))
	}

	return nil
}

func runCaseDelete(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	id := args[0]

	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	// Check if case exists
	c, err := mgr.LoadCase(ctx, id)
	if err != nil {
		return err
	}

	// Confirm deletion
	if !caseForce {
		fmt.Printf("Delete case %q (%s)? [y/N] ", c.ID, c.Title)
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			app.Render.Info("Cancelled")
			return nil
		}
	}

	if err := mgr.DeleteCase(ctx, id); err != nil {
		return err
	}

	app.Render.Success("Deleted case: %s", id)

	return nil
}

// completeCaseIDs provides tab completion for case IDs.
func completeCaseIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	mgr, err := cases.NewManager()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ids := mgr.GetCaseIDs(cmd.Context())
	var matches []string
	for _, id := range ids {
		if strings.HasPrefix(id, toComplete) {
			matches = append(matches, id)
		}
	}

	return matches, cobra.ShellCompDirectiveNoFileComp
}

func runCaseMark(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	if err := mgr.MarkLastQuery(ctx); err != nil {
		return err
	}

	app.Render.Success("Marked last query as significant")
	return nil
}

func runCaseTimeline(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	c, err := mgr.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no active case")
	}

	entries, err := mgr.GetTimeline(ctx, timelineType, timelineMarkedOnly)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		app.Render.Info("No timeline entries found.")
		return nil
	}

	app.Render.Section(fmt.Sprintf("Timeline: %s", c.Title))
	fmt.Println()

	for _, e := range entries {
		// Timestamp
		ts := ui.MutedStyle.Render(e.Timestamp.Format("2006-01-02 15:04:05"))

		// Type indicator
		var typeStr string
		switch e.Type {
		case "query":
			if e.Marked {
				typeStr = ui.SuccessStyle.Render("[*] QUERY")
			} else {
				typeStr = ui.LabelStyle.Render("QUERY")
			}
		case "note":
			typeStr = ui.TimestampStyle.Render("NOTE")
		case "evidence":
			typeStr = ui.WarningStyle.Render("EVIDENCE")
		default:
			typeStr = e.Type
		}

		fmt.Printf("  %s  %s\n", ts, typeStr)

		// Content based on type
		switch e.Type {
		case "query":
			if e.Command != "" {
				fmt.Printf("    %s\n", e.Command)
			}
			// Prefer SourceURI, fall back to deprecated LogGroup
			sourceDisplay := e.SourceURI
			if sourceDisplay == "" {
				sourceDisplay = e.LogGroup
			}
			if sourceDisplay != "" {
				fmt.Printf("    Source: %s\n", ui.MutedStyle.Render(sourceDisplay))
			}
			if e.Filter != "" {
				fmt.Printf("    Filter: %s\n", ui.MutedStyle.Render(e.Filter))
			}
			if e.Query != "" {
				fmt.Printf("    Query: %s\n", ui.MutedStyle.Render(e.Query))
			}
			fmt.Printf("    Results: %d\n", e.Results)
		case "note":
			fmt.Printf("    %s\n", e.Content)
			if e.Source != "" {
				fmt.Printf("    Source: %s\n", ui.MutedStyle.Render(e.Source))
			}
		case "evidence":
			if e.Content != "" {
				// Truncate long content
				content := e.Content
				if len(content) > 100 {
					content = content[:97] + "..."
				}
				fmt.Printf("    %s\n", content)
			}
			if e.Source != "" {
				fmt.Printf("    Source: %s\n", ui.MutedStyle.Render(e.Source))
			}
		}
		fmt.Println()
	}

	return nil
}

func runCaseNote(cmd *cobra.Command, args []string) error {
	var content string
	var source string

	switch {
	case noteFile != "":
		// Read from file
		data, err := os.ReadFile(noteFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		content = strings.TrimSpace(string(data))
		source = "file:" + noteFile

	case noteEditor:
		// Open editor
		text, err := openEditor("")
		if err != nil {
			return err
		}
		content = strings.TrimSpace(text)
		source = "editor"

	case len(args) > 0:
		// Inline note from args
		content = strings.Join(args, " ")
		source = "inline"

	default:
		// Read from stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Data is being piped in
			data, err := os.ReadFile("/dev/stdin")
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			content = strings.TrimSpace(string(data))
			source = "stdin"
		} else {
			return fmt.Errorf("no note content provided. Use: clew case note <text>, -f <file>, -e, or pipe from stdin")
		}
	}

	if content == "" {
		return fmt.Errorf("note content is empty")
	}

	ctx := cmd.Context()
	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	if err := mgr.AddNoteToTimeline(ctx, content, source); err != nil {
		return err
	}

	app := GetApp(cmd)
	app.Render.Success("Added note to case")
	return nil
}

func runCaseSummary(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	var summary string

	switch {
	case summaryFile != "":
		// Read from file
		data, err := os.ReadFile(summaryFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		summary = strings.TrimSpace(string(data))

	case summaryEditor:
		// Open editor with current summary
		mgr, err := cases.NewManager()
		if err != nil {
			return err
		}
		c, err := mgr.GetActiveCase(ctx)
		if err != nil {
			return err
		}
		if c == nil {
			return fmt.Errorf("no active case")
		}

		text, err := openEditor(c.Summary)
		if err != nil {
			return err
		}
		summary = strings.TrimSpace(text)

	case len(args) > 0:
		// Inline summary from args
		summary = strings.Join(args, " ")

	default:
		// Read from stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := os.ReadFile("/dev/stdin")
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			summary = strings.TrimSpace(string(data))
		} else {
			return fmt.Errorf("no summary content provided. Use: clew case summary <text>, -f <file>, -e, or pipe from stdin")
		}
	}

	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	if err := mgr.SetSummary(ctx, summary); err != nil {
		return err
	}

	app := GetApp(cmd)
	app.Render.Success("Updated case summary")
	return nil
}

// openEditor opens an editor with the given initial content and returns the edited text.
func openEditor(initial string) (string, error) {
	// Determine editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi" // Default fallback
	}

	// Create temp file
	tmpfile, err := os.CreateTemp("", "clew-*.md")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	// Write initial content
	if initial != "" {
		if _, err := tmpfile.WriteString(initial); err != nil {
			return "", fmt.Errorf("failed to write temp file: %w", err)
		}
	}
	_ = tmpfile.Close()

	// Open editor
	cmd := exec.Command(editor, tmpfile.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor failed: %w", err)
	}

	// Read result
	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to read edited file: %w", err)
	}

	return string(data), nil
}

func runCaseKeep(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	ptrInput := args[0]

	// Resolve short prefix to full @ptr with metadata (Docker-style)
	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	ptr, ptrMeta, err := mgr.ResolvePtrWithMetadata(ctx, ptrInput)
	if err != nil {
		return err
	}

	// Show the resolved pointer if it was expanded
	if ptr != ptrInput {
		app.Render.Info("Resolved to: %s...", ptr[:min(40, len(ptr))])
	}

	// Use cached profile for cross-account support, fall back to current profile
	awsProfile := app.GetProfile()
	if ptrMeta != nil && ptrMeta.Profile != "" {
		awsProfile = ptrMeta.Profile
		app.Debugf("Using cached profile: %s", awsProfile)
	}

	// Create AWS client
	rawClient, err := cloudwatch.NewLogsClient(awsProfile, app.GetRegion())
	if err != nil {
		return fmt.Errorf("failed to create AWS client: %w", err)
	}

	logsClient := cloudwatch.NewClient(rawClient)

	// Fetch the log record
	record, err := logsClient.GetLogRecord(ctx, ptr)
	if err != nil {
		return fmt.Errorf("failed to fetch log record: %w", err)
	}

	// Extract log group from fields if available
	logGroup := record.Fields["@logGroup"]
	if logGroup == "" {
		// Try to get from @log field (format: "accountId:logGroupName")
		if logField := record.Fields["@log"]; logField != "" {
			if parts := strings.SplitN(logField, ":", 2); len(parts) == 2 {
				logGroup = parts[1]
			}
		}
	}
	// Fall back to cached metadata from query (enables cross-account support)
	// Prefer SourceURI, fall back to deprecated LogGroup
	if logGroup == "" && ptrMeta != nil {
		if ptrMeta.SourceURI != "" {
			logGroup = ptrMeta.SourceURI
		} else if ptrMeta.LogGroup != "" {
			logGroup = ptrMeta.LogGroup
		}
	}
	if logGroup == "" {
		logGroup = "unknown"
	}

	// Parse timestamp
	var ts time.Time
	if record.Timestamp != "" {
		ts, _ = time.Parse(time.RFC3339Nano, record.Timestamp)
	}

	// Determine profile and account ID (prefer cached metadata for cross-account accuracy)
	profile := app.GetProfile()
	accountID := app.GetAccountID()
	if ptrMeta != nil {
		if ptrMeta.Profile != "" {
			profile = ptrMeta.Profile
		}
		if ptrMeta.AccountID != "" {
			accountID = ptrMeta.AccountID
		}
	}

	// Determine source URI (prefer cached metadata)
	sourceURI := ""
	sourceType := "cloudwatch"
	if ptrMeta != nil && ptrMeta.SourceURI != "" {
		sourceURI = ptrMeta.SourceURI
		if ptrMeta.SourceType != "" {
			sourceType = ptrMeta.SourceType
		}
	} else if logGroup != "" && logGroup != "unknown" {
		sourceURI = "cloudwatch:///" + logGroup
	}

	// Create evidence item with full log record data
	item := cases.EvidenceItem{
		Ptr:        ptr,
		Message:    record.Message,
		Timestamp:  ts,
		SourceURI:  sourceURI,
		SourceType: sourceType,
		Stream:     record.LogStream,
		Profile:    profile,
		AccountID:  accountID,
		Annotation: keepAnnotation,
		RawFields:  record.Fields,
	}

	// Add to case (reuse mgr from earlier)
	if err := mgr.AddEvidence(ctx, item); err != nil {
		return err
	}

	app.Render.Success("Added evidence to case")
	if keepAnnotation != "" {
		app.Render.Info("Annotation: %s", keepAnnotation)
	}

	return nil
}

func runCaseAnnotate(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	ptr := args[0]
	annotation := args[1]

	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	if err := mgr.AnnotateEvidence(ctx, ptr, annotation); err != nil {
		return err
	}

	app.Render.Success("Updated evidence annotation")
	return nil
}

func runCaseEvidence(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	evidence, err := mgr.GetEvidence(ctx)
	if err != nil {
		return err
	}

	if len(evidence) == 0 {
		app.Render.Info("No evidence collected.")
		return nil
	}

	c, _ := mgr.GetActiveCase(ctx)
	app.Render.Section(fmt.Sprintf("Evidence: %s", c.Title))
	fmt.Println()

	for i, e := range evidence {
		fmt.Printf("  %s %d\n", ui.LabelStyle.Render("Evidence"), i+1)

		// Timestamp
		if !e.Timestamp.IsZero() {
			fmt.Printf("    Timestamp:  %s\n", e.Timestamp.Format("2006-01-02 15:04:05"))
		}

		// Source and stream (prefer new fields, fall back to deprecated)
		sourceDisplay := e.SourceURI
		if sourceDisplay == "" {
			sourceDisplay = e.LogGroup
		}
		fmt.Printf("    Source:     %s\n", sourceDisplay)
		streamDisplay := e.Stream
		if streamDisplay == "" {
			streamDisplay = e.LogStream
		}
		if streamDisplay != "" {
			fmt.Printf("    Stream:     %s\n", streamDisplay)
		}

		// Message (truncated if long)
		msg := e.Message
		if len(msg) > 200 {
			msg = msg[:197] + "..."
		}
		fmt.Printf("    Message:    %s\n", msg)

		// Annotation
		if e.Annotation != "" {
			fmt.Printf("    %s  %s\n", ui.SuccessStyle.Render("Annotation:"), e.Annotation)
		}

		// @ptr (truncated for display)
		ptrDisplay := e.Ptr
		if len(ptrDisplay) > 40 {
			ptrDisplay = ptrDisplay[:37] + "..."
		}
		fmt.Printf("    @ptr:       %s\n", ui.MutedStyle.Render(ptrDisplay))

		fmt.Println()
	}

	return nil
}

func runCaseReport(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()
	mgr, err := cases.NewManager()
	if err != nil {
		return err
	}

	c, err := mgr.GetActiveCase(ctx)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("no active case")
	}

	// Determine output format
	format := reportFormat
	if reportOutput != "" {
		if strings.HasSuffix(reportOutput, ".json") {
			format = "json"
		} else if strings.HasSuffix(reportOutput, ".md") || strings.HasSuffix(reportOutput, ".markdown") {
			format = "md"
		} else if strings.HasSuffix(reportOutput, ".pdf") {
			format = "pdf"
		}
	}

	// Generate report
	var report string
	switch format {
	case "json":
		report, err = generateJSONReport(c, reportFull)
	case "pdf":
		pdfOutput := reportOutput
		if pdfOutput == "" {
			pdfOutput = c.ID + ".pdf"
		}
		if err := generatePDFReport(c, reportFull, reportOutput); err != nil {
			return err
		}
		app.Render.Success("Report saved to %s", pdfOutput)
		return nil
	default:
		report, err = generateMarkdownReport(c, reportFull)
	}
	if err != nil {
		return err
	}

	// Output
	if reportOutput != "" {
		if err := os.WriteFile(reportOutput, []byte(report), 0644); err != nil {
			return fmt.Errorf("failed to write report: %w", err)
		}
		app.Render.Success("Report saved to %s", reportOutput)
	} else {
		fmt.Print(report)
	}

	return nil
}
