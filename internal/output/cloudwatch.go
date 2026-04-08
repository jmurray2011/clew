package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/ui"
	"github.com/jmurray2011/clew/pkg/timeutil"
)

// FormatLogGroups outputs CloudWatch log group information in the configured format.
func (f *Formatter) FormatLogGroups(groups []cloudwatch.LogGroupInfo) error {
	switch f.format {
	case FormatJSON:
		return f.formatGroupsJSON(groups)
	case FormatCSV:
		return f.formatGroupsCSV(groups)
	default:
		return f.formatGroupsText(groups)
	}
}

func (f *Formatter) formatGroupsText(groups []cloudwatch.LogGroupInfo) error {
	if len(groups) == 0 {
		_, _ = fmt.Fprintln(f.writer, ui.MutedStyle.Render("No log groups found."))
		return nil
	}

	for _, g := range groups {
		_, _ = fmt.Fprintln(f.writer, ui.SuccessStyle.Render(g.Name))

		_, _ = fmt.Fprint(f.writer, ui.MutedStyle.Render("  Size: "))
		_, _ = fmt.Fprint(f.writer, timeutil.FormatBytes(g.StoredBytes))

		if g.RetentionDays > 0 {
			_, _ = fmt.Fprintf(f.writer, "  |  Retention: %d days", g.RetentionDays)
		} else {
			_, _ = fmt.Fprint(f.writer, "  |  Retention: Never expire")
		}

		if !g.CreationTime.IsZero() {
			_, _ = fmt.Fprintf(f.writer, "  |  Created: %s", g.CreationTime.Format("2006-01-02"))
		}

		_, _ = fmt.Fprintln(f.writer)
	}

	return nil
}

func (f *Formatter) formatGroupsJSON(groups []cloudwatch.LogGroupInfo) error {
	type jsonGroup struct {
		Name          string `json:"name"`
		StoredBytes   int64  `json:"storedBytes"`
		RetentionDays int    `json:"retentionDays,omitempty"`
		CreationTime  string `json:"creationTime,omitempty"`
	}

	jsonGroups := make([]jsonGroup, len(groups))
	for i, g := range groups {
		jsonGroups[i] = jsonGroup{
			Name:          g.Name,
			StoredBytes:   g.StoredBytes,
			RetentionDays: g.RetentionDays,
		}
		if !g.CreationTime.IsZero() {
			jsonGroups[i].CreationTime = g.CreationTime.Format("2006-01-02T15:04:05Z")
		}
	}

	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonGroups)
}

func (f *Formatter) formatGroupsCSV(groups []cloudwatch.LogGroupInfo) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	if err := writer.Write([]string{"name", "storedBytes", "retentionDays", "creationTime"}); err != nil {
		return err
	}

	for _, g := range groups {
		creationTime := ""
		if !g.CreationTime.IsZero() {
			creationTime = g.CreationTime.Format("2006-01-02T15:04:05Z")
		}
		record := []string{g.Name, fmt.Sprintf("%d", g.StoredBytes), fmt.Sprintf("%d", g.RetentionDays), creationTime}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}
