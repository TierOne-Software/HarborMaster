package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/manager"
)

var (
	lockUpdateProject string
	lockUpdateTag     string
	lockUpdateDryRun  bool
	lockUpdateSync    bool

	lockAdoptProject string
	lockAdoptTag     string
	lockAdoptDryRun  bool
	lockAdoptForce   bool
)

var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Manage the lock file",
	Long: `Manage the lock file without touching repository checkouts.

'hm lock update' pins each repository to the latest commit on its
configured branch (resolved against the remote). 'hm lock adopt' pins each
repository to its current local HEAD. Both only rewrite .harbormaster.lock;
use 'hm sync' (or 'hm lock update --sync') to move the checkouts.`,
}

var lockUpdateCmd = &cobra.Command{
	Use:   "update [repository...]",
	Short: "Pin repositories to the latest commit on their configured branch",
	Long: `Resolve each repository's configured branch against its remote and
record the tip commit in the lock file. Local checkouts are not modified.

Repositories whose config pins a commit or tag are skipped: their lock
entry follows the config, not a moving branch.

Positional repository names, --project, and --tag can be combined; the
union of all matching repositories is updated.`,
	RunE: runLockUpdate,
}

var lockAdoptCmd = &cobra.Command{
	Use:   "adopt [repository...]",
	Short: "Pin repositories to their current local HEAD",
	Long: `Record each repository's current local HEAD commit in the lock
file. The checkout is not modified.

A HEAD that has not been pushed to origin cannot be reproduced by
'hm sync --locked' on another machine, so adopting it is refused unless
--force is given.

Positional repository names, --project, and --tag can be combined; the
union of all matching repositories is updated.`,
	RunE: runLockAdopt,
}

func init() {
	lockUpdateCmd.Flags().StringVarP(&lockUpdateProject, "project", "p", "", "update repositories in project")
	lockUpdateCmd.Flags().StringVarP(&lockUpdateTag, "tag", "t", "", "update repositories with tag")
	lockUpdateCmd.Flags().BoolVar(&lockUpdateDryRun, "dry-run", false, "show what would change without writing the lock file")
	lockUpdateCmd.Flags().BoolVar(&lockUpdateSync, "sync", false, "also check out the new pins after updating the lock file")

	lockAdoptCmd.Flags().StringVarP(&lockAdoptProject, "project", "p", "", "adopt repositories in project")
	lockAdoptCmd.Flags().StringVarP(&lockAdoptTag, "tag", "t", "", "adopt repositories with tag")
	lockAdoptCmd.Flags().BoolVar(&lockAdoptDryRun, "dry-run", false, "show what would change without writing the lock file")
	lockAdoptCmd.Flags().BoolVar(&lockAdoptForce, "force", false, "adopt a HEAD that has not been pushed to origin")

	lockCmd.AddCommand(lockUpdateCmd)
	lockCmd.AddCommand(lockAdoptCmd)
	rootCmd.AddCommand(lockCmd)
}

func runLockUpdate(cmd *cobra.Command, args []string) error {
	filter := buildFilter(args, lockUpdateProject, lockUpdateTag)
	mgr := manager.NewRepositoryManager(cfg, manager.WithLockFile(lf))

	changes, err := mgr.UpdateLockToRemote(filter)
	if err != nil {
		return err
	}

	printLockChanges(changes, "remote tip", lockUpdateDryRun)

	if lockUpdateDryRun {
		return lockChangesError(changes)
	}
	if err := saveLockFile(); err != nil {
		return fmt.Errorf("failed to save lock file: %w", err)
	}
	if err := lockChangesError(changes); err != nil {
		return err
	}

	if lockUpdateSync {
		return syncLockedRepos(filter)
	}
	return nil
}

func runLockAdopt(cmd *cobra.Command, args []string) error {
	filter := buildFilter(args, lockAdoptProject, lockAdoptTag)
	mgr := manager.NewRepositoryManager(cfg, manager.WithLockFile(lf))

	changes, err := mgr.UpdateLockToLocal(filter, lockAdoptForce)
	if err != nil {
		return err
	}

	printLockChanges(changes, "local HEAD", lockAdoptDryRun)

	if lockAdoptDryRun {
		return lockChangesError(changes)
	}
	if err := saveLockFile(); err != nil {
		return fmt.Errorf("failed to save lock file: %w", err)
	}
	return lockChangesError(changes)
}

// printLockChanges renders the per-repository results. Failures and
// warnings go to stderr so machine consumers of stdout never see them and
// --quiet cannot hide them; the table itself honors --quiet.
func printLockChanges(changes []manager.LockChange, source string, dryRun bool) {
	for _, c := range changes {
		if c.Error != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", c.Name, c.Error)
		}
		if c.Warning != "" {
			fmt.Fprintf(os.Stderr, "  %s: warning: %s\n", c.Name, c.Warning)
		}
	}

	if quiet {
		return
	}

	if dryRun {
		fmt.Println("Dry run — lock file will not be modified:")
		fmt.Println()
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, c := range changes {
		switch {
		case c.Error != nil:
			// Already reported on stderr.
		case c.Skipped:
			_, _ = fmt.Fprintf(w, "  %s	-	skipped: %s\n", c.Name, c.SkipReason)
		case !c.Changed:
			_, _ = fmt.Fprintf(w, "  %s	%s	already up to date\n", c.Name, shortSHA(c.NewSHA))
		default:
			old := shortSHA(c.OldSHA)
			if c.OldSHA == "" {
				old = "-"
			}
			note := source
			if c.Dirty {
				note += ", dirty worktree"
			}
			_, _ = fmt.Fprintf(w, "  %s	%s -> %s	%s\n", c.Name, old, shortSHA(c.NewSHA), note)
		}
	}
	_ = w.Flush()
}

// lockChangesError returns an error summarizing failed repositories, or nil.
func lockChangesError(changes []manager.LockChange) error {
	failed := 0
	for _, c := range changes {
		if c.Error != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d repositories failed", failed, len(changes))
	}
	return nil
}

// syncLockedRepos syncs the filtered repositories to their (newly updated)
// lock entries.
func syncLockedRepos(filter manager.Filter) error {
	interactive := !quiet && isatty.IsTerminal(os.Stdout.Fd())
	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
		manager.WithLocked(true),
		manager.WithInteractive(interactive),
	)

	// As in 'hm sync', --quiet silences the progress UI on stdout but never
	// the failure report on stderr.
	if quiet {
		if devnull, devErr := os.OpenFile(os.DevNull, os.O_WRONLY, 0); devErr == nil {
			orig := os.Stdout
			os.Stdout = devnull
			defer func() {
				os.Stdout = orig
				_ = devnull.Close()
			}()
		}
	}

	result, err := mgr.Sync(filter)
	if err != nil {
		return err
	}
	if result.HasFailures() {
		for _, f := range result.FailedResults() {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", f.RepoName, f.Error)
		}
		return fmt.Errorf("%d of %d repositories failed to sync", result.FailureCount, result.TotalRepos)
	}
	return nil
}
