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

func (r *Repository) ReadPackedRefs() (map[string]string, error) {
	packedPath := filepath.Join(r.GitDir, "packed-refs")
	data, err := os.ReadFile(packedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}

	refs := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			refs[parts[1]] = parts[0]
		}
	}
	return refs, nil
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
		if err == nil {
			commitHash := strings.TrimSpace(string(refData))
			return branchName, true, commitHash, nil
		}
		if os.IsNotExist(err) {
			packed, pErr := r.ReadPackedRefs()
			if pErr == nil {
				if hash, ok := packed[refPath]; ok {
					return branchName, true, hash, nil
				}
			}
			return branchName, true, "", nil
		}
		return branchName, true, "", err
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

	branchMap := make(map[string]bool)
	if _, err := os.Stat(headsDir); err == nil {
		_ = filepath.WalkDir(headsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				rel, err := filepath.Rel(headsDir, path)
				if err == nil {
					branchMap[filepath.ToSlash(rel)] = true
				}
			}
			return nil
		})
	}

	packed, err := r.ReadPackedRefs()
	if err == nil {
		for refName := range packed {
			if strings.HasPrefix(refName, "refs/heads/") {
				branchMap[strings.TrimPrefix(refName, "refs/heads/")] = true
			}
		}
	}

	var branches []string
	for b := range branchMap {
		branches = append(branches, b)
	}
	sort.Strings(branches)

	if !isSymbolic {
		currentBranch = ""
	}
	return branches, currentBranch, nil
}

func (r *Repository) ListTags() ([]string, error) {
	tagsDir := filepath.Join(r.GitDir, "refs", "tags")
	tagMap := make(map[string]bool)
	if _, err := os.Stat(tagsDir); err == nil {
		_ = filepath.WalkDir(tagsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				rel, err := filepath.Rel(tagsDir, path)
				if err == nil {
					tagMap[filepath.ToSlash(rel)] = true
				}
			}
			return nil
		})
	}

	packed, err := r.ReadPackedRefs()
	if err == nil {
		for refName := range packed {
			if strings.HasPrefix(refName, "refs/tags/") {
				tagMap[strings.TrimPrefix(refName, "refs/tags/")] = true
			}
		}
	}

	var tags []string
	for t := range tagMap {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags, nil
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

func resolveBaseDir(workDir string) string {
	baseDir := workDir
	cwd, err := os.Getwd()
	if err == nil {
		evalWorkDir, err1 := filepath.EvalSymlinks(workDir)
		if err1 != nil {
			evalWorkDir = workDir
		}
		evalCwd, err2 := filepath.EvalSymlinks(cwd)
		if err2 != nil {
			evalCwd = cwd
		}
		if rel, err := filepath.Rel(evalWorkDir, evalCwd); err == nil && !strings.HasPrefix(rel, "..") {
			baseDir = cwd
		}
	}
	return baseDir
}

func (r *Repository) Add(patterns []string) error {
	idx, err := r.LoadIndex()
	if err != nil {
		return err
	}

	baseDir := resolveBaseDir(r.WorkDir)

	for _, pattern := range patterns {
		absPath := pattern
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Clean(filepath.Join(baseDir, pattern))
		}

		evalWorkDir, _ := filepath.EvalSymlinks(r.WorkDir)
		if evalWorkDir == "" {
			evalWorkDir = r.WorkDir
		}
		evalAbsPath, _ := filepath.EvalSymlinks(absPath)
		if evalAbsPath == "" {
			evalAbsPath = absPath
		}
		relToRepo, err := filepath.Rel(evalWorkDir, evalAbsPath)
		if err != nil || strings.HasPrefix(relToRepo, "..") {
			relToRepo, err = filepath.Rel(r.WorkDir, absPath)
			if err != nil || strings.HasPrefix(relToRepo, "..") {
				return fmt.Errorf("pathspec '%s' is outside repository at '%s'", pattern, r.WorkDir)
			}
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

	var data []byte
	mode := uint32(0100644)

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(absPath)
		if err != nil {
			return err
		}
		data = []byte(target)
		mode = 0120000
	} else {
		data, err = os.ReadFile(absPath)
		if err != nil {
			return err
		}
		if info.Mode()&0111 != 0 {
			mode = 0100755
		}
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

	entry := index.IndexEntry{
		MtimeSec: uint32(info.ModTime().Unix()),
		FileSize: uint32(len(data)),
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

	baseDir := resolveBaseDir(r.WorkDir)

	for _, p := range paths {
		absPath := p
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Clean(filepath.Join(baseDir, p))
		}
		evalWorkDir, _ := filepath.EvalSymlinks(r.WorkDir)
		if evalWorkDir == "" {
			evalWorkDir = r.WorkDir
		}
		evalAbsPath, _ := filepath.EvalSymlinks(absPath)
		if evalAbsPath == "" {
			evalAbsPath = absPath
		}
		relPath, err := filepath.Rel(evalWorkDir, evalAbsPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			relPath, err = filepath.Rel(r.WorkDir, absPath)
			if err != nil || strings.HasPrefix(relPath, "..") {
				return fmt.Errorf("pathspec '%s' is outside repository at '%s'", p, r.WorkDir)
			}
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
			cleanEmptyDirs(filepath.Dir(absPath), r.WorkDir)
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

		if entry.Mode == "40000" || entry.Mode == "040000" {
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
			fullPath := filepath.Join(r.WorkDir, filepath.FromSlash(p))
			var content []byte
			var readErr error
			if wf.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(fullPath)
				if err == nil {
					content = []byte(target)
				} else {
					readErr = err
				}
			} else {
				content, readErr = os.ReadFile(fullPath)
			}
			if readErr == nil {
				hash, _ := object.HashObject(object.TypeBlob, content)
				idxSha := hex.EncodeToString(idxEntry.Sha1[:])
				if hash != idxSha {
					res.Unstaged = append(res.Unstaged, FileStatus{Path: p, Status: StatusModified})
				}
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
		packed, pErr := r.ReadPackedRefs()
		if pErr == nil {
			if h, ok := packed["refs/heads/"+target]; ok {
				targetCommitHash = h
				isBranch = true
			}
		}
	}

	if !isBranch {
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
			cleanEmptyDirs(filepath.Dir(filePath), r.WorkDir)
		}
	}

	newIdx := &index.Index{}
	for p, f := range targetFiles {
		_, content, _, err := object.ReadObject(r.GitDir, f.Hash)
		if err != nil {
			return err
		}

		filePath := filepath.Join(r.WorkDir, filepath.FromSlash(p))
		if err := r.writeFileToWorkDir(filePath, f.Mode, content); err != nil {
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
		} else if f.Mode == "120000" {
			modeUint = 0120000
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

		idxMap := make(map[string]index.IndexEntry)
		for _, entry := range idx.Entries {
			idxMap[entry.Path] = entry
		}

		allPathsMap := make(map[string]bool)
		for _, entry := range idx.Entries {
			allPathsMap[entry.Path] = true
		}
		for p := range headFiles {
			allPathsMap[p] = true
		}

		var allPaths []string
		for p := range allPathsMap {
			allPaths = append(allPaths, p)
		}
		sort.Strings(allPaths)

		for _, p := range allPaths {
			idxEntry, inIdx := idxMap[p]
			headEntry, inHead := headFiles[p]

			if inIdx && inHead {
				idxSha := hex.EncodeToString(idxEntry.Sha1[:])
				if idxSha != headEntry.Hash {
					_, oldData, _, err1 := object.ReadObject(r.GitDir, headEntry.Hash)
					_, newData, _, err2 := object.ReadObject(r.GitDir, idxSha)
					if err1 == nil && err2 == nil {
						d := diff.DiffStrings(p, p, string(oldData), string(newData))
						sb.WriteString(d)
					}
				}
			} else if inIdx && !inHead {
				idxSha := hex.EncodeToString(idxEntry.Sha1[:])
				_, newData, _, err := object.ReadObject(r.GitDir, idxSha)
				if err == nil {
					d := diff.DiffStrings("/dev/null", p, "", string(newData))
					sb.WriteString(d)
				}
			} else if !inIdx && inHead {
				_, oldData, _, err := object.ReadObject(r.GitDir, headEntry.Hash)
				if err == nil {
					d := diff.DiffStrings(p, "/dev/null", string(oldData), "")
					sb.WriteString(d)
				}
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
			var newContent string
			var isDeleted bool

			info, err := os.Lstat(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					isDeleted = true
				} else {
					continue
				}
			} else if info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(filePath)
				if err != nil {
					continue
				}
				newContent = target
			} else {
				newData, err := os.ReadFile(filePath)
				if err != nil {
					continue
				}
				newContent = string(newData)
			}

			if isDeleted {
				d := diff.DiffStrings(entry.Path, "/dev/null", oldContent, "")
				sb.WriteString(d)
			} else {
				d := diff.DiffStrings(entry.Path, entry.Path, oldContent, newContent)
				sb.WriteString(d)
			}
		}
	}

	return sb.String(), nil
}

func (r *Repository) Merge(target string) (string, error) {
	currentBranch, isSymbolic, headHash, err := r.ResolveHEAD()
	if err != nil {
		return "", err
	}
	if headHash == "" {
		return "", errors.New("cannot merge into a branch without commits")
	}

	targetBranchRef := filepath.Join(r.GitDir, "refs", "heads", target)
	var targetCommitHash string
	if refData, err := os.ReadFile(targetBranchRef); err == nil {
		targetCommitHash = strings.TrimSpace(string(refData))
	} else {
		packed, pErr := r.ReadPackedRefs()
		if pErr == nil {
			if h, ok := packed["refs/heads/"+target]; ok {
				targetCommitHash = h
			}
		}
	}
	if targetCommitHash == "" {
		resolved, err := object.ResolveObjectHash(r.GitDir, target)
		if err != nil {
			return "", fmt.Errorf("could not resolve %s", target)
		}
		targetCommitHash = resolved
	}

	if headHash == targetCommitHash {
		return "Already up to date.", nil
	}

	baseHash, err := r.FindMergeBase(headHash, targetCommitHash)
	if err != nil {
		return "", err
	}

	if baseHash == targetCommitHash {
		return "Already up to date.", nil
	}

	if baseHash == headHash {
		if err := r.Checkout(target); err != nil {
			return "", err
		}
		if isSymbolic {
			if err := r.UpdateBranch(currentBranch, targetCommitHash); err != nil {
				return "", err
			}
		}
		return fmt.Sprintf("Fast-forward merge to %s", targetCommitHash[:7]), nil
	}

	baseCommit, err := r.LoadCommit(baseHash)
	if err != nil {
		return "", err
	}
	ourCommit, err := r.LoadCommit(headHash)
	if err != nil {
		return "", err
	}
	theirCommit, err := r.LoadCommit(targetCommitHash)
	if err != nil {
		return "", err
	}

	baseFiles, err := r.FlattenTree(baseCommit.TreeHash, "")
	if err != nil {
		return "", err
	}
	ourFiles, err := r.FlattenTree(ourCommit.TreeHash, "")
	if err != nil {
		return "", err
	}
	theirFiles, err := r.FlattenTree(theirCommit.TreeHash, "")
	if err != nil {
		return "", err
	}

	allPathsMap := make(map[string]bool)
	for p := range baseFiles {
		allPathsMap[p] = true
	}
	for p := range ourFiles {
		allPathsMap[p] = true
	}
	for p := range theirFiles {
		allPathsMap[p] = true
	}

	var allPaths []string
	for p := range allPathsMap {
		allPaths = append(allPaths, p)
	}
	sort.Strings(allPaths)

	idx, err := r.LoadIndex()
	if err != nil {
		return "", err
	}

	hasConflict := false

	for _, p := range allPaths {
		fBase, inBase := baseFiles[p]
		fOurs, inOurs := ourFiles[p]
		fTheirs, inTheirs := theirFiles[p]

		filePath := filepath.Join(r.WorkDir, filepath.FromSlash(p))

		if !inBase && inOurs && !inTheirs {
			continue
		}
		if !inBase && !inOurs && inTheirs {
			_, data, _, err := object.ReadObject(r.GitDir, fTheirs.Hash)
			if err != nil {
				return "", err
			}
			if err := r.writeFileToWorkDir(filePath, fTheirs.Mode, data); err != nil {
				return "", err
			}
			hBytes, _ := hex.DecodeString(fTheirs.Hash)
			var sha [20]byte
			copy(sha[:], hBytes)
			m := uint32(0100644)
			if fTheirs.Mode == "100755" {
				m = 0100755
			} else if fTheirs.Mode == "120000" {
				m = 0120000
			}
			idx.AddOrUpdate(index.IndexEntry{Mode: m, FileSize: uint32(len(data)), Sha1: sha, Path: p})
			continue
		}
		if inBase && !inOurs && inTheirs {
			if fBase.Hash == fTheirs.Hash {
				continue
			}
		}
		if inBase && inOurs && !inTheirs {
			if fBase.Hash == fOurs.Hash {
				_ = os.Remove(filePath)
				cleanEmptyDirs(filepath.Dir(filePath), r.WorkDir)
				idx.Remove(p)
				continue
			}
		}
		if inOurs && inTheirs && fOurs.Hash == fTheirs.Hash {
			continue
		}
		if inBase && inOurs && inTheirs {
			if fOurs.Hash == fBase.Hash && fTheirs.Hash != fBase.Hash {
				_, data, _, err := object.ReadObject(r.GitDir, fTheirs.Hash)
				if err != nil {
					return "", err
				}
				if err := r.writeFileToWorkDir(filePath, fTheirs.Mode, data); err != nil {
					return "", err
				}
				hBytes, _ := hex.DecodeString(fTheirs.Hash)
				var sha [20]byte
				copy(sha[:], hBytes)
				m := uint32(0100644)
				if fTheirs.Mode == "100755" {
					m = 0100755
				} else if fTheirs.Mode == "120000" {
					m = 0120000
				}
				idx.AddOrUpdate(index.IndexEntry{Mode: m, FileSize: uint32(len(data)), Sha1: sha, Path: p})
				continue
			}
			if fTheirs.Hash == fBase.Hash && fOurs.Hash != fBase.Hash {
				continue
			}
		}

		var ourContent, theirContent string
		if inOurs {
			_, data, _, _ := object.ReadObject(r.GitDir, fOurs.Hash)
			ourContent = string(data)
		}
		if inTheirs {
			_, data, _, _ := object.ReadObject(r.GitDir, fTheirs.Hash)
			theirContent = string(data)
		}

		conflictData := fmt.Sprintf("<<<<<<< HEAD\n%s=======\n%s>>>>>>> %s\n", ourContent, theirContent, target)
		hasConflict = true
		_ = os.MkdirAll(filepath.Dir(filePath), 0755)
		_ = os.WriteFile(filePath, []byte(conflictData), 0644)
	}

	if hasConflict {
		_ = r.SaveIndex(idx)
		return "", errors.New("Automatic merge failed; fix conflicts and then commit the result.")
	}

	if err := r.SaveIndex(idx); err != nil {
		return "", err
	}

	treeHash, err := idx.BuildTree(r.GitDir)
	if err != nil {
		return "", err
	}

	author := os.Getenv("GIT_AUTHOR_NAME")
	email := os.Getenv("GIT_AUTHOR_EMAIL")
	if author == "" {
		author = "minigit author"
	}
	if email == "" {
		email = "author@minigit.local"
	}
	authorStr := fmt.Sprintf("%s <%s>", author, email)
	now := time.Now()

	commitObj := &object.Commit{
		TreeHash:      treeHash,
		ParentHashes:  []string{headHash, targetCommitHash},
		Author:        authorStr,
		AuthorTime:    now,
		Committer:     authorStr,
		CommitterTime: now,
		Message:       fmt.Sprintf("Merge branch '%s'", target),
	}

	commitHash, err := object.WriteObject(r.GitDir, object.TypeCommit, commitObj.Serialize())
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

	return fmt.Sprintf("Merged %s into %s (%s)", target, currentBranch, commitHash[:7]), nil
}

func (r *Repository) FindMergeBase(commitA, commitB string) (string, error) {
	visited := make(map[string]bool)
	queue := []string{commitA}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if visited[curr] {
			continue
		}
		visited[curr] = true
		c, err := r.LoadCommit(curr)
		if err == nil {
			queue = append(queue, c.ParentHashes...)
		}
	}

	queueB := []string{commitB}
	visitedB := make(map[string]bool)
	for len(queueB) > 0 {
		curr := queueB[0]
		queueB = queueB[1:]
		if visited[curr] {
			return curr, nil
		}
		if visitedB[curr] {
			continue
		}
		visitedB[curr] = true
		c, err := r.LoadCommit(curr)
		if err == nil {
			queueB = append(queueB, c.ParentHashes...)
		}
	}

	return "", errors.New("no common ancestor found")
}

func (r *Repository) writeFileToWorkDir(filePath, mode string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	_ = os.Remove(filePath)
	if mode == "120000" {
		return os.Symlink(string(data), filePath)
	}
	fMode := os.FileMode(0644)
	if mode == "100755" {
		fMode = 0755
	}
	return os.WriteFile(filePath, data, fMode)
}

func cleanEmptyDirs(dir, base string) {
	for dir != base && dir != "." && dir != "/" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(dir); err != nil {
			break
		}
		dir = filepath.Dir(dir)
	}
}
