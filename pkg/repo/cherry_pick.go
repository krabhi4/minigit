package repo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"minigit/pkg/object"
)

func (r *Repository) CherryPick(targetRev string, mainline ...int) (string, error) {
	st, err := r.Status()
	if err != nil {
		return "", err
	}
	if len(st.Unmerged) > 0 {
		return "", errors.New("fatal: cherry-pick is not possible because you have unmerged files.")
	}

	pickHash, err := r.ResolveRevision(targetRev)
	if err != nil {
		return "", err
	}

	pickCommit, err := r.LoadCommit(pickHash)
	if err != nil {
		return "", err
	}

	mainlineParent := 0
	if len(mainline) > 0 {
		mainlineParent = mainline[0]
	}
	if len(pickCommit.ParentHashes) > 1 && mainlineParent <= 0 {
		return "", fmt.Errorf("error: commit %s is a merge but no -m option was given.\nfatal: cherry-pick failed", pickHash[:7])
	}
	if mainlineParent > 0 {
		if mainlineParent > len(pickCommit.ParentHashes) {
			return "", fmt.Errorf("fatal: commit %s does not have parent %d", pickHash[:7], mainlineParent)
		}
	}

	currentBranch, isSymbolic, headHash, err := r.ResolveHEAD()
	if err != nil {
		return "", err
	}
	if headHash == "" {
		return "", errors.New("cannot cherry-pick into a branch without commits")
	}

	headCommit, err := r.LoadCommit(headHash)
	if err != nil {
		return "", err
	}

	var baseFiles map[string]FlattenedFile
	if len(pickCommit.ParentHashes) > 0 {
		baseParentIdx := 0
		if mainlineParent > 0 {
			baseParentIdx = mainlineParent - 1
		}
		baseCommit, err := r.LoadCommit(pickCommit.ParentHashes[baseParentIdx])
		if err != nil {
			return "", err
		}
		baseFiles, err = r.FlattenTree(baseCommit.TreeHash, "")
		if err != nil {
			return "", err
		}
	} else {
		baseFiles = make(map[string]FlattenedFile)
	}

	ourFiles, err := r.FlattenTree(headCommit.TreeHash, "")
	if err != nil {
		return "", err
	}

	theirFiles, err := r.FlattenTree(pickCommit.TreeHash, "")
	if err != nil {
		return "", err
	}

	incomingChanges := make(map[string]bool)
	for p, fTheirs := range theirFiles {
		fBase, inBase := baseFiles[p]
		if !inBase || fBase.Hash != fTheirs.Hash {
			incomingChanges[p] = true
		}
	}
	for p := range baseFiles {
		if _, inTheirs := theirFiles[p]; !inTheirs {
			incomingChanges[p] = true
		}
	}
	if err := r.checkWorktreeDirty(incomingChanges, "cherry-pick"); err != nil {
		return "", err
	}

	subject := strings.SplitN(pickCommit.Message, "\n", 2)[0]
	labelTheirs := fmt.Sprintf("%s... %s", pickHash[:7], subject)

	idx, hasConflict, err := r.mergeTrees3Way(baseFiles, ourFiles, theirFiles, labelTheirs)
	if err != nil {
		return "", err
	}

	if hasConflict {
		_ = os.WriteFile(filepath.Join(r.GitDir, "CHERRY_PICK_HEAD"), []byte(pickHash+"\n"), 0644)
		_ = os.WriteFile(filepath.Join(r.GitDir, "MERGE_MSG"), []byte(pickCommit.Message), 0644)
		_ = r.SaveIndex(idx)
		return "", fmt.Errorf("error: could not apply %s... %s\nhint: after resolving the conflicts, mark the corrected paths\nhint: with 'minigit add <paths>' and commit the result with 'minigit commit'", pickHash[:7], subject)
	}

	if err := r.SaveIndex(idx); err != nil {
		return "", err
	}

	treeHash, err := idx.BuildTree(r.GitDir)
	if err != nil {
		return "", err
	}

	if treeHash == headCommit.TreeHash {
		return "", errors.New("The previous cherry-pick is now empty, possibly due to conflict resolution.")
	}

	committerStr := r.defaultAuthor()
	now := time.Now()

	newCommit := &object.Commit{
		TreeHash:      treeHash,
		ParentHashes:  []string{headHash},
		Author:        pickCommit.Author,
		AuthorTime:    pickCommit.AuthorTime,
		Committer:     committerStr,
		CommitterTime: now,
		Message:       pickCommit.Message,
	}

	commitHash, err := object.WriteObject(r.GitDir, object.TypeCommit, newCommit.Serialize())
	if err != nil {
		return "", err
	}

	if isSymbolic {
		if err := r.UpdateBranch(currentBranch, commitHash); err != nil {
			return "", err
		}
	} else {
		if err := r.SetHEADDetached(commitHash); err != nil {
			return "", err
		}
	}

	_ = os.Remove(filepath.Join(r.GitDir, "CHERRY_PICK_HEAD"))
	_ = os.Remove(filepath.Join(r.GitDir, "MERGE_MSG"))

	return fmt.Sprintf("[%s %s] %s", currentBranch, commitHash[:7], subject), nil
}

func (r *Repository) CherryPickAbort() error {
	cpHeadPath := filepath.Join(r.GitDir, "CHERRY_PICK_HEAD")
	if _, err := os.Stat(cpHeadPath); os.IsNotExist(err) {
		return errors.New("fatal: no cherry-pick or revert in progress")
	}

	_, _, headHash, err := r.ResolveHEAD()
	if err != nil {
		return err
	}
	if headHash == "" {
		return errors.New("fatal: HEAD is invalid")
	}

	headCommit, err := r.LoadCommit(headHash)
	if err != nil {
		return err
	}

	if err := r.resetTree(headCommit.TreeHash, true); err != nil {
		return err
	}

	_ = os.Remove(cpHeadPath)
	_ = os.Remove(filepath.Join(r.GitDir, "MERGE_MSG"))
	return nil
}

func (r *Repository) CherryPickContinue() (string, error) {
	cpHeadPath := filepath.Join(r.GitDir, "CHERRY_PICK_HEAD")
	cpData, err := os.ReadFile(cpHeadPath)
	if err != nil {
		return "", errors.New("fatal: no cherry-pick or revert in progress")
	}
	pickHash := strings.TrimSpace(string(cpData))

	st, err := r.Status()
	if err != nil {
		return "", err
	}
	if len(st.Unmerged) > 0 {
		return "", errors.New("error: Committing is not possible because you have unmerged files.\nfatal: Exiting because of an unresolved conflict.")
	}

	pickCommit, err := r.LoadCommit(pickHash)
	if err != nil {
		return "", err
	}

	mergeMsgPath := filepath.Join(r.GitDir, "MERGE_MSG")
	msgData, err := os.ReadFile(mergeMsgPath)
	commitMsg := pickCommit.Message
	if err == nil && len(strings.TrimSpace(string(msgData))) > 0 {
		commitMsg = string(msgData)
	}

	commitHash, err := r.Commit(commitMsg, pickCommit.Author)
	if err != nil {
		return "", err
	}

	branch, _, _, _ := r.ResolveHEAD()
	if branch == "" {
		branch = "HEAD"
	}
	firstLine := strings.SplitN(commitMsg, "\n", 2)[0]
	return fmt.Sprintf("[%s %s] %s", branch, commitHash[:7], firstLine), nil
}
