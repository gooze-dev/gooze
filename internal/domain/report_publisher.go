package domain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
	registry OCIRegistry
}

// NewReportPublisher creates a ReportPublisher backed by the given registry.
func NewReportPublisher(registry OCIRegistry) ReportPublisher {
	return &reportPublisher{registry: registry}
}

func (p *reportPublisher) Push(ctx context.Context, args PushArgs) error {
	return transfer(ctx, "push", args.Ref, args.Reports, RegistryOptions{PlainHTTP: args.PlainHTTP, Insecure: args.Insecure}, p.registry.Push)
}

func (p *reportPublisher) Pull(ctx context.Context, args PullArgs) error {
	return transfer(ctx, "pull", args.Ref, args.Reports, RegistryOptions{PlainHTTP: args.PlainHTTP, Insecure: args.Insecure}, p.registry.Pull)
}

// transfer pushes or pulls (op) the reports at ref with move, the registry's
// Push or Pull. Its errors end the command with ExitRegistry.
func transfer(ctx context.Context, op, ref string, reports m.Path, opts RegistryOptions,
	move func(context.Context, string, m.Path, RegistryOptions) error,
) error {
	if err := validateRef(ref, reports); err != nil {
		return err
	}

	slog.Info("Transferring reports", "op", op, "ref", ref, "reports", reports)

	if err := move(ctx, ref, reports, opts); err != nil {
		return WithExitCode(m.ExitRegistry, fmt.Errorf("%s reports: %w", op, err))
	}

	return nil
}

func validateRef(ref string, reports m.Path) error {
	if ref == "" {
		return WithExitCode(m.ExitUsage, errors.New("registry reference is required"))
	}

	if reports == "" {
		return WithExitCode(m.ExitUsage, errors.New("reports directory is required"))
	}

	return nil
}
