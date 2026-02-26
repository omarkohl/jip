package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/omarkohl/jip/internal/auth"
	gh "github.com/omarkohl/jip/internal/github"
	"github.com/omarkohl/jip/internal/jj"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:     "send [revsets...]",
	Aliases: []string{"s"},
	Short:   "Create or update PRs for a stack of changes",
	Long: `Send creates or updates GitHub pull requests for each change in the
resolved stack. Each change gets its own PR targeting the base branch.

Default revset is @- (the last committed change and its ancestors up to base).`,
	RunE: runSend,
}

func init() {
	rootCmd.AddCommand(sendCmd)
	sendCmd.Flags().StringP("base", "b", "main", "Base branch")
	sendCmd.Flags().String("remote", "origin", "Push remote name")
	sendCmd.Flags().StringP("upstream", "u", "", "Upstream remote name or URL (where PRs are opened)")
	sendCmd.Flags().BoolP("dry-run", "n", false, "Show what would happen without making changes")
	sendCmd.Flags().StringSliceP("reviewer", "r", nil, "Add reviewers (repeatable, comma-separated)")
	sendCmd.Flags().BoolP("draft", "d", false, "Create PRs as drafts")
	sendCmd.Flags().BoolP("existing", "x", false, "Only update PRs that already exist (skip new ones)")
	sendCmd.Flags().Bool("no-stack", false, "Send only the tip of each stack as a single PR")
}

// sendOpts holds configuration for the send pipeline.
type sendOpts struct {
	base           string
	remote         string
	upstream       string // upstream remote URL (where PRs are opened); empty = same as remote
	upstreamRemote string // upstream as a named remote (for fetching); empty when upstream is a URL
	pushOwner      string // owner parsed from push remote (for cross-fork head prefix)
	dryRun         bool
	draft          bool
	existing       bool
	noStack        bool
	reviewers      []string
	revsets        []string
}

// changeState tracks the state of each change through the send pipeline.
type changeState struct {
	change   *jj.Change
	bookmark jj.ChangeBookmark
	pr       *gh.PRInfo // nil if no existing PR
	isNew    bool       // true if PR was just created
	changed  bool       // true if existing PR was modified (title, body, or interdiff)
}

// skipReason records why a change was skipped during send.
type skipReason struct {
	reason   string
	ancestor string // non-empty when skipped because an ancestor was skipped
}

func runSend(cmd *cobra.Command, args []string) error {
	base, _ := cmd.Flags().GetString("base")
	remote, _ := cmd.Flags().GetString("remote")
	upstream, _ := cmd.Flags().GetString("upstream")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	reviewers, _ := cmd.Flags().GetStringSlice("reviewer")
	// Trim whitespace from each reviewer (e.g. "-r alice, bob").
	var cleanReviewers []string
	for _, r := range reviewers {
		r = strings.TrimSpace(r)
		if r != "" {
			cleanReviewers = append(cleanReviewers, r)
		}
	}
	reviewers = cleanReviewers
	draft, _ := cmd.Flags().GetBool("draft")
	existing, _ := cmd.Flags().GetBool("existing")
	noStack, _ := cmd.Flags().GetBool("no-stack")
	w := cmd.OutOrStdout()

	revsets := args
	if len(revsets) == 0 {
		revsets = []string{"@-"}
	}

	// 1. Resolve auth.
	token, source := auth.ResolveToken(defaultHost)
	if token == "" {
		return fmt.Errorf("not authenticated — run 'jip auth login' or set GH_TOKEN")
	}
	_, _ = fmt.Fprintf(w, "Auth: %s\n", source)

	// 2. Detect repo from remote.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting cwd: %w", err)
	}
	runner := jj.NewRunner(cwd)

	remoteData, err := runner.GitRemoteList()
	if err != nil {
		return fmt.Errorf("listing remotes: %w", err)
	}
	remotes := jj.ParseRemoteList(remoteData)
	remoteURL, ok := remotes[remote]
	if !ok {
		return fmt.Errorf("remote %q not found (available: %v)", remote, remotes)
	}

	// Resolve upstream URL: if set, PRs target that repo; otherwise same as push remote.
	upstreamURL := remoteURL
	upstreamIsRemote := false // true when --upstream is a remote name (not a URL)
	if upstream != "" {
		if strings.Contains(upstream, "://") || strings.Contains(upstream, "@") {
			upstreamURL = upstream
		} else if u, ok := remotes[upstream]; ok {
			upstreamURL = u
			upstreamIsRemote = true
		} else {
			return fmt.Errorf("upstream remote %q not found (available: %v)", upstream, remotes)
		}
	}

	apiURL := os.Getenv("GITHUB_API_URL")
	client, err := gh.NewClient(token, upstreamURL, apiURL)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "Repo: %s/%s\n", client.Owner(), client.Repo())

	// For cross-fork PRs, parse the push remote owner to prefix the head ref.
	var pushOwner string
	if upstream != "" {
		pushOwner, _, err = gh.ParseRepoFromURL(remoteURL)
		if err != nil {
			return fmt.Errorf("parsing push remote URL: %w", err)
		}
	}

	var upstreamRemoteName string
	if upstreamIsRemote {
		upstreamRemoteName = upstream
	}

	return executeSend(runner, client, sendOpts{
		base:           base,
		remote:         remote,
		upstream:       upstream,
		upstreamRemote: upstreamRemoteName,
		pushOwner:      pushOwner,
		dryRun:         dryRun,
		draft:          draft,
		existing:       existing,
		noStack:        noStack,
		reviewers:      reviewers,
		revsets:        revsets,
	}, w)
}

// executeSend runs the core send algorithm: resolve stacks, ensure bookmarks,
// push branches, and create/update PRs.
func executeSend(runner jj.Runner, client gh.Service, opts sendOpts, w io.Writer) error {
	// Fetch from remote (and upstream if it's a named remote).
	_, _ = fmt.Fprintf(w, "Fetching %s...\n", opts.remote)
	if err := runner.GitFetch(opts.remote); err != nil {
		return fmt.Errorf("fetching %s: %w", opts.remote, err)
	}
	if opts.upstreamRemote != "" && opts.upstreamRemote != opts.remote {
		_, _ = fmt.Fprintf(w, "Fetching %s...\n", opts.upstreamRemote)
		if err := runner.GitFetch(opts.upstreamRemote); err != nil {
			return fmt.Errorf("fetching %s: %w", opts.upstreamRemote, err)
		}
	}

	repoFullName := client.Owner() + "/" + client.Repo()

	// 2. Resolve stacks.
	dags, err := jj.ResolveStacks(runner, opts.revsets, opts.base)
	if err != nil {
		return fmt.Errorf("resolving stacks: %w", err)
	}
	if len(dags) == 0 {
		_, _ = fmt.Fprintln(w, "No changes to send.")
		return nil
	}

	// If --no-stack, reduce each DAG to its tip (leaf) change only.
	if opts.noStack {
		for i, dag := range dags {
			leaves := dag.LeafChanges()
			if len(leaves) != 1 {
				return fmt.Errorf("--no-stack requires a linear stack (found %d tips in one DAG)", len(leaves))
			}
			tip := leaves[0]
			dags[i] = &jj.ChangeDAG{
				Changes: []*jj.Change{tip},
				ByID:    map[string]*jj.Change{tip.ChangeID: tip},
			}
		}
	}

	// 3. Get existing bookmarks.
	bookmarkData, err := runner.BookmarkList()
	if err != nil {
		return fmt.Errorf("listing bookmarks: %w", err)
	}
	bookmarks, err := jj.ParseBookmarkList(bookmarkData)
	if err != nil {
		return fmt.Errorf("parsing bookmarks: %w", err)
	}

	// Build lookup: collect all remote branches, query GitHub for existing PRs.
	bookmarkByName := make(map[string]*jj.BookmarkInfo, len(bookmarks))
	for i := range bookmarks {
		bookmarkByName[bookmarks[i].Name] = &bookmarks[i]
	}

	var remoteBranches []string
	remoteBranchSet := make(map[string]bool)
	for _, dag := range dags {
		for _, change := range dag.Changes {
			for _, bName := range change.Bookmarks {
				bi, ok := bookmarkByName[bName]
				if !ok {
					continue
				}
				if _, hasRemote := bi.Remotes[opts.remote]; hasRemote && !remoteBranchSet[bName] {
					remoteBranches = append(remoteBranches, bName)
					remoteBranchSet[bName] = true
				}
			}
		}
	}

	var prMap map[string]*gh.PRInfo
	if len(remoteBranches) > 0 {
		prMap, err = client.LookupPRsByBranch(remoteBranches)
		if err != nil {
			return fmt.Errorf("looking up PRs: %w", err)
		}
	} else {
		prMap = make(map[string]*gh.PRInfo)
	}

	// 4. Process each DAG.
	var allStates []changeState

	for _, dag := range dags {
		// shouldUseExisting: prefer bookmarks that already have a PR, then any jip/ bookmark.
		shouldUse := func(changeID, bookmark string) bool {
			if _, hasPR := prMap[bookmark]; hasPR {
				return true
			}
			return strings.HasPrefix(bookmark, "jip/")
		}

		results, err := jj.EnsureBookmarks(runner, dag, bookmarks, opts.remote, shouldUse, !opts.existing)
		if err != nil {
			return fmt.Errorf("ensuring bookmarks: %w", err)
		}

		// Map change ID -> bookmark result.
		bmByChange := make(map[string]jj.ChangeBookmark, len(results))
		for _, r := range results {
			bmByChange[r.ChangeID] = r
		}

		for _, change := range dag.Changes {
			bm := bmByChange[change.ChangeID]
			existingPR := prMap[bm.Bookmark]
			allStates = append(allStates, changeState{
				change:   change,
				bookmark: bm,
				pr:       existingPR,
			})
		}
	}

	// Filter to existing PRs only when --existing is set.
	if opts.existing {
		var filtered []changeState
		for _, s := range allStates {
			if s.pr != nil {
				filtered = append(filtered, s)
			}
		}
		skipped := len(allStates) - len(filtered)
		if skipped > 0 {
			_, _ = fmt.Fprintf(w, "\nSkipping %d change(s) without existing PRs.\n", skipped)
		}
		allStates = filtered
		if len(allStates) == 0 {
			_, _ = fmt.Fprintln(w, "No existing PRs to update.")
			return nil
		}
	}

	// Detect diverged/behind bookmarks and skip them (plus descendants).
	skippedIDs := make(map[string]skipReason)

	for _, s := range allStates {
		// Check if any parent was skipped.
		for _, pid := range s.change.ParentIDs {
			if _, ok := skippedIDs[pid]; ok {
				skippedIDs[s.change.ChangeID] = skipReason{
					reason:   "skipped because ancestor was skipped",
					ancestor: pid,
				}
				break
			}
		}
		if _, ok := skippedIDs[s.change.ChangeID]; ok {
			continue // already marked via ancestor
		}
		if s.bookmark.Displaced {
			skippedIDs[s.change.ChangeID] = skipReason{
				reason: "remote is ahead of local — pull changes or reset the bookmark",
			}
		} else if s.bookmark.Conflict {
			skippedIDs[s.change.ChangeID] = skipReason{
				reason: "local and remote have diverged — resolve with `jj bookmark set` or force-push",
			}
		} else if s.bookmark.SyncState == jj.SyncBehind {
			skippedIDs[s.change.ChangeID] = skipReason{
				reason: "remote is ahead of local — pull changes first",
			}
		}
	}

	var activeStates, skippedStates []changeState
	for _, s := range allStates {
		if _, ok := skippedIDs[s.change.ChangeID]; ok {
			skippedStates = append(skippedStates, s)
		} else {
			activeStates = append(activeStates, s)
		}
	}

	if opts.dryRun {
		_, _ = fmt.Fprintf(w, "\nDry run — %d change(s) would be sent:\n\n", len(activeStates))
		for _, s := range activeStates {
			action := "CREATE"
			if s.pr != nil {
				action = fmt.Sprintf("UPDATE #%d", s.pr.Number)
			}
			bmStatus := "new"
			if !s.bookmark.IsNew {
				bmStatus = "existing"
			}
			_, _ = fmt.Fprintf(w, "  %s  %.12s  %s\n", action, s.change.ChangeID, s.change.Description)
			_, _ = fmt.Fprintf(w, "         bookmark: %s (%s)\n", s.bookmark.Bookmark, bmStatus)
		}
		if len(skippedStates) > 0 {
			printSkippedChanges(w, skippedStates, skippedIDs)
		}
		if len(skippedStates) > 0 {
			return fmt.Errorf("%d change(s) skipped due to diverged or behind bookmarks", len(skippedStates))
		}
		return nil
	}

	if len(activeStates) > 0 {
		// 5. Push all bookmarks.
		var pushBookmarks []string
		for _, s := range activeStates {
			pushBookmarks = append(pushBookmarks, s.bookmark.Bookmark)
		}
		_, _ = fmt.Fprintf(w, "\nPushing %d bookmark(s)...\n", len(pushBookmarks))
		if err := runner.GitPush(pushBookmarks, true, opts.remote); err != nil {
			return fmt.Errorf("pushing: %w", err)
		}

		// 6. Create/update PRs.
		for i := range activeStates {
			s := &activeStates[i]
			if s.pr != nil {
				// Existing PR — update title if changed, post interdiff comment.
				if s.pr.Title != s.change.Description {
					title := s.change.Description
					if err := client.UpdatePR(s.pr.Number, gh.UpdatePROpts{Title: &title}); err != nil {
						return fmt.Errorf("updating PR #%d title: %w", s.pr.Number, err)
					}
					s.changed = true
				}

				// Post interdiff comment: compare the old remote commit to the new local commit.
				bi := bookmarkByName[s.bookmark.Bookmark]
				if bi != nil {
					if rs, ok := bi.Remotes[opts.remote]; ok && rs.Target != "" && rs.Target != s.change.CommitID {
						diff, err := runner.Interdiff(rs.Target, s.change.CommitID)
						if err != nil {
							_, _ = fmt.Fprintf(w, "  warning: interdiff failed for #%d: %v\n", s.pr.Number, err)
						} else {
							comment := gh.BuildDiffComment(diff, repoFullName, opts.base, rs.Target, s.change.CommitID)
							if err := client.CommentOnPR(s.pr.Number, comment); err != nil {
								return fmt.Errorf("commenting on PR #%d: %w", s.pr.Number, err)
							}
							s.changed = true
						}
					}
				}
			} else {
				// New PR — create it.
				title := s.change.Description
				if title == "" {
					title = fmt.Sprintf("jip: %.12s", s.change.ChangeID)
				}
				head := s.bookmark.Bookmark
				if opts.pushOwner != "" {
					head = opts.pushOwner + ":" + head
				}
				pr, err := client.CreatePR(head, opts.base, title, "", opts.draft)
				if err != nil {
					return fmt.Errorf("creating PR for %s: %w", s.change.ChangeID, err)
				}
				s.pr = pr
				s.isNew = true

				if len(opts.reviewers) > 0 {
					if err := client.RequestReviewers(pr.Number, opts.reviewers); err != nil {
						_, _ = fmt.Fprintf(w, "  warning: failed to add reviewers to #%d: %v\n", pr.Number, err)
					}
				}
			}
		}

		// 7. Update all PR bodies with stack navigation (skip when --no-stack).
		if !opts.noStack {
			// Each PR's stack only includes its ancestors and descendants (its
			// dependency chain), not unrelated branches in the same DAG.
			perChangeStack := computeStackPRs(activeStates)

			for i, s := range activeStates {
				body := gh.BuildStackedPRBody(
					s.change.CommitID,
					repoFullName,
					s.pr.Number,
					perChangeStack[i],
					"", // commit body — we only have the first line in Description
				)
				if body != s.pr.Body {
					if err := client.UpdatePR(s.pr.Number, gh.UpdatePROpts{Body: &body}); err != nil {
						return fmt.Errorf("updating PR #%d body: %w", s.pr.Number, err)
					}
					activeStates[i].changed = true
				}
			}
		}

		// 8. Print summary.
		_, _ = fmt.Fprintf(w, "\n%d PR(s) sent:\n\n", len(activeStates))
		for _, s := range activeStates {
			action := "updated"
			if s.isNew {
				action = "created"
			} else if !s.changed {
				action = "up-to-date"
			}
			_, _ = fmt.Fprintf(w, "  #%-4d %s  %s\n", s.pr.Number, action, s.pr.URL)
			_, _ = fmt.Fprintf(w, "         %.12s  %s\n", s.change.ChangeID, s.change.Description)
		}
	}

	if len(skippedStates) > 0 {
		printSkippedChanges(w, skippedStates, skippedIDs)
		return fmt.Errorf("%d change(s) skipped due to diverged or behind bookmarks", len(skippedStates))
	}
	return nil
}

// computeStackPRs computes per-change stack PR number lists. Each change's
// stack includes only its ancestors and descendants (the dependency chain),
// not unrelated branches in the same DAG. PR numbers are returned in the
// same topological order as the input states.
func computeStackPRs(states []changeState) [][]int {
	idxByChange := make(map[string]int, len(states))
	for i, s := range states {
		idxByChange[s.change.ChangeID] = i
	}

	// Build child edges (parent → children) within the known set.
	children := make(map[string][]string)
	for _, s := range states {
		for _, pid := range s.change.ParentIDs {
			if _, ok := idxByChange[pid]; ok {
				children[pid] = append(children[pid], s.change.ChangeID)
			}
		}
	}

	result := make([][]int, len(states))
	for i, s := range states {
		relevant := map[string]bool{s.change.ChangeID: true}

		// Walk ancestors (follow parent edges).
		var walkUp func(string)
		walkUp = func(id string) {
			for _, pid := range states[idxByChange[id]].change.ParentIDs {
				if _, ok := idxByChange[pid]; ok && !relevant[pid] {
					relevant[pid] = true
					walkUp(pid)
				}
			}
		}
		walkUp(s.change.ChangeID)

		// Walk descendants (follow child edges).
		var walkDown func(string)
		walkDown = func(id string) {
			for _, cid := range children[id] {
				if !relevant[cid] {
					relevant[cid] = true
					walkDown(cid)
				}
			}
		}
		walkDown(s.change.ChangeID)

		// Collect PR numbers preserving topological order.
		var prs []int
		for _, st := range states {
			if relevant[st.change.ChangeID] {
				prs = append(prs, st.pr.Number)
			}
		}
		result[i] = prs
	}
	return result
}

// printSkippedChanges reports changes that were skipped due to diverged/behind bookmarks.
func printSkippedChanges(w io.Writer, skipped []changeState, reasons map[string]skipReason) {
	_, _ = fmt.Fprintf(w, "\nSkipped %d change(s):\n\n", len(skipped))
	for _, s := range skipped {
		r := reasons[s.change.ChangeID]
		_, _ = fmt.Fprintf(w, "  %.12s  %s\n", s.change.ChangeID, s.change.Description)
		_, _ = fmt.Fprintf(w, "         %s\n", r.reason)
	}
}
