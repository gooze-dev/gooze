package adapter

import (
	"fmt"
	"reflect"

	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/spf13/cobra"
)

// SimpleUI implements UI using cobra Command's Println.
type SimpleUI struct {
	cmd *cobra.Command
}

// NewSimpleUI creates a new SimpleUI.
func NewSimpleUI(cmd *cobra.Command) *SimpleUI {
	return &SimpleUI{cmd: cmd}
}

// ShowNotImplemented displays a "not implemented" message.
func (p *SimpleUI) ShowNotImplemented(count int) error {
	p.cmd.Printf("Found %d source files\n", count)
	p.cmd.Println("Mutation testing not yet implemented. Use --list or -l flag to see mutation counts.")

	return nil
}

// DisplayMutationEstimations displays pre-calculated mutation estimations.
func (p *SimpleUI) DisplayMutationEstimations(estimations map[m.Path]MutationEstimation) error {
	if len(estimations) == 0 {
		p.cmd.Printf("\nTotal: 0 mutations across 0 files\n")
		return nil
	}

	totalArithmetic := 0
	totalBoolean := 0

	// Sort paths for consistent output
	paths := make([]m.Path, 0, len(estimations))
	for path := range estimations {
		paths = append(paths, path)
	}

	// Simple sort by string comparison
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if string(paths[i]) > string(paths[j]) {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}

	// Display per-file counts
	for _, path := range paths {
		est := estimations[path]
		p.cmd.Printf("%s:\n", path)
		p.cmd.Printf("  - %d arithmetic mutations\n", est.Arithmetic)
		p.cmd.Printf("  - %d boolean mutations\n", est.Boolean)
		totalArithmetic += est.Arithmetic
		totalBoolean += est.Boolean
	}

	// Display totals
	p.cmd.Printf("\nTotal across %d file(s):\n", len(estimations))
	p.cmd.Printf("  - %d arithmetic mutations\n", totalArithmetic)
	p.cmd.Printf("  - %d boolean mutations\n", totalBoolean)

	return nil
}

// DisplayMutationResults displays the results of mutation testing.
func (p *SimpleUI) DisplayMutationResults(sources []m.Source, fileResults map[m.Path]interface{}) error {
	if err := p.outPrintf("\nMutation Testing Results:\n"); err != nil {
		return err
	}

	if err := p.outPrintf("========================\n\n"); err != nil {
		return err
	}

	totalKilled := 0
	totalSurvived := 0
	totalMutations := 0

	sortedSources := sortSources(sources)

	// Display results for each file
	for _, source := range sortedSources {
		result := fileResults[source.Origin]
		if result == nil {
			continue
		}

		reports := extractReportsFromResult(result)
		fileKilled, fileSurvived := countReportStats(reports)

		fileMutations := len(reports)
		totalMutations += fileMutations
		totalKilled += fileKilled
		totalSurvived += fileSurvived

		if err := p.displayFileReports(source.Origin, fileMutations, reports); err != nil {
			return err
		}
	}

	return p.displaySummary(totalMutations, totalKilled, totalSurvived)
}

func sortSources(sources []m.Source) []m.Source {
	// Sort sources for consistent output
	sortedSources := make([]m.Source, len(sources))
	copy(sortedSources, sources)

	for i := 0; i < len(sortedSources); i++ {
		for j := i + 1; j < len(sortedSources); j++ {
			if string(sortedSources[i].Origin) > string(sortedSources[j].Origin) {
				sortedSources[i], sortedSources[j] = sortedSources[j], sortedSources[i]
			}
		}
	}

	return sortedSources
}

func countReportStats(reports []m.Report) (killed, survived int) {
	for _, report := range reports {
		if report.Killed {
			killed++
		} else {
			survived++
		}
	}

	return
}

func (p *SimpleUI) displayFileReports(origin m.Path, fileMutations int, reports []m.Report) error {
	if err := p.outPrintf("%s: %d mutations\n", origin, fileMutations); err != nil {
		return err
	}

	if fileMutations == 0 {
		return nil
	}

	// Show each mutation result
	for _, report := range reports {
		var err error
		if report.Killed {
			err = p.outPrintf("  ✓ %s - killed\n", report.MutationID)
		} else {
			err = p.outPrintf("  ✗ %s - survived\n", report.MutationID)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func (p *SimpleUI) displaySummary(totalMutations, totalKilled, totalSurvived int) error {
	score := 0.0
	if totalMutations > 0 {
		score = float64(totalKilled) / float64(totalMutations) * 100
	}

	if err := p.outPrintf("\nSummary:\n"); err != nil {
		return err
	}

	return p.outPrintf("Total: %d | Killed: %d | Survived: %d | Score: %.1f%%\n",
		totalMutations, totalKilled, totalSurvived, score)
}

// outPrintf writes formatted output to the underlying cobra command's stdout.
func (p *SimpleUI) outPrintf(format string, args ...interface{}) error {
	_, err := fmt.Fprintf(p.cmd.OutOrStdout(), format, args...)
	return err
}

// extractReportsFromResult extracts mutation reports from a result value using
// either a Reports() method or reflection on a Reports field.
func extractReportsFromResult(result interface{}) []m.Report {
	if result == nil {
		return nil
	}

	if ptr, ok := result.(interface{ Reports() []m.Report }); ok {
		return ptr.Reports()
	}

	v := reflect.ValueOf(result)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil
	}

	reportsField := v.FieldByName("Reports")
	if !reportsField.IsValid() || reportsField.Kind() != reflect.Slice {
		return nil
	}

	if reports, ok := reportsField.Interface().([]m.Report); ok {
		return reports
	}

	return nil
}
