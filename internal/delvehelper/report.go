// Report generation: orchestrates md → tex → pdf via convert.
package delvehelper

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// dirStamp extracts a YYYY-MM-DDTHH-MM-SS timestamp from a directory name,
// e.g. ".debug_2026-02-27T09-08-40" → "2026-02-27T09-08-40".
func dirStamp(dir string) string {
	base := filepath.Base(dir)
	re := regexp.MustCompile(`(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})`)
	return re.FindString(base)
}

func cmdReportBuild(args []string) error {
	fs := flag.NewFlagSet("report-build", flag.ContinueOnError)
	pkg := fs.String("pkg", "", "package path for report title")
	date := fs.String("date", "", "date for report title (YYYY-MM-DD)")
	verbose := fs.Bool("v", false, "write generated LaTeX to stderr for debugging")
	doPDF := fs.Bool("pdf", false, "compile to PDF with pdflatex after generating .tex")
	outPath := fs.String("out", "", "copy PDF to this path (requires -pdf)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("usage: report-build [-pkg pkg] [-date date] [-pdf] [-out path] <dbgdir>")
	}
	dbgDir := rest[0]

	// If no explicit output path, derive a stamped filename from the dbgDir name.
	if *outPath == "" && *doPDF {
		if stamp := dirStamp(dbgDir); stamp != "" {
			*outPath = "./debug_report_" + stamp + ".pdf"
		} else if *date != "" {
			*outPath = "./debug_report_" + *date + ".pdf"
		}
	}

	tex, mdCount, err := MDToTex(dbgDir, *pkg, *date)
	if err != nil {
		return err
	}

	reportPath := filepath.Join(dbgDir, "debug_report.tex")
	if err := os.WriteFile(reportPath, []byte(tex), 0644); err != nil {
		return fmt.Errorf("write %s: %w", reportPath, err)
	}
	if *verbose {
		fmt.Fprintln(os.Stderr, "--- generated LaTeX (first 2000 chars) ---")
		preview := tex
		if len(preview) > 2000 {
			preview = preview[:2000] + "\n... (truncated)"
		}
		fmt.Fprintln(os.Stderr, preview)
		fmt.Fprintln(os.Stderr, "--- end ---")
	}
	fmt.Printf("wrote %s from %d markdown fragments\n", reportPath, mdCount)

	if *doPDF {
		if err := TexToPDF(dbgDir); err != nil {
			return err
		}
		if *outPath != "" {
			if err := CopyPDF(dbgDir, *outPath); err != nil {
				return err
			}
		}
	}
	return nil
}
