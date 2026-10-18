package repo

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"minigit/pkg/index"
	"minigit/pkg/object"
)

func (r *Repository) StashPush(customMsg string) (string, error) {
	st, err := r.Status()
	if err != nil {
		return "", err
	}

	if len(st.Unmerged) > 0 {
		return "", errors.New("cannot stash changes: you have unmerged files.")
	}

	if len(st.Staged) == 0 && len(st.Unstaged) == 0 {
		return "No local changes to save", nil
	}

	branch, _, headHash, err := r.ResolveHEAD()
	if err != nil {
		return "", err
	}
	if headHash == "" {
		return "", errors.New("You do not have the initial commit yet")
	}

	headCommit, err := r.LoadCommit(headHash)
	if err != nil {
		return "", err
	}
	headSubject := strings.SplitN(headCommit.Message, "\n", 2)[0]

	if branch == "" {
		branch = "HEAD"
	}

	idx, err := r.LoadIndex()
	if err != nil {
		return "", err
	}

	indexTreeHash, err := idx.BuildTree(r.GitDir)
	if err != nil {
		return "", err
	}

	authorStr := r.defaultAuthor()
	now := time.Now()

	indexCommitMsg := fmt.Sprintf("index on %s: %s %s", branch, headHash[:7], headSubject)
	indexCommitObj := &object.Commit{
		TreeHash:      indexTreeHash,
		ParentHashes:  []string{headHash},
		Author:        authorStr,
		AuthorTime:    now,
		Committer:     authorStr,
		CommitterTime: now,
		Message:       indexCommitMsg,
	}

	indexCommitHash, err := object.WriteObject(r.GitDir, object.TypeCommit, indexCommitObj.Serialize())
	if err != nil {
		return "", err
	}

	workIdx := &index.Index{}
	workIdx.Entries = append(workIdx.Entries, idx.Entries...)

	for _, u := range st.Unstaged {
		absPath := filepath.Join(r.WorkDir, filepath.FromSlash(u.Path))
		if u.Status == StatusModified {
			fi, err := os.Lstat(absPath)
			if err != nil {
				return "", err
			}
			var data []byte
			modeUint := uint32(0100644)
			if fi.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(absPath)
				if err != nil {
					return "", err
				}
				data = []byte(target)
				modeUint = 0120000
			} else {
				data, err = os.ReadFile(absPath)
				if err != nil {
					return "", err
				}
				if fi.Mode()&0111 != 0 {
					modeUint = 0100755
				}
			}
			blobHash, err := object.WriteObject(r.GitDir, object.TypeBlob, data)
			if err != nil {
				return "", err
			}
			hBytes, err := hex.DecodeString(blobHash)
			if err != nil {
				return "", err
			}
			var sha [20]byte
			copy(sha[:], hBytes)

			workIdx.AddOrUpdate(index.IndexEntry{
				Mode:     modeUint,
				FileSize: uint32(len(data)),
				Sha1:     sha,
				Path:     u.Path,
			})
		} else if u.Status == StatusDeleted {
			workIdx.Remove(u.Path)
		}
	}

	workTreeHash, err := workIdx.BuildTree(r.GitDir)
	if err != nil {
		return "", err
	}

	var stashMsg string
	if customMsg != "" {
		stashMsg = fmt.Sprintf("On %s: %s", branch, customMsg)
	} else {
		stashMsg = fmt.Sprintf("WIP on %s: %s %s", branch, headHash[:7], headSubject)
	}

	stashCommitObj := &object.Commit{
		TreeHash:      workTreeHash,
		ParentHashes:  []string{headHash, indexCommitHash},
		Author:        authorStr,
		AuthorTime:    now,
		Committer:     authorStr,
		CommitterTime: now,
		Message:       stashMsg,
	}

	stashCommitHash, err := object.WriteObject(r.GitDir, object.TypeCommit, stashCommitObj.Serialize())
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Join(r.GitDir, "refs"), 0755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(r.GitDir, "logs", "refs"), 0755); err != nil {
		return "", err
	}

	stashRefFile := filepath.Join(r.GitDir, "refs", "stash")
	oldHash := "0000000000000000000000000000000000000000"
	if data, err := os.ReadFile(stashRefFile); err == nil {
		trimmed := strings.TrimSpace(string(data))
		if len(trimmed) == 40 {
			oldHash = trimmed
		}
	}

	logFile := filepath.Join(r.GitDir, "logs", "refs", "stash")
	_, offset := now.Zone()
	tzStr := object.FormatTimezone(offset)
	logLine := fmt.Sprintf("%s %s %s %d %s\t%s\n", oldHash, stashCommitHash, authorStr, now.Unix(), tzStr, stashMsg)

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(logLine); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	if err := os.WriteFile(stashRefFile, []byte(stashCommitHash+"\n"), 0644); err != nil {
		return "", err
	}

	if err := r.resetTree(headCommit.TreeHash, true); err != nil {
		return "", err
	}

	return fmt.Sprintf("Saved working directory and index state %s", stashMsg), nil
}

func (r *Repository) StashList() ([]string, error) {
	logFile := filepath.Join(r.GitDir, "logs", "refs", "stash")
	data, err := os.ReadFile(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			stashRefFile := filepath.Join(r.GitDir, "refs", "stash")
			if refData, err := os.ReadFile(stashRefFile); err == nil {
				h := strings.TrimSpace(string(refData))
				if c, err := r.LoadCommit(h); err == nil {
					return []string{fmt.Sprintf("stash@{0}: %s", strings.TrimSpace(c.Message))}, nil
				}
			}
			return nil, nil
		}
		return nil, err
	}

	lines := parseNonEmptyLines(string(data))
	if len(lines) == 0 {
		return nil, nil
	}

	var results []string
	for i := 0; i < len(lines); i++ {
		line := lines[len(lines)-1-i]
		tabIdx := strings.IndexByte(line, '\t')
		if tabIdx != -1 {
			msg := line[tabIdx+1:]
			results = append(results, fmt.Sprintf("stash@{%d}: %s", i, msg))
		}
	}
	return results, nil
}

func (r *Repository) StashApply(spec string, restoreIndex ...bool) (string, error) {
	doRestore := len(restoreIndex) > 0 && restoreIndex[0]
	return r.applyStashInternal(spec, false, doRestore)
}

func (r *Repository) StashPop(spec string, restoreIndex ...bool) (string, error) {
	doRestore := len(restoreIndex) > 0 && restoreIndex[0]
	return r.applyStashInternal(spec, true, doRestore)
}

func (r *Repository) applyStashInternal(spec string, dropAfter bool, restoreIndex bool) (string, error) {
	n, err := parseStashIndex(spec)
	if err != nil {
		return "", err
	}

	logFile := filepath.Join(r.GitDir, "logs", "refs", "stash")
	stashRefFile := filepath.Join(r.GitDir, "refs", "stash")

	stashHash := ""
	data, err := os.ReadFile(logFile)
	if err != nil {
		if os.IsNotExist(err) && n == 0 {
			if refData, err := os.ReadFile(stashRefFile); err == nil {
				stashHash = strings.TrimSpace(string(refData))
			}
		}
	} else {
		lines := parseNonEmptyLines(string(data))
		if n >= 0 && n < len(lines) {
			targetLine := lines[len(lines)-1-n]
			fields := strings.Fields(targetLine)
			if len(fields) >= 2 {
				stashHash = fields[1]
			}
		}
	}

	if stashHash == "" {
		return "", fmt.Errorf("error: stash@{%d} does not exist", n)
	}

	stashCommit, err := r.LoadCommit(stashHash)
	if err != nil {
		return "", err
	}
	if len(stashCommit.ParentHashes) < 1 {
		return "", fmt.Errorf("corrupt stash commit %s", stashHash)
	}

	headAtStash := stashCommit.ParentHashes[0]
	baseCommit, err := r.LoadCommit(headAtStash)
	if err != nil {
		return "", err
	}

	baseFiles, err := r.FlattenTree(baseCommit.TreeHash, "")
	if err != nil {
		return "", err
	}

	stashFiles, err := r.FlattenTree(stashCommit.TreeHash, "")
	if err != nil {
		return "", err
	}

	_, _, currentHeadHash, err := r.ResolveHEAD()
	if err != nil {
		return "", err
	}
	if currentHeadHash == "" {
		return "", errors.New("cannot apply stash without commits")
	}

	currentCommit, err := r.LoadCommit(currentHeadHash)
	if err != nil {
		return "", err
	}

	ourFiles, err := r.FlattenTree(currentCommit.TreeHash, "")
	if err != nil {
		return "", err
	}

	incomingChanges := make(map[string]bool)
	for p, fStash := range stashFiles {
		fBase, inBase := baseFiles[p]
		if !inBase || fBase.Hash != fStash.Hash {
			incomingChanges[p] = true
		}
	}
	for p := range baseFiles {
		if _, inStash := stashFiles[p]; !inStash {
			incomingChanges[p] = true
		}
	}
	if err := r.checkWorktreeDirty(incomingChanges, "merge"); err != nil {
		return "", fmt.Errorf("%s\nThe stash entry is kept in case you need it again.", err.Error())
	}

	idx, hasConflict, err := r.mergeTrees3Way(baseFiles, ourFiles, stashFiles, "Updated upstream")
	if err != nil {
		return "", err
	}

	if hasConflict {
		_ = r.SaveIndex(idx)
		return "", errors.New("CONFLICT (content): Merge conflict\nThe stash entry is kept in case you need it again.")
	}

	if restoreIndex && len(stashCommit.ParentHashes) > 1 {
		indexCommit, err := r.LoadCommit(stashCommit.ParentHashes[1])
		if err == nil {
			indexFiles, err := r.FlattenTree(indexCommit.TreeHash, "")
			if err == nil {
				finalIdx := &index.Index{}
				for p, f := range ourFiles {
					hBytes, _ := hex.DecodeString(f.Hash)
					var sha [20]byte
					copy(sha[:], hBytes)
					m := uint32(0100644)
					if f.Mode == "100755" || f.Mode == "0100755" {
						m = 0100755
					} else if f.Mode == "120000" || f.Mode == "0120000" {
						m = 0120000
					}
					finalIdx.AddOrUpdate(index.IndexEntry{Mode: m, Sha1: sha, Path: p})
				}
				for p, f := range indexFiles {
					fBase, inBase := baseFiles[p]
					if !inBase || fBase.Hash != f.Hash {
						hBytes, _ := hex.DecodeString(f.Hash)
						var sha [20]byte
						copy(sha[:], hBytes)
						m := uint32(0100644)
						if f.Mode == "100755" || f.Mode == "0100755" {
							m = 0100755
						} else if f.Mode == "120000" || f.Mode == "0120000" {
							m = 0120000
						}
						finalIdx.AddOrUpdate(index.IndexEntry{Mode: m, Sha1: sha, Path: p})
					}
				}
				for p := range baseFiles {
					if _, inIndex := indexFiles[p]; !inIndex {
						finalIdx.Remove(p)
					}
				}
				if err := r.SaveIndex(finalIdx); err != nil {
					return "", err
				}
			}
		}
	} else {
		finalIdx := &index.Index{}
		for p, f := range ourFiles {
			hBytes, _ := hex.DecodeString(f.Hash)
			var sha [20]byte
			copy(sha[:], hBytes)
			m := uint32(0100644)
			if f.Mode == "100755" || f.Mode == "0100755" {
				m = 0100755
			} else if f.Mode == "120000" || f.Mode == "0120000" {
				m = 0120000
			}
			finalIdx.AddOrUpdate(index.IndexEntry{Mode: m, Sha1: sha, Path: p})
		}
		for p, f := range stashFiles {
			if _, inOurs := ourFiles[p]; !inOurs {
				hBytes, _ := hex.DecodeString(f.Hash)
				var sha [20]byte
				copy(sha[:], hBytes)
				m := uint32(0100644)
				if f.Mode == "100755" || f.Mode == "0100755" {
					m = 0100755
				} else if f.Mode == "120000" || f.Mode == "0120000" {
					m = 0120000
				}
				finalIdx.AddOrUpdate(index.IndexEntry{Mode: m, Sha1: sha, Path: p})
			}
		}
		if err := r.SaveIndex(finalIdx); err != nil {
			return "", err
		}
	}

	if dropAfter {
		dropMsg, dropErr := r.StashDrop(fmt.Sprintf("stash@{%d}", n))
		if dropErr != nil {
			return "", dropErr
		}
		return dropMsg, nil
	}

	return "", nil
}

func (r *Repository) StashDrop(spec string) (string, error) {
	n, err := parseStashIndex(spec)
	if err != nil {
		return "", err
	}

	logFile := filepath.Join(r.GitDir, "logs", "refs", "stash")
	stashRefFile := filepath.Join(r.GitDir, "refs", "stash")

	data, err := os.ReadFile(logFile)
	if err != nil {
		if os.IsNotExist(err) && n == 0 {
			if refData, err := os.ReadFile(stashRefFile); err == nil {
				h := strings.TrimSpace(string(refData))
				_ = os.Remove(stashRefFile)
				return fmt.Sprintf("Dropped stash@{0} (%s)", h), nil
			}
		}
		return "", fmt.Errorf("error: stash@{%d} does not exist", n)
	}

	lines := parseNonEmptyLines(string(data))
	if n < 0 || n >= len(lines) {
		return "", fmt.Errorf("error: stash@{%d} does not exist", n)
	}

	dropIdx := len(lines) - 1 - n
	targetLine := lines[dropIdx]
	fields := strings.Fields(targetLine)
	droppedHash := ""
	if len(fields) >= 2 {
		droppedHash = fields[1]
	}

	var newLines []string
	for i, l := range lines {
		if i != dropIdx {
			newLines = append(newLines, l)
		}
	}

	if len(newLines) == 0 {
		_ = os.Remove(stashRefFile)
		_ = os.Remove(logFile)
	} else {
		if err := os.WriteFile(logFile, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
			return "", err
		}
		lastFields := strings.Fields(newLines[len(newLines)-1])
		if len(lastFields) >= 2 {
			_ = os.WriteFile(stashRefFile, []byte(lastFields[1]+"\n"), 0644)
		}
	}

	return fmt.Sprintf("Dropped stash@{%d} (%s)", n, droppedHash), nil
}

func (r *Repository) StashClear() error {
	_ = os.Remove(filepath.Join(r.GitDir, "refs", "stash"))
	_ = os.Remove(filepath.Join(r.GitDir, "logs", "refs", "stash"))
	return nil
}

func parseStashIndex(spec string) (int, error) {
	spec = strings.TrimSpace(spec)
	spec = strings.TrimPrefix(spec, "refs/")
	if spec == "" || spec == "stash" {
		return 0, nil
	}
	if strings.HasPrefix(spec, "stash@{") && strings.HasSuffix(spec, "}") {
		nStr := spec[len("stash@{") : len(spec)-1]
		n, err := strconv.Atoi(nStr)
		if err != nil {
			return 0, fmt.Errorf("'%s' is not a valid-stash-or-commit-reference", spec)
		}
		return n, nil
	}
	n, err := strconv.Atoi(spec)
	if err == nil {
		return n, nil
	}
	return 0, fmt.Errorf("'%s' is not a valid-stash-or-commit-reference", spec)
}

func parseNonEmptyLines(s string) []string {
	raw := strings.Split(s, "\n")
	var res []string
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l != "" {
			res = append(res, l)
		}
	}
	return res
}
