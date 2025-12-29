package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/manager"
	"github.com/tierone/harbormaster/pkg/ui"
)

var (
	statusJSON      bool
	statusProject   string
	statusPorcelain bool
)

var statusCmd = &cobra.Command{
	Use:   "status [repository...]",
	Short: "Show repository status",
	Long: `Show the status of repositories in the workspace.

Displays whether each repository exists, its current commit, lock status,
and whether it needs updating.`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output as JSON")
	statusCmd.Flags().StringVarP(&statusProject, "project", "p", "", "show status for project only")
	statusCmd.Flags().BoolVar(&statusPorcelain, "porcelain", false, "machine-readable output")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Build filter
	filter := manager.Filter{}
	if len(args) > 0 {
		filter.Names = args
	} else if statusProject != "" {
		filter.Projects = []string{statusProject}
	} else {
		filter.All = true
	}

	// Create manager
	mgr := manager.NewRepositoryManager(cfg,
		manager.WithLockFile(lf),
	)

	// Get status
	statuses, err := mgr.Status(filter)
	if err != nil {
		return err
	}

	if len(statuses) == 0 {
		fmt.Println("No repositories configured")
		return nil
	}

	// Output based on format
	if statusJSON {
		return outputStatusJSON(statuses)
	}

	if statusPorcelain {
		return outputStatusPorcelain(statuses)
	}

	return outputStatusTable(statuses)
}

func outputStatusJSON(statuses []manager.RepoStatus) error {
	type jsonStatus struct {
		Name         string `json:"name"`
		Path         string `json:"path"`
		Exists       bool   `json:"exists"`
		CurrentSHA   string `json:"current_sha,omitempty"`
		LockedSHA    string `json:"locked_sha,omitempty"`
		RequestedRef string `json:"requested_ref"`
		Branch       string `json:"branch,omitempty"`
		IsDirty      bool   `json:"is_dirty"`
		NeedsUpdate  bool   `json:"needs_update"`
		Error        string `json:"error,omitempty"`
	}

	output := make([]jsonStatus, len(statuses))
	for i, s := range statuses {
		output[i] = jsonStatus{
			Name:         s.Name,
			Path:         s.Path,
			Exists:       s.Exists,
			CurrentSHA:   s.CurrentSHA,
			LockedSHA:    s.LockedSHA,
			RequestedRef: s.RequestedRef,
			Branch:       s.Branch,
			IsDirty:      s.IsDirty,
			NeedsUpdate:  s.NeedsUpdate,
		}
		if s.Error != nil {
			output[i].Error = s.Error.Error()
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputStatusPorcelain(statuses []manager.RepoStatus) error {
	for _, s := range statuses {
		status := "ok"
		if !s.Exists {
			status = "missing"
		} else if s.NeedsUpdate {
			status = "outdated"
		} else if s.IsDirty {
			status = "dirty"
		}
		if s.Error != nil {
			status = "error"
		}

		sha := s.CurrentSHA
		if sha != "" && len(sha) > 8 {
			sha = sha[:8]
		}

		fmt.Printf("%s\t%s\t%s\t%s\n", s.Name, status, sha, s.RequestedRef)
	}
	return nil
}

func outputStatusTable(statuses []manager.RepoStatus) error {
	// Calculate max repo name width
	maxNameWidth := 10
	for _, s := range statuses {
		if len(s.Name) > maxNameWidth {
			maxNameWidth = len(s.Name)
		}
	}

	// Print header
	fmt.Printf("%-*s  %-8s  %-15s  %-8s  %s\n",
		maxNameWidth, "REPOSITORY", "STATUS", "BRANCH", "COMMIT", "LOCK")

	for _, s := range statuses {
		status, statusPlain := getStatusString(s)
		branch := s.Branch
		if branch == "" {
			branch = s.RequestedRef
		}
		if len(branch) > 15 {
			branch = branch[:15]
		}

		commit := "-"
		if s.CurrentSHA != "" {
			commit = s.CurrentSHA[:min(8, len(s.CurrentSHA))]
		}

		lockStatus := "-"
		if s.LockedSHA != "" {
			if s.CurrentSHA == s.LockedSHA {
				lockStatus = ui.SuccessStyle.Render("locked")
			} else {
				lockStatus = ui.WarningStyle.Render("drift")
			}
		}

		// Print with fixed widths, accounting for ANSI codes in status
		// Status field: print colored text then pad with spaces
		statusPadding := 8 - len(statusPlain)
		fmt.Printf("%-*s  %s%*s  %-15s  %-8s  %s\n",
			maxNameWidth, s.Name,
			status, statusPadding, "",
			branch,
			commit,
			lockStatus,
		)
	}

	return nil
}

func getStatusString(s manager.RepoStatus) (styled string, plain string) {
	if s.Error != nil {
		return ui.ErrorStyle.Render("error"), "error"
	}
	if !s.Exists {
		return ui.WarningStyle.Render("missing"), "missing"
	}
	if s.IsDirty {
		return ui.WarningStyle.Render("dirty"), "dirty"
	}
	if s.NeedsUpdate {
		return ui.WarningStyle.Render("outdated"), "outdated"
	}
	return ui.SuccessStyle.Render("ok"), "ok"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
