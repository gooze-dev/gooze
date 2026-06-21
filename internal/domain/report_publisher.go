package domain

import (
	"context"
	"fmt"
	"log/slog"

	"gooze.dev/pkg/gooze/internal/adapter"
	m "gooze.dev/pkg/gooze/internal/model"
)

// PushArgs contains the arguments for pushing reports to a registry.
type PushArgs struct {
	Reports   m.Path
	Ref       string
	PlainHTTP bool
	Insecure  bool
}

// PullArgs contains the arguments for pulling reports from a registry.
type PullArgs struct {
	Reports   m.Path
	Ref       string
	PlainHTTP bool
	Insecure  bool
}

// ReportPublisher pushes and pulls mutation reports to/from an OCI registry.
type ReportPublisher interface {
	Push(ctx context.Context, args PushArgs) error
	Pull(ctx context.Context, args PullArgs) error
}

type reportPublisher struct {
	registry adapter.OCIRegistry
}

// NewReportPublisher creates a ReportPublisher backed by the given registry.
func NewReportPublisher(registry adapter.OCIRegistry) ReportPublisher {
	return &reportPublisher{registry: registry}
}

func (p *reportPublisher) Push(ctx context.Context, args PushArgs) error {
	if err := validateRef(args.Ref, args.Reports); err != nil {
		return err
	}

	slog.Info("Pushing reports", "ref", args.Ref, "reports", args.Reports)

	opts := adapter.RegistryOptions{PlainHTTP: args.PlainHTTP, Insecure: args.Insecure}
	if err := p.registry.Push(ctx, args.Ref, args.Reports, opts); err != nil {
		return fmt.Errorf("push reports: %w", err)
	}

	return nil
}

func (p *reportPublisher) Pull(ctx context.Context, args PullArgs) error {
	if err := validateRef(args.Ref, args.Reports); err != nil {
		return err
	}

	slog.Info("Pulling reports", "ref", args.Ref, "reports", args.Reports)

	opts := adapter.RegistryOptions{PlainHTTP: args.PlainHTTP, Insecure: args.Insecure}
	if err := p.registry.Pull(ctx, args.Ref, args.Reports, opts); err != nil {
		return fmt.Errorf("pull reports: %w", err)
	}

	return nil
}

func validateRef(ref string, reports m.Path) error {
	if ref == "" {
		return fmt.Errorf("registry reference is required")
	}

	if reports == "" {
		return fmt.Errorf("reports directory is required")
	}

	return nil
}
