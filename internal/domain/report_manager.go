package domain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gooze.dev/pkg/gooze/internal/adapter"
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
)

type ReportManager interface {
	// Deprecated: Use SaveStream for better performance and lower memory usage on large mutation sets.
	Save(ctx context.Context, reportsDir m.Path, reports pkg.FileSpill[m.Report]) error
	SaveStream(ctx context.Context, reportsDir m.Path, reports <-chan m.Report) error
	Merge(ctx context.Context, basePath m.Path) error
	Load(ctx context.Context, path m.Path) ([]m.Report, error)
}

type reportManager struct {
	adapter.ReportStore
}

func NewReportManager(reportStore adapter.ReportStore) ReportManager {
	return &reportManager{
		ReportStore: reportStore,
	}
}

func (rm *reportManager) Merge(ctx context.Context, basePath m.Path) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	shardDirs, err := rm.findShardDirs(basePath)
	if err != nil {
		slog.Error("Failed to find shard directories", "error", err, "basePath", basePath)
		return err
	}

	if len(shardDirs) == 0 {
		slog.Debug("No shard directories found; regenerating index only", "basePath", basePath)
		return rm.regenerateIndex(ctx, basePath)
	}

	merged, err := rm.mergeReports(ctx, basePath, shardDirs)
	if err != nil {
		slog.Error("Failed to merge shard reports", "error", err, "basePath", basePath)
		return err
	}

	slog.Info("Merged shard reports", "basePath", basePath)

	if err := rm.saveMergedReports(ctx, basePath, merged); err != nil {
		return err
	}

	slog.Debug("Saved merged reports and regenerated index", "basePath", basePath)

	return rm.removeShardDirs(shardDirs)
}

func (rm *reportManager) Save(ctx context.Context, reportsDir m.Path, reports pkg.FileSpill[m.Report]) error {
	err := rm.SaveSpillReports(ctx, reportsDir, reports)
	if err != nil {
		slog.Error("Failed to save reports", "error", err, "path", reportsDir)
		return fmt.Errorf("save reports: %w", err)
	}

	err = reports.Close()
	if err != nil {
		slog.Error("Failed to close reports spill", "error", err, "path", reportsDir)
		return fmt.Errorf("close reports spill: %w", err)
	}

	slog.Debug("Saved reports", "path", reportsDir)

	err = rm.regenerateIndex(ctx, reportsDir)
	if err != nil {
		slog.Error("Failed to regenerate reports index", "error", err, "path", reportsDir)
		return fmt.Errorf("regenerate index: %w", err)
	}

	slog.Debug("Regenerated reports index", "path", reportsDir)
	return nil
}

func (rm *reportManager) findShardDirs(base m.Path) ([]string, error) {
	shardDirs, err := findShardDirs(string(base))
	if err != nil {
		slog.Error("Failed to find shard directories", "error", err, "basePath", base)
		return nil, fmt.Errorf("find shard directories: %w", err)
	}

	return shardDirs, nil
}

func (rm *reportManager) SaveStream(ctx context.Context, reportsDir m.Path, reports <-chan m.Report) error {
	err := rm.SaveReportsStream(ctx, reportsDir, reports)
	if err != nil {
		slog.Error("Failed to save reports stream", "error", err, "path", reportsDir)
		return fmt.Errorf("save reports stream: %w", err)
	}

	slog.Debug("Saved reports stream", "path", reportsDir)

	err = rm.regenerateIndex(ctx, reportsDir)
	if err != nil {
		slog.Error("Failed to regenerate reports index after saving stream", "error", err, "path", reportsDir)
		return fmt.Errorf("regenerate index: %w", err)
	}

	slog.Debug("Regenerated reports index after saving stream", "path", reportsDir)
	return nil
}

func (rm *reportManager) Load(ctx context.Context, path m.Path) ([]m.Report, error) {
	reports, err := rm.LoadReports(ctx, path)
	if err != nil {
		slog.Error("Failed to load reports", "error", err, "path", path)
		return nil, fmt.Errorf("load reports: %w", err)
	}

	slog.Debug("Loaded reports", "path", path, "reportCount", len(reports))
	return reports, nil
}

func (rm *reportManager) mergeReports(ctx context.Context, base m.Path, shardDirs []string) ([]m.Report, error) {
	merged := make([]m.Report, 0)

	// First, load existing reports from base directory to preserve cache.
	existingReports, err := rm.loadReportsIfExists(ctx, base)
	if err != nil {
		return nil, fmt.Errorf("load existing reports from base: %w", err)
	}

	merged = append(merged, existingReports...)

	// Then load and merge reports from all shards.
	for _, shardDir := range shardDirs {
		reports, err := rm.LoadReports(ctx, m.Path(shardDir))
		if err != nil {
			slog.Error("Failed to load shard reports", "error", err, "shardDir", shardDir)
			return nil, fmt.Errorf("load shard reports from %s: %w", shardDir, err)
		}

		merged = append(merged, reports...)
	}

	slog.Debug("Merged reports from shards", "totalReports", len(merged))

	return merged, nil
}

func (rm *reportManager) loadReportsIfExists(ctx context.Context, path m.Path) ([]m.Report, error) {
	reports, err := rm.LoadReports(ctx, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	return reports, nil
}

func (rm *reportManager) removeShardDirs(shardDirs []string) error {
	for _, shardDir := range shardDirs {
		if err := os.RemoveAll(shardDir); err != nil {
			return fmt.Errorf("remove shard directory %s: %w", shardDir, err)
		}
	}

	return nil
}

func (rm *reportManager) saveMergedReports(ctx context.Context, basePath m.Path, reports []m.Report) error {
	if err := rm.SaveReports(ctx, basePath, reports); err != nil {
		slog.Error("Failed to save merged reports", "error", err, "basePath", basePath)
		return fmt.Errorf("save merged reports: %w", err)
	}

	slog.Debug("Saved merged reports and regenerated index", "basePath", basePath)

	return rm.regenerateIndex(ctx, basePath)
}

func (rm *reportManager) regenerateIndex(ctx context.Context, basePath m.Path) error {
	if err := rm.RegenerateIndex(ctx, basePath); err != nil {
		return fmt.Errorf("regenerate index: %w", err)
	}

	return nil
}

func findShardDirs(baseDir string) ([]string, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}

		return nil, err
	}

	shardDirs := make([]string, 0)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if !strings.HasPrefix(entry.Name(), ShardDirPrefix) {
			continue
		}

		shardDirs = append(shardDirs, filepath.Join(baseDir, entry.Name()))
	}

	sort.Strings(shardDirs)

	return shardDirs, nil
}
