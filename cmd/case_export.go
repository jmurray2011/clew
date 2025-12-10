package cmd

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jmurray2011/clew/internal/cases"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var exportOutput string

var caseExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export case to a portable archive",
	Long: `Export the active case to a zip archive.

The archive contains:
  - case.yaml: Full case data
  - evidence/: All collected log entries as JSON
  - report.md: Markdown report
  - report.pdf: PDF report (if typst is installed)

Examples:
  clew case export
  # -> Creates <case-id>.zip

  clew case export -F investigation.zip`,
	RunE: runCaseExport,
}

func initCaseExport() {
	caseCmd.AddCommand(caseExportCmd)
	caseExportCmd.Flags().StringVarP(&exportOutput, "file", "F", "", "Output zip file (default: <case-id>.zip)")
}

func runCaseExport(cmd *cobra.Command, args []string) error {
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

	// Determine output filename
	outputFile := exportOutput
	if outputFile == "" {
		outputFile = c.ID + ".zip"
	}

	// Ensure .zip extension
	if !strings.HasSuffix(outputFile, ".zip") {
		outputFile += ".zip"
	}

	// Create the zip file
	zipFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer func() { _ = zipFile.Close() }()

	zipWriter := zip.NewWriter(zipFile)
	defer func() { _ = zipWriter.Close() }()

	// 1. Add case.yaml
	caseYAML, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal case: %w", err)
	}
	if err := addFileToZip(zipWriter, "case.yaml", caseYAML); err != nil {
		return err
	}

	// 2. Add evidence as JSON files
	for i, e := range c.Evidence {
		evidenceData := map[string]interface{}{
			"ptr":          e.Ptr,
			"message":      e.Message,
			"timestamp":    e.Timestamp,
			"source_uri":   e.SourceURI,
			"source_type":  e.SourceType,
			"stream":       e.Stream,
			"log_group":    e.LogGroup,  // Deprecated but kept for backward compat
			"log_stream":   e.LogStream, // Deprecated but kept for backward compat
			"collected_at": e.CollectedAt,
			"annotation":   e.Annotation,
			"raw_fields":   e.RawFields,
		}

		jsonBytes, err := json.MarshalIndent(evidenceData, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal evidence %d: %w", i+1, err)
		}

		// Use a safe filename based on index and truncated ptr
		safePtr := e.Ptr
		if len(safePtr) > 20 {
			safePtr = safePtr[:20]
		}
		filename := filepath.Join("evidence", fmt.Sprintf("%03d-%s.json", i+1, safePtr))
		if err := addFileToZip(zipWriter, filename, jsonBytes); err != nil {
			return err
		}
	}

	// 3. Generate and add markdown report
	mdReport, err := generateMarkdownReport(c, true)
	if err != nil {
		return fmt.Errorf("failed to generate markdown report: %w", err)
	}
	if err := addFileToZip(zipWriter, "report.md", []byte(mdReport)); err != nil {
		return err
	}

	// 4. Try to generate PDF (optional, don't fail if typst not installed)
	if _, err := exec.LookPath("typst"); err == nil {
		// Generate Typst source
		typstContent := generateTypstReport(c, true)

		// Create temp file for Typst source
		tmpfile, err := os.CreateTemp("", "clew-export-*.typ")
		if err == nil {
			tmpPath := tmpfile.Name()
			_, _ = tmpfile.WriteString(typstContent)
			_ = tmpfile.Close()
			defer func() { _ = os.Remove(tmpPath) }()

			// Create temp file for PDF output
			pdfTmp, err := os.CreateTemp("", "clew-export-*.pdf")
			if err == nil {
				pdfPath := pdfTmp.Name()
				_ = pdfTmp.Close()
				defer func() { _ = os.Remove(pdfPath) }()

				// Run typst compile
				typstCmd := exec.Command("typst", "compile", tmpPath, pdfPath)
				if err := typstCmd.Run(); err == nil {
					// Read and add PDF to zip
					pdfBytes, err := os.ReadFile(pdfPath)
					if err == nil {
						_ = addFileToZip(zipWriter, "report.pdf", pdfBytes)
					}
				}
			}
		}
	}

	app.Render.Success("Exported case to %s", outputFile)
	app.Render.Info("Archive contains:")
	app.Render.Info("  - case.yaml (full case data)")
	app.Render.Info("  - evidence/ (%d log entries)", len(c.Evidence))
	app.Render.Info("  - report.md")
	if _, err := exec.LookPath("typst"); err == nil {
		app.Render.Info("  - report.pdf")
	}

	return nil
}

func addFileToZip(zw *zip.Writer, name string, content []byte) error {
	writer, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("failed to create zip entry %s: %w", name, err)
	}
	if _, err := writer.Write(content); err != nil {
		return fmt.Errorf("failed to write zip entry %s: %w", name, err)
	}
	return nil
}
