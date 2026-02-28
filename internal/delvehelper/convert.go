// Conversion: .md → .tex → .pdf with styled boxes (rootcausebox, fixbox).
package delvehelper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// fixMarkdownTables inserts missing header-separator rows so Pandoc recognizes tables.
// Markdown tables require: header row, separator row (| --- | --- |), then data rows.
// A separator is only injected after the FIRST row of a new table; rows inside an
// existing table body are left untouched.
func fixMarkdownTables(md string) string {
	lines := strings.Split(md, "\n")
	var out []string
	inTable := false // true while consecutive | rows are being processed
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		out = append(out, line)
		isTableRow := strings.HasPrefix(line, "|") && strings.Contains(line[1:], "|")
		isSeparator := isTableRow && strings.Contains(line, "---")
		if isTableRow && !inTable && i+1 < len(lines) {
			// First row of a potential new table — insert separator only if missing.
			next := lines[i+1]
			if strings.HasPrefix(next, "|") && !strings.Contains(next, "---") {
				nc := strings.Count(line, "|") - 1
				if nc < 1 {
					nc = 1
				}
				sep := "|"
				for j := 0; j < nc; j++ {
					sep += " --- |"
				}
				out = append(out, sep)
			}
		}
		inTable = isTableRow || isSeparator
	}
	return strings.Join(out, "\n")
}

// fixLongtable simplifies Pandoc's longtable output which uses complex column
// specs and minipage headers that can break layout. Replaces with simple columns.
// Also removes the LTcaptype{none} wrapper which requires ltcaption.
func fixLongtable(latex []byte) []byte {
	s := string(latex)
	// Remove the {\def\LTcaptype{none}...} wrapper that causes "counter none" errors
	ltcaptypeRe := regexp.MustCompile(`\{\s*\\def\\LTcaptype\{none\} % do not increment counter\s*\n`)
	s = ltcaptypeRe.ReplaceAllString(s, "")
	// Remove the closing } that matched the wrapper (after \end{longtable})
	closeBraceRe := regexp.MustCompile(`(\\end\{longtable\}\s*\n)\}`)
	s = closeBraceRe.ReplaceAllString(s, "$1")

	// Replace every longtable column spec with a width-appropriate simple one.
	colSpecRe := regexp.MustCompile(`\\begin\{longtable\}\[\]\{@\{\}\s*(?s:.*?)\}\}\s*\\toprule\\noalign\{\}`)
	colDescRe := regexp.MustCompile(`\[\]\{@\{\}(.*?)@\{\}\}`)
	s = colSpecRe.ReplaceAllStringFunc(s, func(match string) string {
		ncols := strings.Count(match, `>{\raggedright`)
		if ncols == 0 {
			if m := colDescRe.FindStringSubmatch(match); len(m) > 1 {
				for _, ch := range m[1] {
					if ch == 'l' || ch == 'r' || ch == 'c' {
						ncols++
					}
				}
			}
		}
		if ncols == 0 {
			ncols = 4 // last-resort fallback
		}
		var spec string
		switch ncols {
		case 2:
			spec = `@{}p{0.38\linewidth}p{0.52\linewidth}@{}`
		case 3:
			spec = `@{}p{0.22\linewidth}p{0.28\linewidth}p{0.40\linewidth}@{}`
		case 4:
			spec = `@{}p{1.5cm}p{2.2cm}p{3.8cm}p{0.35\linewidth}@{}`
		default:
			w := 0.88 / float64(ncols)
			var sb strings.Builder
			sb.WriteString("@{}")
			for i := 0; i < ncols; i++ {
				fmt.Fprintf(&sb, `p{%.2f\linewidth}`, w)
			}
			sb.WriteString("@{}")
			spec = sb.String()
		}
		return `\begin{longtable}{` + spec + `}` + "\n" + `\toprule` + "\n" + `\noalign{}`
	})

	// Replace minipage-wrapped header cells with plain text
	minipageRe := regexp.MustCompile(`\\begin\{minipage\}\[b\]\{\\linewidth\}\\raggedright\s*\n\s*([^\n]+)\s*\n\s*\\end\{minipage\}`)
	s = minipageRe.ReplaceAllString(s, "$1")
	return []byte(s)
}

// wrapStyledSections wraps "Root Cause" and "Fix" / "Fix Applied" sections in
// the styled tcolorbox environments (rootcausebox, fixbox) from styles.tex.
func wrapStyledSections(latex []byte) []byte {
	s := string(latex)
	rootCauseRe := regexp.MustCompile(`\\(?:section|subsection)\{Root Cause\}[^\n]*\n`)
	fixRe := regexp.MustCompile(`\\(?:section|subsection)\{Fix(?: Applied)?\}[^\n]*\n`)
	nextSecRe := regexp.MustCompile(`\n\\(?:section|subsection)\{`)

	wrap := func(re *regexp.Regexp, boxName string) {
		idx := re.FindStringIndex(s)
		for idx != nil {
			start, end := idx[0], idx[1]
			replacement := "\\begin{" + boxName + "}\n"
			s = s[:start] + replacement + s[end:]
			searchFrom := start + len(replacement)
			nextSec := nextSecRe.FindStringIndex(s[searchFrom:])
			var insertPos int
			if nextSec != nil {
				insertPos = searchFrom + nextSec[0]
			} else if docEnd := strings.Index(s[searchFrom:], "\\end{document}"); docEnd >= 0 {
				insertPos = searchFrom + docEnd
			} else {
				insertPos = len(s)
			}
			s = s[:insertPos] + "\n\\end{" + boxName + "}\n" + s[insertPos:]
			idx = re.FindStringIndex(s)
		}
	}
	wrap(rootCauseRe, "rootcausebox")
	wrap(fixRe, "fixbox")
	return []byte(s)
}

// MDToTex reads .md files from dbgDir, converts to LaTeX via pandoc, applies
// styled boxes (rootcausebox, fixbox), and returns the full document content.
// pkg and date substitute <package> and <YYYY-MM-DD> in the template.
func MDToTex(dbgDir, pkg, date string) (tex string, mdCount int, err error) {
	entries, err := os.ReadDir(dbgDir)
	if err != nil {
		return "", 0, fmt.Errorf("read dir %s: %w", dbgDir, err)
	}
	var mdFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if strings.HasSuffix(strings.ToLower(name), ".md") {
			// Exclude template fragments (frag_*.md) — they contain placeholders, not report content
			if strings.HasPrefix(name, "frag_") {
				continue
			}
			// Exclude Delve Evidence Checklist (not included in PDF)
			if name == "99_checklist.md" {
				continue
			}
			mdFiles = append(mdFiles, name)
		}
	}
	if len(mdFiles) == 0 {
		return "", 0, fmt.Errorf("no .md files found in %s", dbgDir)
	}
	sort.Strings(mdFiles)

	var mdBody strings.Builder
	for i, name := range mdFiles {
		if i > 0 {
			mdBody.WriteString("\n\n")
		}
		content, err := os.ReadFile(filepath.Join(dbgDir, name))
		if err != nil {
			return "", 0, fmt.Errorf("read %s: %w", name, err)
		}
		mdBody.Write(content)
	}

	mdStr := mdBody.String()
	mdStr = fixMarkdownTables(mdStr)

	if _, err := exec.LookPath("pandoc"); err != nil {
		return "", 0, fmt.Errorf("pandoc is required to convert markdown to LaTeX: %w", err)
	}
	mintedFilter, err := templateFS.ReadFile("templates/lua/minted.lua")
	if err != nil {
		return "", 0, fmt.Errorf("read minted filter: %w", err)
	}
	filterFile, err := os.CreateTemp("", "delve-minted-*.lua")
	if err != nil {
		return "", 0, fmt.Errorf("create temp filter: %w", err)
	}
	defer os.Remove(filterFile.Name())
	if _, err := filterFile.Write(mintedFilter); err != nil {
		filterFile.Close()
		return "", 0, fmt.Errorf("write minted filter: %w", err)
	}
	if err := filterFile.Close(); err != nil {
		return "", 0, fmt.Errorf("close minted filter: %w", err)
	}
	pandoc := exec.Command("pandoc", "-f", "markdown", "-t", "latex", "--wrap=preserve",
		"--lua-filter="+filterFile.Name())
	pandoc.Stdin = strings.NewReader(mdStr)
	latexBody, err := pandoc.Output()
	if err != nil {
		return "", 0, fmt.Errorf("pandoc failed: %w", err)
	}
	latexBody = fixLongtable(latexBody)
	latexBody = wrapStyledSections(latexBody)

	tpl, err := templateFS.ReadFile("templates/tex/debug_report_template_md.tex")
	if err != nil {
		return "", 0, fmt.Errorf("read template: %w", err)
	}
	if !strings.Contains(string(tpl), "%%MD_BODY%%") {
		return "", 0, fmt.Errorf("template missing %%MD_BODY%% placeholder")
	}
	out := strings.Replace(string(tpl), "%%MD_BODY%%", string(latexBody), 1)
	if pkg != "" {
		out = strings.ReplaceAll(out, "<package>", pkg)
	}
	if date != "" {
		out = strings.ReplaceAll(out, "<YYYY-MM-DD>", date)
	}
	return out, len(mdFiles), nil
}

// ensureReportTemplates copies preamble (and styles) from embedded templates to dbgDir
// so PDF compilation works even if Step 0 was skipped or used stale files.
func ensureReportTemplates(dbgDir string) error {
	for _, name := range []string{"debug_report_preamble.tex", "styles.tex"} {
		content, err := templateFS.ReadFile("templates/tex/" + name)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", name, err)
		}
		dest := filepath.Join(dbgDir, name)
		if err := os.WriteFile(dest, content, 0644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}
	return nil
}

// TexToPDF compiles debug_report.tex in dbgDir to PDF using pdflatex.
// Ensures preamble (with inlined styles) is present before compiling.
func TexToPDF(dbgDir string) error {
	if err := ensureReportTemplates(dbgDir); err != nil {
		return err
	}
	if _, err := exec.LookPath("pdflatex"); err != nil {
		return fmt.Errorf("pdflatex is required to compile PDF: %w", err)
	}
	pdfPath := filepath.Join(dbgDir, "debug_report.pdf")
	for i := 0; i < 2; i++ {
		cmd := exec.Command("pdflatex", "-shell-escape", "-interaction=nonstopmode", "debug_report.tex")
		cmd.Dir = dbgDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
	if _, err := os.Stat(pdfPath); err != nil {
		return fmt.Errorf("pdflatex did not produce %s: %w", pdfPath, err)
	}
	fmt.Printf("compiled %s\n", pdfPath)
	return nil
}

// CopyPDF copies debug_report.pdf from dbgDir to dest (resolved to absolute path).
func CopyPDF(dbgDir, dest string) error {
	src := filepath.Join(dbgDir, "debug_report.pdf")
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	if err := os.WriteFile(absDest, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", absDest, err)
	}
	fmt.Printf("copied %s -> %s\n", src, absDest)
	return nil
}
