package repo

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"minigit/pkg/index"
)

type ResetMode int

const (
	ResetSoft ResetMode = iota
	ResetMixed
	ResetHard
)

func (r *Repository) Reset(mode ResetMode, targetRev string) (string, error) {
	if targetRev == "" {
		targetRev = "HEAD"
	}

	currentBranch, isSymbolic, headHash, err := r.ResolveHEAD()
	if err != nil {
		return "", err
	}

	if headHash == "" {
		if targetRev != "" && targetRev != "HEAD" {
			return "", errors.New("cannot reset to revision on unborn branch")
		}
		switch mode {
		case ResetSoft:
			r.cleanMergeState()
			return "", nil
		case ResetMixed:
			_ = r.SaveIndex(&index.Index{})
			r.cleanMergeState()
			return "", nil
		case ResetHard:
			idx, _ := r.LoadIndex()
			if idx != nil {
				for _, entry := range idx.Entries {
					filePath := filepath.Join(r.WorkDir, filepath.FromSlash(entry.Path))
					_ = os.Remove(filePath)
					cleanEmptyDirs(filepath.Dir(filePath), r.WorkDir)
				}
			}
			_ = r.SaveIndex(&index.Index{})
			r.cleanMergeState()
			return "", nil
		default:
			return "", errors.New("unsupported reset mode")
		}
	}

	targetHash, err := r.ResolveRevision(targetRev)
	if err != nil {
		return "", err
	}

	targetCommit, err := r.LoadCommit(targetHash)
	if err != nil {
		return "", fmt.Errorf("revision '%s' does not point to a commit", targetRev)
	}

	switch mode {
	case ResetSoft:
		if isSymbolic {
			if err := r.UpdateBranch(currentBranch, targetHash); err != nil {
				return "", err
			}
		} else {
			if err := r.SetHEADDetached(targetHash); err != nil {
				return "", err
			}
		}
		r.cleanMergeState()
		return "", nil

	case ResetMixed:
		if isSymbolic {
			if err := r.UpdateBranch(currentBranch, targetHash); err != nil {
				return "", err
			}
		} else {
			if err := r.SetHEADDetached(targetHash); err != nil {
				return "", err
			}
		}
		if err := r.resetTree(targetCommit.TreeHash, false); err != nil {
			return "", err
		}
		r.cleanMergeState()

		st, err := r.Status()
		if err == nil && len(st.Unstaged) > 0 {
			var sb strings.Builder
			sb.WriteString("Unstaged changes after reset:\n")
			for _, u := range st.Unstaged {
				switch u.Status {
				case StatusModified:
					sb.WriteString(fmt.Sprintf("M\t%s\n", u.Path))
				case StatusDeleted:
					sb.WriteString(fmt.Sprintf("D\t%s\n", u.Path))
				}
			}
			return sb.String(), nil
		}
		return "", nil

	case ResetHard:
		if isSymbolic {
			if err := r.UpdateBranch(currentBranch, targetHash); err != nil {
				return "", err
			}
		} else {
			if err := r.SetHEADDetached(targetHash); err != nil {
				return "", err
			}
		}
		if err := r.resetTree(targetCommit.TreeHash, true); err != nil {
			return "", err
		}
		r.cleanMergeState()

		firstLine := strings.SplitN(targetCommit.Message, "\n", 2)[0]
		return fmt.Sprintf("HEAD is now at %s %s\n", targetHash[:7], firstLine), nil

	default:
		return "", errors.New("unsupported reset mode")
	}
}

func (r *Repository) ResetPaths(targetRev string, paths []string) error {
	if targetRev == "" {
		targetRev = "HEAD"
	}

	_, _, headHash, _ := r.ResolveHEAD()
	var targetFiles map[string]FlattenedFile
	if headHash != "" {
		targetHash, err := r.ResolveRevision(targetRev)
		if err != nil {
			return err
		}
		targetCommit, err := r.LoadCommit(targetHash)
		if err != nil {
			return fmt.Errorf("revision '%s' does not point to a commit", targetRev)
		}
		targetFiles, err = r.FlattenTree(targetCommit.TreeHash, "")
		if err != nil {
			return err
		}
	}
	if targetFiles == nil {
		targetFiles = make(map[string]FlattenedFile)
	}

	idx, err := r.LoadIndex()
	if err != nil {
		idx = &index.Index{}
	}

	for _, p := range paths {
		relPath := p
		if filepath.IsAbs(p) {
			rel, err := filepath.Rel(r.WorkDir, p)
			if err == nil {
				relPath = rel
			}
		}
		gitPath := filepath.ToSlash(relPath)

		matched := false
		if tf, inTarget := targetFiles[gitPath]; inTarget {
			matched = true
			hBytes, err := hex.DecodeString(tf.Hash)
			if err == nil {
				var sha [20]byte
				copy(sha[:], hBytes)
				m := uint32(0100644)
				if tf.Mode == "100755" || tf.Mode == "0100755" {
					m = 0100755
				} else if tf.Mode == "120000" || tf.Mode == "0120000" {
					m = 0120000
				}
				idx.AddOrUpdate(index.IndexEntry{
					Mode: m,
					Sha1: sha,
					Path: gitPath,
				})
			}
		} else {
			if idx.Remove(gitPath) {
				matched = true
			}
		}

		if !matched {
			prefix := gitPath + "/"
			for tp, tf := range targetFiles {
				if strings.HasPrefix(tp, prefix) {
					hBytes, err := hex.DecodeString(tf.Hash)
					if err == nil {
						var sha [20]byte
						copy(sha[:], hBytes)
						m := uint32(0100644)
						if tf.Mode == "100755" || tf.Mode == "0100755" {
							m = 0100755
						} else if tf.Mode == "120000" || tf.Mode == "0120000" {
							m = 0120000
						}
						idx.AddOrUpdate(index.IndexEntry{
							Mode: m,
							Sha1: sha,
							Path: tp,
						})
					}
				}
			}
			for _, ie := range idx.Entries {
				if strings.HasPrefix(ie.Path, prefix) {
					if _, inTarget := targetFiles[ie.Path]; !inTarget {
						idx.Remove(ie.Path)
					}
				}
			}
		}
	}

	return r.SaveIndex(idx)
}

func (r *Repository) cleanMergeState() {
	_ = os.Remove(filepath.Join(r.GitDir, "MERGE_HEAD"))
	_ = os.Remove(filepath.Join(r.GitDir, "MERGE_MSG"))
	_ = os.Remove(filepath.Join(r.GitDir, "MERGE_MODE"))
	_ = os.Remove(filepath.Join(r.GitDir, "CHERRY_PICK_HEAD"))
}
