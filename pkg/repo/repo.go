package repo

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"minigit/pkg/diff"
	"minigit/pkg/index"
	"minigit/pkg/object"
)

type Repository struct {
	WorkDir string
	GitDir  string
}

func Init(workDir string) (*Repository, error) {
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, err
	}

	gitDir := filepath.Join(absWorkDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return &Repository{WorkDir: absWorkDir, GitDir: gitDir}, nil
	}

	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "tags"), 0755); err != nil {
		return nil, err
	}

	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		return nil, err
	}

	configPath := filepath.Join(gitDir, "config")
	configContent := "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n"
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return nil, err
	}

	return &Repository{
		WorkDir: absWorkDir,
		GitDir:  gitDir,
	}, nil
}

func FindRepo(startDir string) (*Repository, error) {
	absStart, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}

	curr := absStart
	for {
		gitDir := filepath.Join(curr, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return &Repository{
				WorkDir: curr,
				GitDir:  gitDir,
			}, nil
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}

	return nil, errors.New("not a git repository (or any of the parent directories)")
}

func (r *Repository) ResolveHEAD() (string, bool, string, error) {
	headPath := filepath.Join(r.GitDir, "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "", false, "", err
	}

	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "ref: ") {
		refPath := strings.TrimPrefix(trimmed, "ref: ")
		branchName := strings.TrimPrefix(refPath, "refs/heads/")
		fullRefPath := filepath.Join(r.GitDir, refPath)
		refData, err := os.ReadFile(fullRefPath)
		if err != nil {
			if os.IsNotExist(err) {
				return branchName, true, "", nil
			}
			return branchName, true, "", err
		}
		commitHash := strings.TrimSpace(string(refData))
		return branchName, true, commitHash, nil
	}

	commitHash, err := object.ResolveObjectHash(r.GitDir, trimmed)
	if err != nil {
		return "", false, "", err
	}
	return "", false, commitHash, nil
}

func (r *Repository) SetHEADRef(refName string) error {
	headPath := filepath.Join(r.GitDir, "HEAD")
	content := fmt.Sprintf("ref: refs/heads/%s\n", refName)
	return os.WriteFile(headPath, []byte(content), 0644)
}

func (r *Repository) SetHEADDetached(commitHash string) error {
	headPath := filepath.Join(r.GitDir, "HEAD")
	content := commitHash + "\n"
	return os.WriteFile(headPath, []byte(content), 0644)
}

func (r *Repository) UpdateBranch(branchName string, commitHash string) error {
	refPath := filepath.Join(r.GitDir, "refs", "heads", branchName)
	if err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(refPath, []byte(commitHash+"\n"), 0644)
}

func (r *Repository) ListBranches() ([]string, string, error) {
	headsDir := filepath.Join(r.GitDir, "refs", "heads")
	currentBranch, isSymbolic, _, err := r.ResolveHEAD()
	if err != nil {
		return nil, "", err
	}

	var branches []string
	if _, err := os.Stat(headsDir); err == nil {
		err = filepath.WalkDir(headsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				rel, err := filepath.Rel(headsDir, path)
				if err == nil {
					branches = append(branches, filepath.ToSlash(rel))
				}
			}
			return nil
		})
		if err != nil {
			return nil, "", err
		}
	}

	sort.Strings(branches)
	if !isSymbolic {
		currentBranch = ""
	}
	return branches, currentBranch, nil
}

func (r *Repository) CreateBranch(name string, commitHash string) error {
	refPath := filepath.Join(r.GitDir, "refs", "heads", name)
	if _, err := os.Stat(refPath); err == nil {
		return fmt.Errorf("a branch named '%s' already exists", name)
	}
	if err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(refPath, []byte(commitHash+"\n"), 0644)
}

func (r *Repository) DeleteBranch(name string) error {
	currentBranch, isSymbolic, _, err := r.ResolveHEAD()
	if err == nil && isSymbolic && currentBranch == name {
		return fmt.Errorf("cannot delete checked out branch '%s'", name)
	}
	refPath := filepath.Join(r.GitDir, "refs", "heads", name)
	if _, err := os.Stat(refPath); os.IsNotExist(err) {
		return fmt.Errorf("branch '%s' not found", name)
	}
	return os.Remove(refPath)
}

func (r *Repository) IndexPath() string {
	return filepath.Join(r.GitDir, "index")
}

func (r *Repository) LoadIndex() (*index.Index, error) {
	return index.ReadIndex(r.IndexPath())
}

func (r *Repository) SaveIndex(idx *index.Index) error {
	return idx.Write(r.IndexPath())
}

func (r *Repository) Add(patterns []string) error {
	idx, err := r.LoadIndex()
	if err != nil {
		return err
	}

	for _, pattern := range patterns {
		absPath := pattern
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(r.WorkDir, pattern)
		}

		info, err := os.Lstat(absPath)
		if err != nil {
			matches, globErr := filepath.Glob(absPath)
			if globErr == nil && len(matches) > 0 {
				for _, m := range matches {
					if err := r.addSinglePath(idx, m); err != nil {
						return err
					}
				}
				continue
			}
			return err
		}

		if info.IsDir() {
			err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() && d.Name() == ".git" {
					return filepath.SkipDir
				}
				if !d.IsDir() {
					return r.addSinglePath(idx, path)
				}
				return nil
			})
			if err != nil {
				return err
			}
		} else {
			if err := r.addSinglePath(idx, absPath); err != nil {
				return err
			}
		}
	}

	return r.SaveIndex(idx)
}

func (r *Repository) addSinglePath(idx *index.Index, absPath string) error {
	relPath, err := filepath.Rel(r.WorkDir, absPath)
	if err != nil {
		return err
	}
	gitPath := filepath.ToSlash(relPath)
	if strings.HasPrefix(gitPath, ".git") {
		return nil
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}

	hash, err := object.WriteObject(r.GitDir, object.TypeBlob, data)
	if err != nil {
		return err
	}

	hashBytes, err := hex.DecodeString(hash)
	if err != nil {
		return err
	}

	var sha [20]byte
	copy(sha[:], hashBytes)

	mode := uint32(0100644)
	if info.Mode()&0111 != 0 {
		mode = 0100755
	}

	entry := index.IndexEntry{
		MtimeSec: uint32(info.ModTime().Unix()),
		FileSize: uint32(info.Size()),
		Mode:     mode,
		Sha1:     sha,
		Path:     gitPath,
	}

	idx.AddOrUpdate(entry)
	return nil
}

func (r *Repository) Remove(paths []string, cached bool) error {
	idx, err := r.LoadIndex()
	if err != nil {
		return err
	}

	for _, p := range paths {
		absPath := p
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(r.WorkDir, p)
		}
		relPath, err := filepath.Rel(r.WorkDir, absPath)
		if err != nil {
			return err
		}
		gitPath := filepath.ToSlash(relPath)

		removed := idx.Remove(gitPath)
		if !removed {
			return fmt.Errorf("path '%s' did not match any files", gitPath)
		}

		if !cached {
			if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}

	return r.SaveIndex(idx)
}

func (r *Repository) Commit(message string, author string) (string, error) {
	idx, err := r.LoadIndex()
	if err != nil {
		return "", err
	}

	treeHash, err := idx.BuildTree(r.GitDir)
	if err != nil {
		return "", err
	}

	branchName, isSymbolic, headHash, err := r.ResolveHEAD()
	if err != nil {
		return "", err
	}

	if headHash != "" {
		parentCommit, err := r.LoadCommit(headHash)
		if err == nil && parentCommit.TreeHash == treeHash {
			return "", errors.New("nothing to commit, working tree clean")
		}
	}

	var parentHashes []string
	if headHash != "" {
		parentHashes = append(parentHashes, headHash)
	}

	if author == "" {
		author = os.Getenv("GIT_AUTHOR_NAME")
		email := os.Getenv("GIT_AUTHOR_EMAIL")
		if author == "" {
			author = "minigit author"
		}
		if email == "" {
			email = "author@minigit.local"
		}
		author = fmt.Sprintf("%s <%s>", author, email)
	}

	now := time.Now()
	commitObj := &object.Commit{
		TreeHash:      treeHash,
		ParentHashes:  parentHashes,
		Author:        author,
		AuthorTime:    now,
		Committer:     author,
		CommitterTime: now,
		Message:       message,
	}

	commitHash, err := object.WriteObject(r.GitDir, object.TypeCommit, commitObj.Serialize())
	if err != nil {
		return "", err
	}

	if isSymbolic {
		if err := r.UpdateBranch(branchName, commitHash); err != nil {
			return "", err
		}
	} else {
		if err := r.SetHEADDetached(commitHash); err != nil {
			return "", err
		}
	}

	return commitHash, nil
}

func (r *Repository) LoadCommit(hash string) (*object.Commit, error) {
	objType, data, _, err := object.ReadObject(r.GitDir, hash)
	if err != nil {
		return nil, err
	}
	if objType != object.TypeCommit {
		return nil, fmt.Errorf("object %s is a %s, not a commit", hash, objType)
	}
	return object.ParseCommit(data)
}

type FlattenedFile struct {
	Mode string
	Hash string
}

func (r *Repository) FlattenTree(treeHash string, prefix string) (map[string]FlattenedFile, error) {
	res := make(map[string]FlattenedFile)
	if treeHash == "" {
		return res, nil
	}

	objType, data, _, err := object.ReadObject(r.GitDir, treeHash)
	if err != nil {
		return nil, err
	}
	if objType != object.TypeTree {
		return nil, fmt.Errorf("object %s is %s, not a tree", treeHash, objType)
	}

	tree, err := object.ParseTree(data)
	if err != nil {
		return nil, err
	}

	for _, entry := range tree.Entries {
		entryPath := entry.Name
		if prefix != "" {
			entryPath = prefix + "/" + entry.Name
		}

		if entry.Mode == "40000" {
			sub, err := r.FlattenTree(entry.Hash, entryPath)
			if err != nil {
				return nil, err
			}
			for k, v := range sub {
				res[k] = v
			}
		} else {
			res[entryPath] = FlattenedFile{
				Mode: entry.Mode,
				Hash: entry.Hash,
			}
		}
	}

	return res, nil
}

type StatusType string

const (
	StatusAdded    StatusType = "new file"
	StatusModified StatusType = "modified"
	StatusDeleted  StatusType = "deleted"
)

type FileStatus struct {
	Path   string
	Status StatusType
}

type RepoStatus struct {
	Branch        string
	IsDetached    bool
	HEADHash      string
	Staged        []FileStatus
	Unstaged      []FileStatus
	Untracked     []string
	Clean         bool
}

func (r *Repository) Status() (*RepoStatus, error) {
	branch, isSymbolic, headHash, err := r.ResolveHEAD()
	if err != nil {
		return nil, err
	}

	res := &RepoStatus{
		Branch:     branch,
		IsDetached: !isSymbolic,
		HEADHash:   headHash,
	}

	var headFiles map[string]FlattenedFile
	if headHash != "" {
		commit, err := r.LoadCommit(headHash)
		if err == nil {
			headFiles, _ = r.FlattenTree(commit.TreeHash, "")
		}
	}
	if headFiles == nil {
		headFiles = make(map[string]FlattenedFile)
	}

	idx, err := r.LoadIndex()
	if err != nil {
		return nil, err
	}

	indexFiles := make(map[string]index.IndexEntry)
	for _, entry := range idx.Entries {
		indexFiles[entry.Path] = entry
	}

	for p, idxEntry := range indexFiles {
		headEntry, inHead := headFiles[p]
		idxSha := hex.EncodeToString(idxEntry.Sha1[:])
		if !inHead {
			res.Staged = append(res.Staged, FileStatus{Path: p, Status: StatusAdded})
		} else if headEntry.Hash != idxSha {
			res.Staged = append(res.Staged, FileStatus{Path: p, Status: StatusModified})
		}
	}
	for p := range headFiles {
		if _, inIndex := indexFiles[p]; !inIndex {
			res.Staged = append(res.Staged, FileStatus{Path: p, Status: StatusDeleted})
		}
	}

	workingFiles := make(map[string]os.FileInfo)
	err = filepath.WalkDir(r.WorkDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			rel, err := filepath.Rel(r.WorkDir, path)
			if err != nil {
				return err
			}
			gitPath := filepath.ToSlash(rel)
			info, err := d.Info()
			if err != nil {
				return err
			}
			workingFiles[gitPath] = info
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for p, idxEntry := range indexFiles {
		wf, exists := workingFiles[p]
		if !exists {
			res.Unstaged = append(res.Unstaged, FileStatus{Path: p, Status: StatusDeleted})
		} else {
			content, err := os.ReadFile(filepath.Join(r.WorkDir, filepath.FromSlash(p)))
			if err == nil {
				hash, _ := object.HashObject(object.TypeBlob, content)
				idxSha := hex.EncodeToString(idxEntry.Sha1[:])
				if hash != idxSha {
					res.Unstaged = append(res.Unstaged, FileStatus{Path: p, Status: StatusModified})
				}
			} else {
				_ = wf
			}
		}
	}

	for p := range workingFiles {
		if _, inIndex := indexFiles[p]; !inIndex {
			res.Untracked = append(res.Untracked, p)
		}
	}

	sort.Slice(res.Staged, func(i, j int) bool { return res.Staged[i].Path < res.Staged[j].Path })
	sort.Slice(res.Unstaged, func(i, j int) bool { return res.Unstaged[i].Path < res.Unstaged[j].Path })
	sort.Strings(res.Untracked)

	res.Clean = len(res.Staged) == 0 && len(res.Unstaged) == 0 && len(res.Untracked) == 0
	return res, nil
}

type LogEntry struct {
	Hash   string
	Commit *object.Commit
}

func (r *Repository) Log(startHash string, maxCount int) ([]LogEntry, error) {
	if startHash == "" {
		_, _, headHash, err := r.ResolveHEAD()
		if err != nil {
			return nil, err
		}
		if headHash == "" {
			return nil, errors.New("your current branch does not have any commits yet")
		}
		startHash = headHash
	} else {
		resolved, err := object.ResolveObjectHash(r.GitDir, startHash)
		if err != nil {
			return nil, err
		}
		startHash = resolved
	}

	var entries []LogEntry
	curr := startHash
	for curr != "" {
		commit, err := r.LoadCommit(curr)
		if err != nil {
			return nil, err
		}
		entries = append(entries, LogEntry{
			Hash:   curr,
			Commit: commit,
		})

		if maxCount > 0 && len(entries) >= maxCount {
			break
		}

		if len(commit.ParentHashes) > 0 {
			curr = commit.ParentHashes[0]
		} else {
			curr = ""
		}
	}

	return entries, nil
}

func (r *Repository) Checkout(target string) error {
	targetBranchRef := filepath.Join(r.GitDir, "refs", "heads", target)
	var targetCommitHash string
	isBranch := false

	if refData, err := os.ReadFile(targetBranchRef); err == nil {
		targetCommitHash = strings.TrimSpace(string(refData))
		isBranch = true
	} else {
		resolved, err := object.ResolveObjectHash(r.GitDir, target)
		if err != nil {
			return fmt.Errorf("pathspec '%s' did not match any file(s) known to git", target)
		}
		targetCommitHash = resolved
	}

	targetCommit, err := r.LoadCommit(targetCommitHash)
	if err != nil {
		return err
	}

	targetFiles, err := r.FlattenTree(targetCommit.TreeHash, "")
	if err != nil {
		return err
	}

	idx, err := r.LoadIndex()
	if err != nil {
		return err
	}

	for _, entry := range idx.Entries {
		if _, existsInTarget := targetFiles[entry.Path]; !existsInTarget {
			filePath := filepath.Join(r.WorkDir, filepath.FromSlash(entry.Path))
			_ = os.Remove(filePath)
		}
	}

	newIdx := &index.Index{}
	for p, f := range targetFiles {
		_, content, _, err := object.ReadObject(r.GitDir, f.Hash)
		if err != nil {
			return err
		}

		filePath := filepath.Join(r.WorkDir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}

		mode := os.FileMode(0644)
		if f.Mode == "100755" {
			mode = 0755
		}
		if err := os.WriteFile(filePath, content, mode); err != nil {
			return err
		}

		hashBytes, err := hex.DecodeString(f.Hash)
		if err != nil {
			return err
		}
		var sha [20]byte
		copy(sha[:], hashBytes)

		modeUint := uint32(0100644)
		if f.Mode == "100755" {
			modeUint = 0100755
		}

		newIdx.AddOrUpdate(index.IndexEntry{
			Mode:     modeUint,
			FileSize: uint32(len(content)),
			Sha1:     sha,
			Path:     p,
		})
	}

	if err := r.SaveIndex(newIdx); err != nil {
		return err
	}

	if isBranch {
		return r.SetHEADRef(target)
	}
	return r.SetHEADDetached(targetCommitHash)
}

func (r *Repository) Diff(staged bool) (string, error) {
	idx, err := r.LoadIndex()
	if err != nil {
		return "", err
	}

	var sb strings.Builder

	if staged {
		_, _, headHash, err := r.ResolveHEAD()
		if err != nil {
			return "", err
		}

		var headFiles map[string]FlattenedFile
		if headHash != "" {
			commit, err := r.LoadCommit(headHash)
			if err == nil {
				headFiles, _ = r.FlattenTree(commit.TreeHash, "")
			}
		}
		if headFiles == nil {
			headFiles = make(map[string]FlattenedFile)
		}

		for _, entry := range idx.Entries {
			var oldContent string
			if headEntry, ok := headFiles[entry.Path]; ok {
				_, data, _, err := object.ReadObject(r.GitDir, headEntry.Hash)
				if err == nil {
					oldContent = string(data)
				}
			}

			_, newData, _, err := object.ReadObject(r.GitDir, hex.EncodeToString(entry.Sha1[:]))
			if err != nil {
				continue
			}
			newContent := string(newData)

			d := diff.DiffStrings(entry.Path, entry.Path, oldContent, newContent)
			if d != "" {
				sb.WriteString(d)
			}
		}
	} else {
		for _, entry := range idx.Entries {
			_, data, _, err := object.ReadObject(r.GitDir, hex.EncodeToString(entry.Sha1[:]))
			if err != nil {
				continue
			}
			oldContent := string(data)

			filePath := filepath.Join(r.WorkDir, filepath.FromSlash(entry.Path))
			newData, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			newContent := string(newData)

			d := diff.DiffStrings(entry.Path, entry.Path, oldContent, newContent)
			if d != "" {
				sb.WriteString(d)
			}
		}
	}

	return sb.String(), nil
}
