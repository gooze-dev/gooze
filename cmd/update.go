package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/controller"
	"gooze.dev/pkg/gooze/internal/domain"
	"gooze.dev/pkg/gooze/pkg/config"
)

// refreshTimeout bounds a background release lookup. Nothing waits for it: the
// command exits whenever it is done, abandoning a lookup still in flight.
const refreshTimeout = 10 * time.Second

var releaseChecker domain.ReleaseChecker

// pendingNotice is the update notice known from the cache when the command
// started, printed when it ends. It is nil when there is nothing to announce.
var pendingNotice *domain.UpdateNotice

// buildVersion returns the module version this binary was built from, such as
// "v1.2.0", or "" when unknown.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	return info.Main.Version
}

// startUpdateCheck runs the update check for cmd without ever delaying it.
func startUpdateCheck(cmd *cobra.Command) {
	if releaseChecker == nil || !updateCheckEnabled(cmd) {
		return
	}

	pendingNotice = checkForUpdate(releaseChecker, buildVersion(), updateNotifier())
}

// checkForUpdate answers from the cache only, so it never touches the network
// on the command's path, and returns the notice to print at exit. When the
// cache is stale it refreshes it in a background goroutine whose result shows
// up in a running TUI and in later runs' cache, never at this exit: so an
// offline or slow network changes nothing the user sees. notifier may be nil.
func checkForUpdate(checker domain.ReleaseChecker, current string, notifier controller.UpdateNotifier) *domain.UpdateNotice {
	if !domain.Released(current) {
		return nil
	}

	cachedTag, fresh := checker.Cached()

	cached, hasCached := domain.NoticeFor(current, cachedTag)
	if hasCached && notifier != nil {
		notifier.DisplayUpdateNotice(context.Background(), cached)
	}

	if !fresh {
		go refreshRelease(checker, current, cachedTag, notifier)
	}

	if !hasCached {
		return nil
	}

	return &cached
}

// refreshRelease looks up the latest release for later runs and, when it
// reveals a newer one than the cache did, tells a TUI that is still running.
// Failures are only logged: they must not reach the user.
func refreshRelease(checker domain.ReleaseChecker, current, cachedTag string, notifier controller.UpdateNotifier) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()

	latest, err := checker.Refresh(ctx)
	if err != nil {
		slog.Debug("update check failed", "error", err)

		return
	}

	if notice, ok := domain.NoticeFor(current, latest); ok && latest != cachedTag && notifier != nil {
		notifier.DisplayUpdateNotice(ctx, notice)
	}
}

// updateNotifier returns the UI as an UpdateNotifier when it can show notices
// while running (the TUI), or nil.
func updateNotifier() controller.UpdateNotifier {
	if notifier, ok := ui.(controller.UpdateNotifier); ok {
		return notifier
	}

	return nil
}

// updateCheckEnabled skips the check when opted out (no-update-check in
// gooze.yaml, --no-update-check or GOOZE_NO_UPDATE_CHECK) and for shell
// completion, whose output is consumed by the shell.
func updateCheckEnabled(cmd *cobra.Command) bool {
	if viper.GetBool(config.KeyNoUpdateCheck) {
		return false
	}

	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "completion" || strings.HasPrefix(c.Name(), cobra.ShellCompRequestCmd) {
			return false
		}
	}

	return true
}

// printUpdateNotice writes the notice known at startup to w. It never waits,
// and stays quiet when the user interrupted the command.
func printUpdateNotice(ctx context.Context, w io.Writer) {
	if pendingNotice == nil || ctx.Err() != nil {
		return
	}

	_, _ = fmt.Fprintln(w, *pendingNotice)
}
