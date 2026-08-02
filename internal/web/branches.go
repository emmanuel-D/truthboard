// Branch cleanup is the second deliberate exception to this server's
// read-only doctrine (the sync loop is the first), and the only one that
// deletes anything. It exists because the board already derives the
// finding: a unit reported done is a branch whose commits are in the
// integration branch, and "landed but not deleted" has been on the drift
// report since the beginning with nothing to do about it.
//
// Everything here is arranged so the answer stays honest. The audit decides
// what is safe — this file only asks it and acts. An integration branch is
// never deletable, a checked-out branch is never deletable, and a branch
// carrying work git cannot find anywhere else is refused until someone says
// force. What actually got deleted is reported ref by ref: half a deletion
// reported as a success would be exactly the kind of lie this tool exists
// to catch.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

// branchResult says which refs went and which did not. Both lists, always:
// "deleted" alone cannot express a push that origin refused.
type branchResult struct {
	Repo    string   `json:"repo,omitempty"`
	Branch  string   `json:"branch"`
	Deleted []string `json:"deleted"`
	Failed  []string `json:"failed,omitempty"`
	Merged  bool     `json:"merged"`
}

// branchDelete retires one branch: its local ref, its ref on origin, or
// both. Gated exactly like an intent write by the guard in Handler — on a
// shared board that means the edit token, and on a read-only one it means
// no.
func branchDelete(hub string, changed func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "branches are read from /api/board; this route only deletes", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		name := q.Get("name")
		if strings.TrimSpace(name) == "" {
			http.Error(w, "which branch? pass ?name=<branch>", http.StatusBadRequest)
			return
		}
		t, err := audit.FindBranch(hub, q.Get("repo"), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// The integration branch is where every status comes from. No
		// force flag, no confirmation, no route: it is not deletable.
		if t.IsIntegration {
			http.Error(w, fmt.Sprintf("%s is the integration branch — every status on this board is derived from it, so it is not deletable here",
				t.Label()), http.StatusBadRequest)
			return
		}

		wantLocal := q.Get("local") == "1" && t.Local
		wantRemote := q.Get("remote") == "1" && t.Remote
		if !wantLocal && !wantRemote {
			http.Error(w, fmt.Sprintf("nothing to delete: %s has %s — say ?local=1 and/or ?remote=1 for refs that exist",
				t.Label(), refsOf(t)), http.StatusBadRequest)
			return
		}
		if wantLocal && t.CheckedOut {
			http.Error(w, fmt.Sprintf("%s is checked out in the repository the board reads — git will not delete the branch it is standing on; switch away first, or delete only the ref on origin",
				t.Label()), http.StatusConflict)
			return
		}
		// Unmerged work is the whole reason this needs a guard: once both
		// refs are gone those commits are reachable only from a reflog
		// nobody will think to read.
		if !t.Merged && q.Get("force") != "1" {
			http.Error(w, fmt.Sprintf("%s is not merged into %s — %s. Deleting it drops that work; retry with ?force=1 to delete it anyway.",
				t.Label(), t.Integration, t.Evidence), http.StatusConflict)
			return
		}

		res := branchResult{Repo: t.Repo, Branch: t.Name, Merged: t.Merged}
		if wantRemote {
			// Origin first: it is the ref most likely to refuse (no push
			// rights, a protected branch), and failing before the local
			// ref is touched leaves the repository exactly as it was.
			if _, err := gitMutate(t.Path, "push", "--quiet", "origin", "--delete", t.Name); err != nil {
				res.Failed = append(res.Failed, "origin: "+oneLine(err.Error()))
			} else {
				res.Deleted = append(res.Deleted, "origin")
				if t.Bare {
					// A mirror's refs/heads is its copy of origin, and
					// push --delete does not prune it. Drop it here so the
					// very next audit stops reporting a branch that no
					// longer exists anywhere.
					if _, err := gitMutate(t.Path, "update-ref", "-d", "refs/heads/"+t.Name); err != nil {
						res.Failed = append(res.Failed, "mirror copy: "+oneLine(err.Error()))
					}
				}
			}
		}
		if wantLocal && len(res.Failed) == 0 {
			// -D, not -d: whether the work is safe was decided above, on
			// the audit's evidence — git's own check does not know about
			// squash merges, and refusing there would contradict a board
			// that reports the branch as landed.
			if _, err := gitMutate(t.Path, "branch", "-D", t.Name); err != nil {
				res.Failed = append(res.Failed, "local: "+oneLine(err.Error()))
			} else {
				res.Deleted = append(res.Deleted, "local")
			}
		}

		if len(res.Deleted) == 0 {
			http.Error(w, fmt.Sprintf("could not delete %s — %s", t.Label(), strings.Join(res.Failed, "; ")), http.StatusBadGateway)
			return
		}
		changed()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	}
}

// refsOf describes what a branch actually has, for the message that fires
// when a request asks to delete a ref that is not there.
func refsOf(t *audit.BranchTarget) string {
	switch {
	case t.Local && t.Remote:
		return "a local ref and one on origin"
	case t.Local:
		return "only a local ref"
	case t.Remote:
		return "only a ref on origin"
	}
	return "no refs left"
}
