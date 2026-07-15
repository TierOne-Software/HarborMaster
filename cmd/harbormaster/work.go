package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/manager"
	"github.com/tierone/harbormaster/pkg/ui"
	"github.com/tierone/harbormaster/pkg/work"
)

var (
	workProject string
	workTag     string
	workMessage string
	workJSON    bool
	workAll     bool
	workForce   bool
	workTitle   string
	workBody    string
)

var workCmd = &cobra.Command{
	Use:   "work",
	Short: "Manage coordinated multi-repository work sessions",
	Long: `Manage work sessions that span multiple repositories.

A work session creates a shared branch across selected repositories,
allowing you to commit, push, and create pull requests across all of
them in a single command.

Use 'hm work start' to begin and 'hm work end' to finish a session.`,
}

var workStartCmd = &cobra.Command{
	Use:   "start <branch> [repository...]",
	Short: "Start a new work session by creating a branch across repositories",
	Long: `Start a new work session. Creates the named branch in all matching
repositories and tracks them as a group.

Repositories can be selected by name, project, or tag; all selectors
are combined as a union. If no filter is specified, all git
repositories in the workspace are included.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runWorkStart,
}

var workEndCmd = &cobra.Command{
	Use:   "end",
	Short: "End the current work session and restore original branches",
	Long: `End the current work session. Each repository is checked out back
to the branch it was on before the session started.

Fails if any repository has uncommitted changes unless --force is used.`,
	RunE: runWorkEnd,
}

var workAddCmd = &cobra.Command{
	Use:   "add <repository>",
	Short: "Add a repository to the current work session",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkAdd,
}

var workRemoveCmd = &cobra.Command{
	Use:     "remove <repository>",
	Aliases: []string{"rm"},
	Short:   "Remove a repository from the current work session",
	Args:    cobra.ExactArgs(1),
	RunE:    runWorkRemove,
}

var workStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all repositories in the current work session",
	RunE:  runWorkStatus,
}

var workCommitCmd = &cobra.Command{
	Use:   "commit [repository...]",
	Short: "Commit changes across work session repositories",
	Long: `Stage and commit all changes in work session repositories.

By default, commits only in repos specified as arguments. Use --all
to commit across all repos in the session. Repos with no changes are skipped.`,
	RunE: runWorkCommit,
}

var workPushCmd = &cobra.Command{
	Use:   "push [repository...]",
	Short: "Push branches across work session repositories",
	Long: `Push the work session branch to origin for selected repositories.

By default, pushes only repos specified as arguments. Use --all to
push all repos in the session. Sets upstream tracking on first push.`,
	RunE: runWorkPush,
}

var workPRCmd = &cobra.Command{
	Use:   "pr [repository...]",
	Short: "Create pull requests across work session repositories",
	Long: `Create pull requests for the work session branch in selected repositories.

Requires the GitHub CLI (gh) to be installed and authenticated.
By default, creates PRs only for repos specified as arguments.
Use --all to create PRs for all repos in the session.`,
	RunE: runWorkPR,
}

func init() {
	// start flags
	workStartCmd.Flags().StringVarP(&workProject, "project", "p", "", "include repositories from project")
	workStartCmd.Flags().StringVarP(&workTag, "tag", "t", "", "include repositories with tag")

	// end flags
	workEndCmd.Flags().BoolVarP(&workForce, "force", "f", false, "end even with uncommitted changes")

	// status flags
	workStatusCmd.Flags().BoolVar(&workJSON, "json", false, "output as JSON")

	// commit flags
	workCommitCmd.Flags().StringVarP(&workMessage, "message", "m", "", "commit message")
	_ = workCommitCmd.MarkFlagRequired("message")
	workCommitCmd.Flags().BoolVar(&workAll, "all", false, "commit in all work session repositories")

	// push flags
	workPushCmd.Flags().BoolVar(&workAll, "all", false, "push all work session repositories")

	// pr flags
	workPRCmd.Flags().StringVar(&workTitle, "title", "", "pull request title (defaults to branch name)")
	workPRCmd.Flags().StringVar(&workBody, "body", "", "pull request body")
	workPRCmd.Flags().BoolVar(&workAll, "all", false, "create PRs for all work session repositories")

	// Build command tree
	workCmd.AddCommand(workStartCmd)
	workCmd.AddCommand(workEndCmd)
	workCmd.AddCommand(workAddCmd)
	workCmd.AddCommand(workRemoveCmd)
	workCmd.AddCommand(workStatusCmd)
	workCmd.AddCommand(workCommitCmd)
	workCmd.AddCommand(workPushCmd)
	workCmd.AddCommand(workPRCmd)
	rootCmd.AddCommand(workCmd)
}

func requireWorkSession() error {
	if ws == nil {
		return fmt.Errorf("no active work session\nUse 'hm work start <branch>' to begin one")
	}
	return nil
}

func runWorkStart(cmd *cobra.Command, args []string) error {
	branch := args[0]
	repoNames := args[1:]

	// Check for existing session
	if ws != nil {
		return fmt.Errorf("a work session is already active: '%s'\nUse 'hm work end' to finish it first", ws.Name)
	}

	// Build filter: names, --project, and --tag are unioned.
	filter := buildFilter(repoNames, workProject, workTag)

	// Create manager
	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	// Start work session
	session, err := mgr.WorkStart(branch, branch, filter)
	if err != nil {
		return err
	}

	// Save session. If the session file cannot be written, nothing records
	// the branch switches WorkStart just made, so roll them back rather
	// than stranding every repository on an untracked work branch.
	ws = session
	if err := saveWorkFile(); err != nil {
		ws = nil
		if rbErr := mgr.WorkEnd(session, true); rbErr != nil {
			return fmt.Errorf("failed to save work session: %w\n"+
				"rollback also failed: %v — repositories may still be on branch %q", err, rbErr, branch)
		}
		return fmt.Errorf("failed to save work session: %w (repositories were restored to their original branches)", err)
	}

	if !quiet {
		fmt.Printf("Started work session '%s' with %d repositories:\n", ws.Name, len(ws.Repos))
		for _, r := range ws.Repos {
			fmt.Printf("  %s %s (%s -> %s)\n",
				ui.SuccessStyle.Render("✓"),
				r.Name, r.OriginalBranch, branch)
		}
	}

	return nil
}

func runWorkEnd(cmd *cobra.Command, args []string) error {
	if err := requireWorkSession(); err != nil {
		return err
	}

	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	if err := mgr.WorkEnd(ws, workForce); err != nil {
		return err
	}

	if !quiet {
		fmt.Printf("Ended work session '%s'. Restored %d repositories to original branches.\n",
			ws.Name, len(ws.Repos))
	}

	// Delete the work session file
	if err := deleteWorkFile(); err != nil {
		return fmt.Errorf("failed to remove work session file: %w", err)
	}

	ws = nil
	return nil
}

func runWorkAdd(cmd *cobra.Command, args []string) error {
	if err := requireWorkSession(); err != nil {
		return err
	}

	repoName := args[0]

	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	if err := mgr.WorkAdd(ws, repoName); err != nil {
		return err
	}

	if err := saveWorkFile(); err != nil {
		return fmt.Errorf("failed to save work session: %w", err)
	}

	if !quiet {
		fmt.Printf("Added '%s' to work session '%s'\n", repoName, ws.Name)
	}

	return nil
}

func runWorkRemove(cmd *cobra.Command, args []string) error {
	if err := requireWorkSession(); err != nil {
		return err
	}

	repoName := args[0]

	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	if err := mgr.WorkRemove(ws, repoName); err != nil {
		return err
	}

	if err := saveWorkFile(); err != nil {
		return fmt.Errorf("failed to save work session: %w", err)
	}

	if !quiet {
		fmt.Printf("Removed '%s' from work session '%s'\n", repoName, ws.Name)
	}

	return nil
}

func runWorkStatus(cmd *cobra.Command, args []string) error {
	if err := requireWorkSession(); err != nil {
		return err
	}

	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	statuses, err := mgr.WorkStatus(ws)
	if err != nil {
		return err
	}

	if workJSON {
		return outputWorkStatusJSON(ws, statuses)
	}

	return outputWorkStatusTable(ws, statuses)
}

func outputWorkStatusJSON(session *work.WorkSession, statuses []manager.WorkRepoStatus) error {
	type jsonFileChange struct {
		Status string `json:"status"`
		Path   string `json:"path"`
	}

	type jsonRepoStatus struct {
		Name           string           `json:"name"`
		Path           string           `json:"path"`
		Branch         string           `json:"branch"`
		OriginalBranch string           `json:"original_branch"`
		IsDirty        bool             `json:"is_dirty"`
		IsOnBranch     bool             `json:"is_on_branch"`
		ChangedFiles   []jsonFileChange `json:"changed_files,omitempty"`
		Error          string           `json:"error,omitempty"`
	}

	type jsonOutput struct {
		Name   string           `json:"name"`
		Branch string           `json:"branch"`
		Repos  []jsonRepoStatus `json:"repos"`
	}

	output := jsonOutput{
		Name:   session.Name,
		Branch: session.Branch,
		Repos:  make([]jsonRepoStatus, len(statuses)),
	}

	for i, s := range statuses {
		rs := jsonRepoStatus{
			Name:           s.Name,
			Path:           s.Path,
			Branch:         s.Branch,
			OriginalBranch: s.OriginalBranch,
			IsDirty:        s.IsDirty,
			IsOnBranch:     s.IsOnBranch,
		}
		if s.Error != nil {
			rs.Error = s.Error.Error()
		}
		for _, f := range s.ChangedFiles {
			rs.ChangedFiles = append(rs.ChangedFiles, jsonFileChange{
				Status: f.Status,
				Path:   f.Path,
			})
		}
		output.Repos[i] = rs
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputWorkStatusTable(session *work.WorkSession, statuses []manager.WorkRepoStatus) error {
	fmt.Printf("Work session: %s (branch: %s)\n\n", session.Name, session.Branch)

	maxNameWidth := 10
	for _, s := range statuses {
		if len(s.Name) > maxNameWidth {
			maxNameWidth = len(s.Name)
		}
	}

	fmt.Printf("%-*s  %-8s  %-15s  %s\n",
		maxNameWidth, "REPOSITORY", "STATUS", "BRANCH", "CHANGES")

	for _, s := range statuses {
		var status, statusPlain string
		if s.Error != nil {
			status = ui.ErrorStyle.Render("error")
			statusPlain = "error"
		} else if !s.IsOnBranch {
			status = ui.WarningStyle.Render("detached")
			statusPlain = "detached"
		} else if s.IsDirty {
			status = ui.WarningStyle.Render("dirty")
			statusPlain = "dirty"
		} else {
			status = ui.SuccessStyle.Render("clean")
			statusPlain = "clean"
		}

		changeCount := "-"
		if len(s.ChangedFiles) > 0 {
			changeCount = fmt.Sprintf("%d files", len(s.ChangedFiles))
		}

		statusPadding := 8 - len(statusPlain)
		fmt.Printf("%-*s  %s%*s  %-15s  %s\n",
			maxNameWidth, s.Name,
			status, statusPadding, "",
			s.Branch,
			changeCount,
		)

		// Show changed files for dirty repos
		if s.IsDirty && len(s.ChangedFiles) > 0 {
			for _, f := range s.ChangedFiles {
				fmt.Printf("%-*s    %s %s\n", maxNameWidth, "", f.Status, f.Path)
			}
		}
	}

	return nil
}

func runWorkCommit(cmd *cobra.Command, args []string) error {
	if err := requireWorkSession(); err != nil {
		return err
	}

	repoNames, err := workTargetRepos(args)
	if err != nil {
		return err
	}

	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	results, err := mgr.WorkCommit(ws, workMessage, repoNames)
	if err != nil {
		return err
	}

	// Failures always go to stderr, even with --quiet.
	hasErrors := false
	for _, r := range results {
		if r.Error != nil {
			hasErrors = true
			fmt.Fprintf(os.Stderr, "  %s %s: %s\n", ui.ErrorStyle.Render("✗"), r.RepoName, r.Error)
		} else if !quiet {
			if r.SHA == "" {
				fmt.Printf("  %s %s: nothing to commit\n", ui.WarningStyle.Render("-"), r.RepoName)
			} else {
				fmt.Printf("  %s %s: %s\n", ui.SuccessStyle.Render("✓"), r.RepoName, shortSHA(r.SHA))
			}
		}
	}
	if hasErrors {
		return fmt.Errorf("some commits failed")
	}

	return nil
}

// workTargetRepos resolves the repositories a work subcommand operates on.
// Either explicit names or --all must be given, but not both; with --all
// it returns nil, which the manager treats as "all session repos".
func workTargetRepos(args []string) ([]string, error) {
	if workAll {
		if len(args) > 0 {
			return nil, fmt.Errorf("cannot combine --all with explicit repository names")
		}
		return nil, nil
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("specify repository names or use --all")
	}
	return args, nil
}

func runWorkPush(cmd *cobra.Command, args []string) error {
	if err := requireWorkSession(); err != nil {
		return err
	}

	repoNames, err := workTargetRepos(args)
	if err != nil {
		return err
	}

	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	results, err := mgr.WorkPush(ws, repoNames)
	if err != nil {
		return err
	}

	// Failures always go to stderr, even with --quiet.
	hasErrors := false
	for _, r := range results {
		if r.Error != nil {
			hasErrors = true
			fmt.Fprintf(os.Stderr, "  %s %s: %s\n", ui.ErrorStyle.Render("✗"), r.RepoName, r.Error)
		} else if !quiet {
			fmt.Printf("  %s %s: pushed to origin/%s\n", ui.SuccessStyle.Render("✓"), r.RepoName, ws.Branch)
		}
	}
	if hasErrors {
		return fmt.Errorf("some pushes failed")
	}

	return nil
}

func runWorkPR(cmd *cobra.Command, args []string) error {
	if err := requireWorkSession(); err != nil {
		return err
	}

	// Check if gh is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) is required for PR creation\nInstall it from https://cli.github.com/")
	}

	repoNames, err := workTargetRepos(args)
	if err != nil {
		return err
	}
	if repoNames == nil {
		repoNames = ws.RepoNames()
	}

	title := workTitle
	if title == "" {
		title = ws.Branch
	}

	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	hasErrors := false
	for _, name := range repoNames {
		if !ws.HasRepo(name) {
			fmt.Fprintf(os.Stderr, "  %s %s: not in work session\n", ui.ErrorStyle.Render("✗"), name)
			hasErrors = true
			continue
		}

		repo, ok := cfg.GetRepository(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "  %s %s: not found in config\n", ui.ErrorStyle.Render("✗"), name)
			hasErrors = true
			continue
		}

		repoPath := mgr.GetRepoPath(repo)

		ghArgs := []string{"pr", "create", "--title", title}
		if workBody != "" {
			ghArgs = append(ghArgs, "--body", workBody)
		}

		ghCmd := exec.Command("gh", ghArgs...)
		ghCmd.Dir = repoPath
		output, err := ghCmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s %s: %s\n", ui.ErrorStyle.Render("✗"), name, strings.TrimSpace(string(output)))
			hasErrors = true
			continue
		}

		prURL := strings.TrimSpace(string(output))
		if !quiet {
			fmt.Printf("  %s %s: %s\n", ui.SuccessStyle.Render("✓"), name, prURL)
		}
	}

	if hasErrors {
		return fmt.Errorf("some PR creations failed")
	}

	return nil
}
