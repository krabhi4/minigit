package repo

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"minigit/pkg/diff"
	"minigit/pkg/ignore"
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

func Clone(srcPath, dstPath string) (*Repository, error) {
	srcAbs, err := filepath.Abs(srcPath)
	if err != nil {
		return nil, err
	}

	srcGitDir := filepath.Join(srcAbs, ".git")
	if info, err := os.Stat(srcGitDir); err != nil || !info.IsDir() {
		headFile := filepath.Join(srcAbs, "HEAD")
		if _, err := os.Stat(headFile); err == nil {
			srcGitDir = srcAbs
		} else {
			return nil, fmt.Errorf("repository '%s' does not exist", srcPath)
		}
	}

	if dstPath == "" {
		base := filepath.Base(filepath.Clean(srcPath))
		dstPath = strings.TrimSuffix(base, ".git")
	}

	dstAbs, err := filepath.Abs(dstPath)
	if err != nil {
		return nil, err
	}

	if entries, err := os.ReadDir(dstAbs); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("destination path '%s' already exists and is not an empty directory", dstPath)
	}

	dstGitDir := filepath.Join(dstAbs, ".git")
	if err := os.MkdirAll(dstGitDir, 0755); err != nil {
		return nil, err
	}

	srcObjects := filepath.Join(srcGitDir, "objects")
	dstObjects := filepath.Join(dstGitDir, "objects")
	if err := copyDir(srcObjects, dstObjects); err != nil {
		return nil, err
	}

	srcRefs := filepath.Join(srcGitDir, "refs")
	dstRefs := filepath.Join(dstGitDir, "refs")
	if err := copyDir(srcRefs, dstRefs); err != nil {
		return nil, err
	}

	srcPackedRefs := filepath.Join(srcGitDir, "packed-refs")
	if _, err := os.Stat(srcPackedRefs); err == nil {
		if err := copyFile(srcPackedRefs, filepath.Join(dstGitDir, "packed-refs")); err != nil {
			return nil, err
		}
	}

	srcHead := filepath.Join(srcGitDir, "HEAD")
	if err := copyFile(srcHead, filepath.Join(dstGitDir, "HEAD")); err != nil {
		return nil, err
	}

	configPath := filepath.Join(dstGitDir, "config")
	configContent := "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n"
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return nil, err
	}

	r := &Repository{
		WorkDir: dstAbs,
		GitDir:  dstGitDir,
	}

	branchName, isSymbolic, headHash, err := r.ResolveHEAD()
	if err != nil {
		return nil, err
	}

	if headHash != "" {
		if isSymbolic && branchName != "" {
			if err := r.Checkout(branchName); err != nil {
				return nil, err
			}
		} else {
			if err := r.Checkout(headHash); err != nil {
				return nil, err
			}
		}
	}

	return r, nil
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

func (r *Repository) ResolveRevision(rev string) (string, error) {
	rev = strings.TrimSpace(rev)
	if rev == "" {
		return "", errors.New("empty revision")
	}

	lastTilde := strings.LastIndex(rev, "~")
	lastCaret := strings.LastIndex(rev, "^")
	splitIdx := lastTilde
	isCaret := false
	if lastCaret > splitIdx {
		splitIdx = lastCaret
		isCaret = true
	}

	if splitIdx > 0 {
		baseRev := rev[:splitIdx]
		nStr := rev[splitIdx+1:]
		valid := true
		for _, ch := range nStr {
			if ch < '0' || ch > '9' {
				valid = false
				break
			}
		}
		if valid {
			if !isCaret {
				n := 1
				if nStr != "" {
					var err error
					n, err = strconv.Atoi(nStr)
					if err != nil {
						return "", fmt.Errorf("invalid revision: %s", rev)
					}
				}
				hash, err := r.ResolveRevision(baseRev)
				if err != nil {
					return "", err
				}
				for i := 0; i < n; i++ {
					c, err := r.LoadCommit(hash)
					if err != nil {
						return "", err
					}
					if len(c.ParentHashes) == 0 {
						return "", fmt.Errorf("revision %s has no parent %d", rev, i+1)
					}
					hash = c.ParentHashes[0]
				}
				return hash, nil
			} else {
				parentNum := 1
				if nStr != "" {
					var err error
					parentNum, err = strconv.Atoi(nStr)
					if err != nil {
						return "", fmt.Errorf("invalid revision: %s", rev)
					}
				}
				if parentNum > 0 {
					hash, err := r.ResolveRevision(baseRev)
					if err != nil {
						return "", err
					}
					c, err := r.LoadCommit(hash)
					if err != nil {
						return "", err
					}
					if len(c.ParentHashes) < parentNum {
						return "", fmt.Errorf("revision %s has no parent %d", rev, parentNum)
					}
					return c.ParentHashes[parentNum-1], nil
				}
			}
		}
	}

	if rev == "HEAD" {
		_, _, headHash, err := r.ResolveHEAD()
		if err != nil {
			return "", err
		}
		if headHash == "" {
			return "", errors.New("HEAD does not point to any commit")
		}
		return headHash, nil
	}

	if rev == "stash" {
		stashPath := filepath.Join(r.GitDir, "refs", "stash")
		if data, err := os.ReadFile(stashPath); err == nil {
			return strings.TrimSpace(string(data)), nil
		}
		packed, err := r.ReadPackedRefs()
		if err == nil {
			if h, ok := packed["refs/stash"]; ok {
				return h, nil
			}
		}
		return "", errors.New("revision 'stash' not found")
	}

	lookupStash := strings.TrimPrefix(rev, "refs/")
	if strings.HasPrefix(lookupStash, "stash@{") && strings.HasSuffix(lookupStash, "}") {
		nStr := lookupStash[len("stash@{") : len(lookupStash)-1]
		n, err := strconv.Atoi(nStr)
		if err == nil {
			logFile := filepath.Join(r.GitDir, "logs", "refs", "stash")
			if data, err := os.ReadFile(logFile); err == nil {
				lines := parseNonEmptyLines(string(data))
				if n >= 0 && n < len(lines) {
					targetLine := lines[len(lines)-1-n]
					fields := strings.Fields(targetLine)
					if len(fields) >= 2 {
						return fields[1], nil
					}
				}
			}
			if n == 0 {
				stashPath := filepath.Join(r.GitDir, "refs", "stash")
				if data, err := os.ReadFile(stashPath); err == nil {
					return strings.TrimSpace(string(data)), nil
				}
			}
		}
		return "", fmt.Errorf("revision '%s' not found", rev)
	}

	if strings.HasPrefix(rev, "refs/") {
		refFile := filepath.Join(r.GitDir, filepath.FromSlash(rev))
		if data, err := os.ReadFile(refFile); err == nil {
			return strings.TrimSpace(string(data)), nil
		}
		packed, err := r.ReadPackedRefs()
		if err == nil {
			if h, ok := packed[rev]; ok {
				return h, nil
			}
		}
	}

	headFile := filepath.Join(r.GitDir, "refs", "heads", rev)
	if data, err := os.ReadFile(headFile); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	tagFile := filepath.Join(r.GitDir, "refs", "tags", rev)
	if data, err := os.ReadFile(tagFile); err == nil {
		targetHash := strings.TrimSpace(string(data))
		objType, objData, _, err := object.ReadObject(r.GitDir, targetHash)
		if err == nil && objType == object.TypeTag {
			lines := strings.Split(string(objData), "\n")
			for _, l := range lines {
				if strings.HasPrefix(l, "object ") {
					return strings.TrimSpace(strings.TrimPrefix(l, "object ")), nil
				}
			}
		}
		return targetHash, nil
	}

	packed, err := r.ReadPackedRefs()
	if err == nil {
		if h, ok := packed["refs/heads/"+rev]; ok {
			return h, nil
		}
		if h, ok := packed["refs/tags/"+rev]; ok {
			objType, objData, _, err := object.ReadObject(r.GitDir, h)
			if err == nil && objType == object.TypeTag {
				lines := strings.Split(string(objData), "\n")
				for _, l := range lines {
					if strings.HasPrefix(l, "object ") {
						return strings.TrimSpace(strings.TrimPrefix(l, "object ")), nil
					}
				}
			}
			return h, nil
		}
	}

	resolved, err := object.ResolveObjectHash(r.GitDir, rev)
	if err == nil {
		objType, objData, _, err := object.ReadObject(r.GitDir, resolved)
		if err == nil && objType == object.TypeTag {
			lines := strings.Split(string(objData), "\n")
			for _, l := range lines {
				if strings.HasPrefix(l, "object ") {
					return strings.TrimSpace(strings.TrimPrefix(l, "object ")), nil
				}
			}
		}
		return resolved, nil
	}

	return "", fmt.Errorf("revision '%s' not found", rev)
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

func (r *Repository) Add(patterns []string, force ...bool) error {
	idx, err := r.LoadIndex()
	if err != nil {
		return err
	}

	forceMode := len(force) > 0 && force[0]
	matcher, _ := ignore.LoadMatcher(r.WorkDir)
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
				rel, err := filepath.Rel(r.WorkDir, path)
				if err != nil {
					return err
				}
				gitPath := filepath.ToSlash(rel)
				if d.IsDir() {
					if gitPath != "." && matcher != nil && matcher.IsIgnored(gitPath, true) {
						return filepath.SkipDir
					}
					return nil
				}
				if !forceMode && !idx.Has(gitPath) && matcher != nil && matcher.IsIgnored(gitPath, false) {
					return nil
				}
				return r.addSinglePath(idx, path)
			})
			if err != nil {
				return err
			}
		} else {
			rel, _ := filepath.Rel(r.WorkDir, absPath)
			gitPath := filepath.ToSlash(rel)
			if !forceMode && !idx.Has(gitPath) && matcher != nil && matcher.IsIgnored(gitPath, false) {
				return fmt.Errorf("The following paths are ignored by one of your .gitignore files:\n%s\nUse -f if you really want to add them.", gitPath)
			}
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
	if gitPath == ".git" || strings.HasPrefix(gitPath, ".git/") {
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

	mergeHeadPath := filepath.Join(r.GitDir, "MERGE_HEAD")
	mergeHeadData, mergeErr := os.ReadFile(mergeHeadPath)
	isMerging := mergeErr == nil && len(bytes.TrimSpace(mergeHeadData)) > 0
	var mergeParentHash string
	if isMerging {
		mergeParentHash = string(bytes.TrimSpace(mergeHeadData))
	}

	if headHash != "" && !isMerging {
		parentCommit, err := r.LoadCommit(headHash)
		if err == nil && parentCommit.TreeHash == treeHash {
			return "", errors.New("nothing to commit, working tree clean")
		}
	}

	var parentHashes []string
	if headHash != "" {
		parentHashes = append(parentHashes, headHash)
	}
	if isMerging && mergeParentHash != "" {
		parentHashes = append(parentHashes, mergeParentHash)
	}

	now := time.Now()
	committer := r.defaultAuthor()
	authorTime := now

	cpHeadPath := filepath.Join(r.GitDir, "CHERRY_PICK_HEAD")
	if cpHeadData, err := os.ReadFile(cpHeadPath); err == nil {
		cpHash := strings.TrimSpace(string(cpHeadData))
		if cpCommit, err := r.LoadCommit(cpHash); err == nil {
			if author == "" {
				author = cpCommit.Author
				authorTime = cpCommit.AuthorTime
			}
		}
	}

	if author == "" {
		author = committer
	}

	commitObj := &object.Commit{
		TreeHash:      treeHash,
		ParentHashes:  parentHashes,
		Author:        author,
		AuthorTime:    authorTime,
		Committer:     committer,
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

	if isMerging {
		_ = os.Remove(mergeHeadPath)
		_ = os.Remove(filepath.Join(r.GitDir, "MERGE_MSG"))
		_ = os.Remove(filepath.Join(r.GitDir, "MERGE_MODE"))
	}
	_ = os.Remove(filepath.Join(r.GitDir, "CHERRY_PICK_HEAD"))

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
	StatusAdded         StatusType = "new file"
	StatusModified      StatusType = "modified"
	StatusDeleted       StatusType = "deleted"
	StatusBothModified  StatusType = "both modified"
	StatusBothAdded     StatusType = "both added"
	StatusDeletedByThem StatusType = "deleted by them"
	StatusDeletedByUs   StatusType = "deleted by us"
)

type FileStatus struct {
	Path   string
	Status StatusType
}

type RepoStatus struct {
	Branch          string
	IsDetached      bool
	HEADHash        string
	IsMerging       bool
	IsCherryPicking bool
	Staged          []FileStatus
	Unstaged        []FileStatus
	Untracked       []string
	Unmerged        []FileStatus
	Clean           bool
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

	if _, err := os.Stat(filepath.Join(r.GitDir, "MERGE_HEAD")); err == nil {
		res.IsMerging = true
	}
	if _, err := os.Stat(filepath.Join(r.GitDir, "CHERRY_PICK_HEAD")); err == nil {
		res.IsCherryPicking = true
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

	stagesByPath := make(map[string]map[int]index.IndexEntry)
	for _, entry := range idx.Entries {
		st := entry.Stage()
		if stagesByPath[entry.Path] == nil {
			stagesByPath[entry.Path] = make(map[int]index.IndexEntry)
		}
		stagesByPath[entry.Path][st] = entry
	}

	indexFiles := make(map[string]index.IndexEntry)
	unmergedPaths := make(map[string]bool)

	for p, stages := range stagesByPath {
		has1 := stages[1].Path != ""
		has2 := stages[2].Path != ""
		has3 := stages[3].Path != ""

		if has1 || has2 || has3 {
			unmergedPaths[p] = true
			var stType StatusType
			if has1 && has2 && has3 {
				stType = StatusBothModified
			} else if !has1 && has2 && has3 {
				stType = StatusBothAdded
			} else if has1 && has2 && !has3 {
				stType = StatusDeletedByThem
			} else if has1 && !has2 && has3 {
				stType = StatusDeletedByUs
			} else {
				stType = StatusBothModified
			}
			res.Unmerged = append(res.Unmerged, FileStatus{Path: p, Status: stType})
		} else if e, ok := stages[0]; ok {
			indexFiles[p] = e
		}
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
		if unmergedPaths[p] {
			continue
		}
		if _, inIndex := indexFiles[p]; !inIndex {
			res.Staged = append(res.Staged, FileStatus{Path: p, Status: StatusDeleted})
		}
	}

	matcher, _ := ignore.LoadMatcher(r.WorkDir)
	workingFiles := make(map[string]os.FileInfo)
	err = filepath.WalkDir(r.WorkDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(r.WorkDir, path)
		if err != nil {
			return err
		}
		gitPath := filepath.ToSlash(rel)
		if d.IsDir() {
			if gitPath != "." && matcher != nil && matcher.IsIgnored(gitPath, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if _, inIndex := indexFiles[gitPath]; !inIndex {
			if matcher != nil && matcher.IsIgnored(gitPath, false) {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		workingFiles[gitPath] = info
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
		if unmergedPaths[p] {
			continue
		}
		if _, inIndex := indexFiles[p]; !inIndex {
			res.Untracked = append(res.Untracked, p)
		}
	}

	sort.Slice(res.Staged, func(i, j int) bool { return res.Staged[i].Path < res.Staged[j].Path })
	sort.Slice(res.Unstaged, func(i, j int) bool { return res.Unstaged[i].Path < res.Unstaged[j].Path })
	sort.Slice(res.Unmerged, func(i, j int) bool { return res.Unmerged[i].Path < res.Unmerged[j].Path })
	sort.Strings(res.Untracked)

	res.Clean = len(res.Staged) == 0 && len(res.Unstaged) == 0 && len(res.Untracked) == 0 && len(res.Unmerged) == 0 && !res.IsMerging && !res.IsCherryPicking
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
		resolved, err := r.ResolveRevision(startHash)
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
		resolved, err := r.ResolveRevision(target)
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

type diffItem struct {
	sortPath string
	content  string
}

type fileEntry struct {
	path string
	mode string
	hash string
	data []byte
}

type renamePair struct {
	oldEntry   fileEntry
	newEntry   fileEntry
	similarity int
}

func (r *Repository) DiffTrees(oldFiles map[string]FlattenedFile, newFiles map[string]FlattenedFile) (string, error) {
	var deleted []fileEntry
	var added []fileEntry
	var modified []fileEntry

	allPathsMap := make(map[string]bool)
	for p := range oldFiles {
		allPathsMap[p] = true
	}
	for p := range newFiles {
		allPathsMap[p] = true
	}

	for p := range allPathsMap {
		oldF, inOld := oldFiles[p]
		newF, inNew := newFiles[p]

		if inOld && inNew {
			if oldF.Hash != newF.Hash {
				_, _, _, err1 := object.ReadObject(r.GitDir, oldF.Hash)
				_, newData, _, err2 := object.ReadObject(r.GitDir, newF.Hash)
				if err1 == nil && err2 == nil {
					modified = append(modified, fileEntry{path: p, mode: newF.Mode, hash: newF.Hash, data: newData})
				}
			}
		} else if inOld && !inNew {
			_, oldData, _, err := object.ReadObject(r.GitDir, oldF.Hash)
			if err == nil {
				deleted = append(deleted, fileEntry{path: p, mode: oldF.Mode, hash: oldF.Hash, data: oldData})
			}
		} else if !inOld && inNew {
			_, newData, _, err := object.ReadObject(r.GitDir, newF.Hash)
			if err == nil {
				added = append(added, fileEntry{path: p, mode: newF.Mode, hash: newF.Hash, data: newData})
			}
		}
	}

	deletedMatched := make(map[string]bool)
	addedMatched := make(map[string]bool)
	var renames []renamePair

	for _, del := range deleted {
		if deletedMatched[del.path] {
			continue
		}
		for _, add := range added {
			if addedMatched[add.path] {
				continue
			}
			if del.hash == add.hash || bytes.Equal(del.data, add.data) {
				renames = append(renames, renamePair{oldEntry: del, newEntry: add, similarity: 100})
				deletedMatched[del.path] = true
				addedMatched[add.path] = true
				break
			}
		}
	}

	type candidate struct {
		del   fileEntry
		add   fileEntry
		score int
	}
	var candidates []candidate
	for _, del := range deleted {
		if deletedMatched[del.path] {
			continue
		}
		for _, add := range added {
			if addedMatched[add.path] {
				continue
			}
			score := diff.ComputeSimilarity(del.data, add.data)
			if score >= 50 {
				candidates = append(candidates, candidate{del: del, add: add, score: score})
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	for _, c := range candidates {
		if deletedMatched[c.del.path] || addedMatched[c.add.path] {
			continue
		}
		renames = append(renames, renamePair{oldEntry: c.del, newEntry: c.add, similarity: c.score})
		deletedMatched[c.del.path] = true
		addedMatched[c.add.path] = true
	}

	var items []diffItem

	for _, rn := range renames {
		content := diff.FormatRenameDiff(rn.oldEntry.path, rn.newEntry.path, rn.similarity, rn.oldEntry.data, rn.newEntry.data)
		items = append(items, diffItem{sortPath: rn.newEntry.path, content: content})
	}

	for _, del := range deleted {
		if !deletedMatched[del.path] {
			content := diff.DiffBytes(del.path, "/dev/null", del.data, nil)
			items = append(items, diffItem{sortPath: del.path, content: content})
		}
	}

	for _, add := range added {
		if !addedMatched[add.path] {
			content := diff.DiffBytes("/dev/null", add.path, nil, add.data)
			items = append(items, diffItem{sortPath: add.path, content: content})
		}
	}

	for _, mod := range modified {
		oldF := oldFiles[mod.path]
		_, oldData, _, _ := object.ReadObject(r.GitDir, oldF.Hash)
		content := diff.DiffBytes(mod.path, mod.path, oldData, mod.data)
		items = append(items, diffItem{sortPath: mod.path, content: content})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].sortPath < items[j].sortPath
	})

	var sb strings.Builder
	for _, it := range items {
		sb.WriteString(it.content)
	}

	return sb.String(), nil
}

func (r *Repository) DiffCommits(commitA, commitB string) (string, error) {
	hashA, err := r.ResolveRevision(commitA)
	if err != nil {
		return "", err
	}
	hashB, err := r.ResolveRevision(commitB)
	if err != nil {
		return "", err
	}

	cA, err := r.LoadCommit(hashA)
	if err != nil {
		return "", err
	}
	cB, err := r.LoadCommit(hashB)
	if err != nil {
		return "", err
	}

	filesA, err := r.FlattenTree(cA.TreeHash, "")
	if err != nil {
		return "", err
	}
	filesB, err := r.FlattenTree(cB.TreeHash, "")
	if err != nil {
		return "", err
	}

	return r.DiffTrees(filesA, filesB)
}

func (r *Repository) DiffCommit(rev string) (string, error) {
	hash, err := r.ResolveRevision(rev)
	if err != nil {
		return "", err
	}
	c, err := r.LoadCommit(hash)
	if err != nil {
		return "", err
	}
	commitFiles, err := r.FlattenTree(c.TreeHash, "")
	if err != nil {
		return "", err
	}

	idx, _ := r.LoadIndex()
	allPathsMap := make(map[string]bool)
	for p := range commitFiles {
		allPathsMap[p] = true
	}
	if idx != nil {
		for _, e := range idx.Entries {
			if e.Stage() == 0 {
				allPathsMap[e.Path] = true
			}
		}
	}

	var allPaths []string
	for p := range allPathsMap {
		allPaths = append(allPaths, p)
	}
	sort.Strings(allPaths)

	var sb strings.Builder
	for _, p := range allPaths {
		oldF, inCommit := commitFiles[p]
		filePath := filepath.Join(r.WorkDir, filepath.FromSlash(p))

		var workData []byte
		var exists bool
		info, err := os.Lstat(filePath)
		if err == nil {
			exists = true
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(filePath)
				if err == nil {
					workData = []byte(target)
				}
			} else {
				data, err := os.ReadFile(filePath)
				if err == nil {
					workData = data
				}
			}
		}

		if inCommit && !exists {
			_, oldData, _, err := object.ReadObject(r.GitDir, oldF.Hash)
			if err == nil {
				d := diff.DiffBytes(p, "/dev/null", oldData, nil)
				sb.WriteString(d)
			}
		} else if !inCommit && exists {
			d := diff.DiffBytes("/dev/null", p, nil, workData)
			sb.WriteString(d)
		} else if inCommit && exists {
			_, oldData, _, err := object.ReadObject(r.GitDir, oldF.Hash)
			if err == nil && !bytes.Equal(oldData, workData) {
				d := diff.DiffBytes(p, p, oldData, workData)
				sb.WriteString(d)
			}
		}
	}

	return sb.String(), nil
}

func (r *Repository) Diff(staged bool) (string, error) {
	idx, err := r.LoadIndex()
	if err != nil {
		return "", err
	}

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

		unmergedPaths := make(map[string]bool)
		for _, entry := range idx.Entries {
			if entry.Stage() != 0 {
				unmergedPaths[entry.Path] = true
			}
		}

		filteredHead := make(map[string]FlattenedFile)
		for p, f := range headFiles {
			if !unmergedPaths[p] {
				filteredHead[p] = f
			}
		}

		idxFiles := make(map[string]FlattenedFile)
		for _, entry := range idx.Entries {
			if entry.Stage() == 0 && !unmergedPaths[entry.Path] {
				idxFiles[entry.Path] = FlattenedFile{
					Mode: fmt.Sprintf("%o", entry.Mode),
					Hash: hex.EncodeToString(entry.Sha1[:]),
				}
			}
		}

		return r.DiffTrees(filteredHead, idxFiles)
	}

	var sb strings.Builder
	for _, entry := range idx.Entries {
		if entry.Stage() != 0 {
			continue
		}
		_, data, _, err := object.ReadObject(r.GitDir, hex.EncodeToString(entry.Sha1[:]))
		if err != nil {
			continue
		}

		filePath := filepath.Join(r.WorkDir, filepath.FromSlash(entry.Path))
		var newData []byte
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
			newData = []byte(target)
		} else {
			readData, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			newData = readData
		}

		if isDeleted {
			d := diff.DiffBytes(entry.Path, "/dev/null", data, nil)
			sb.WriteString(d)
		} else if !bytes.Equal(data, newData) {
			d := diff.DiffBytes(entry.Path, entry.Path, data, newData)
			sb.WriteString(d)
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
		resolved, err := r.ResolveRevision(target)
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
	if err := r.checkWorktreeDirty(incomingChanges, "merge"); err != nil {
		return "", err
	}

	idx, hasConflict, err := r.mergeTrees3Way(baseFiles, ourFiles, theirFiles, target)
	if err != nil {
		return "", err
	}

	if hasConflict {
		_ = os.WriteFile(filepath.Join(r.GitDir, "MERGE_HEAD"), []byte(targetCommitHash+"\n"), 0644)
		_ = os.WriteFile(filepath.Join(r.GitDir, "MERGE_MSG"), []byte(fmt.Sprintf("Merge branch '%s'\n", target)), 0644)
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

	authorStr := r.defaultAuthor()
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

func (r *Repository) MergeAbort() error {
	mergeHeadPath := filepath.Join(r.GitDir, "MERGE_HEAD")
	if _, err := os.Stat(mergeHeadPath); os.IsNotExist(err) {
		return errors.New("fatal: There is no merge to abort (MERGE_HEAD missing).")
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

	_ = os.Remove(mergeHeadPath)
	_ = os.Remove(filepath.Join(r.GitDir, "MERGE_MSG"))
	_ = os.Remove(filepath.Join(r.GitDir, "MERGE_MODE"))
	return nil
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

func createStageEntry(path string, stage int, mode string, hash string, size int) index.IndexEntry {
	hBytes, _ := hex.DecodeString(hash)
	var sha [20]byte
	copy(sha[:], hBytes)

	m := uint32(0100644)
	if mode == "100755" || mode == "0100755" {
		m = 0100755
	} else if mode == "120000" || mode == "0120000" {
		m = 0120000
	}

	flags := uint16(stage&0x3) << 12
	pathLen := len(path)
	if pathLen > 0xFFF {
		pathLen = 0xFFF
	}
	flags |= uint16(pathLen)

	return index.IndexEntry{
		Mode:     m,
		FileSize: uint32(size),
		Sha1:     sha,
		Flags:    flags,
		Path:     path,
	}
}

func (r *Repository) defaultAuthor() string {
	author := os.Getenv("GIT_AUTHOR_NAME")
	email := os.Getenv("GIT_AUTHOR_EMAIL")
	if author == "" {
		author = "minigit author"
	}
	if email == "" {
		email = "author@minigit.local"
	}
	return fmt.Sprintf("%s <%s>", author, email)
}

func (r *Repository) resetTree(targetTreeHash string, resetWorkDir bool) error {
	targetFiles, err := r.FlattenTree(targetTreeHash, "")
	if err != nil {
		return err
	}

	if resetWorkDir {
		idx, _ := r.LoadIndex()
		if idx != nil {
			for _, entry := range idx.Entries {
				if _, inTarget := targetFiles[entry.Path]; !inTarget {
					filePath := filepath.Join(r.WorkDir, filepath.FromSlash(entry.Path))
					_ = os.Remove(filePath)
					cleanEmptyDirs(filepath.Dir(filePath), r.WorkDir)
				}
			}
		}
	}

	newIdx := &index.Index{}
	for p, f := range targetFiles {
		filePath := filepath.Join(r.WorkDir, filepath.FromSlash(p))
		_, data, _, err := object.ReadObject(r.GitDir, f.Hash)
		if err != nil {
			return err
		}

		if resetWorkDir {
			if err := r.writeFileToWorkDir(filePath, f.Mode, data); err != nil {
				return err
			}
		}

		hBytes, err := hex.DecodeString(f.Hash)
		if err != nil {
			return err
		}
		var sha [20]byte
		copy(sha[:], hBytes)

		m := uint32(0100644)
		if f.Mode == "100755" || f.Mode == "0100755" {
			m = 0100755
		} else if f.Mode == "120000" || f.Mode == "0120000" {
			m = 0120000
		}

		newIdx.AddOrUpdate(index.IndexEntry{
			Mode:     m,
			FileSize: uint32(len(data)),
			Sha1:     sha,
			Path:     p,
		})
	}

	return r.SaveIndex(newIdx)
}

func (r *Repository) checkWorktreeDirty(incomingChanges map[string]bool, operation string) error {
	st, err := r.Status()
	if err != nil {
		return err
	}
	var dirtyPaths []string
	for _, s := range st.Staged {
		if incomingChanges[s.Path] {
			dirtyPaths = append(dirtyPaths, s.Path)
		}
	}
	for _, u := range st.Unstaged {
		if incomingChanges[u.Path] {
			dirtyPaths = append(dirtyPaths, u.Path)
		}
	}
	for _, um := range st.Unmerged {
		if incomingChanges[um.Path] {
			dirtyPaths = append(dirtyPaths, um.Path)
		}
	}
	if len(dirtyPaths) > 0 {
		sort.Strings(dirtyPaths)
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("error: Your local changes to the following files would be overwritten by %s:\n", operation))
		for _, p := range dirtyPaths {
			sb.WriteString(fmt.Sprintf("\t%s\n", p))
		}
		sb.WriteString(fmt.Sprintf("Please commit your changes or stash them before you %s.\nAborting", operation))
		return errors.New(sb.String())
	}
	return nil
}

func (r *Repository) mergeTrees3Way(baseFiles, ourFiles, theirFiles map[string]FlattenedFile, labelTheirs string) (*index.Index, bool, error) {
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
		idx = &index.Index{}
	}

	for p, f := range ourFiles {
		if !idx.Has(p) {
			hBytes, _ := hex.DecodeString(f.Hash)
			var sha [20]byte
			copy(sha[:], hBytes)
			m := uint32(0100644)
			if f.Mode == "100755" {
				m = 0100755
			} else if f.Mode == "120000" {
				m = 0120000
			}
			idx.AddOrUpdate(index.IndexEntry{Mode: m, Sha1: sha, Path: p})
		}
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
				return nil, false, err
			}
			if err := r.writeFileToWorkDir(filePath, fTheirs.Mode, data); err != nil {
				return nil, false, err
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
			hasConflict = true
			idx.Remove(p)
			_, baseData, _, _ := object.ReadObject(r.GitDir, fBase.Hash)
			_, theirData, _, _ := object.ReadObject(r.GitDir, fTheirs.Hash)
			idx.AddOrUpdate(createStageEntry(p, 1, fBase.Mode, fBase.Hash, len(baseData)))
			idx.AddOrUpdate(createStageEntry(p, 3, fTheirs.Mode, fTheirs.Hash, len(theirData)))
			_ = r.writeFileToWorkDir(filePath, fTheirs.Mode, theirData)
			continue
		}
		if inBase && inOurs && !inTheirs {
			if fBase.Hash == fOurs.Hash {
				_ = os.Remove(filePath)
				cleanEmptyDirs(filepath.Dir(filePath), r.WorkDir)
				idx.Remove(p)
				continue
			}
			hasConflict = true
			idx.Remove(p)
			_, baseData, _, _ := object.ReadObject(r.GitDir, fBase.Hash)
			_, ourData, _, _ := object.ReadObject(r.GitDir, fOurs.Hash)
			idx.AddOrUpdate(createStageEntry(p, 1, fBase.Mode, fBase.Hash, len(baseData)))
			idx.AddOrUpdate(createStageEntry(p, 2, fOurs.Mode, fOurs.Hash, len(ourData)))
			_ = r.writeFileToWorkDir(filePath, fOurs.Mode, ourData)
			continue
		}
		if inOurs && inTheirs && fOurs.Hash == fTheirs.Hash {
			continue
		}
		if inBase && inOurs && inTheirs {
			if fOurs.Hash == fBase.Hash && fTheirs.Hash != fBase.Hash {
				_, data, _, err := object.ReadObject(r.GitDir, fTheirs.Hash)
				if err != nil {
					return nil, false, err
				}
				if err := r.writeFileToWorkDir(filePath, fTheirs.Mode, data); err != nil {
					return nil, false, err
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

		var baseData, ourData, theirData []byte
		if inBase {
			_, baseData, _, _ = object.ReadObject(r.GitDir, fBase.Hash)
		}
		if inOurs {
			_, ourData, _, _ = object.ReadObject(r.GitDir, fOurs.Hash)
		}
		if inTheirs {
			_, theirData, _, _ = object.ReadObject(r.GitDir, fTheirs.Hash)
		}

		if diff.IsBinary(baseData) || diff.IsBinary(ourData) || diff.IsBinary(theirData) {
			hasConflict = true
			idx.Remove(p)
			if inBase {
				idx.AddOrUpdate(createStageEntry(p, 1, fBase.Mode, fBase.Hash, len(baseData)))
			}
			if inOurs {
				idx.AddOrUpdate(createStageEntry(p, 2, fOurs.Mode, fOurs.Hash, len(ourData)))
				_ = r.writeFileToWorkDir(filePath, fOurs.Mode, ourData)
			}
			if inTheirs {
				idx.AddOrUpdate(createStageEntry(p, 3, fTheirs.Mode, fTheirs.Hash, len(theirData)))
				if !inOurs {
					_ = r.writeFileToWorkDir(filePath, fTheirs.Mode, theirData)
				}
			}
			continue
		}

		mergedContent, fileConflict := diff.Merge3(string(baseData), string(ourData), string(theirData), labelTheirs)
		mode := fOurs.Mode
		if mode == "" {
			mode = fTheirs.Mode
		}
		if err := r.writeFileToWorkDir(filePath, mode, []byte(mergedContent)); err != nil {
			return nil, false, err
		}

		if fileConflict {
			hasConflict = true
			idx.Remove(p)
			if inBase {
				idx.AddOrUpdate(createStageEntry(p, 1, fBase.Mode, fBase.Hash, len(baseData)))
			}
			if inOurs {
				idx.AddOrUpdate(createStageEntry(p, 2, fOurs.Mode, fOurs.Hash, len(ourData)))
			}
			if inTheirs {
				idx.AddOrUpdate(createStageEntry(p, 3, fTheirs.Mode, fTheirs.Hash, len(theirData)))
			}
		} else {
			blobHash, err := object.WriteObject(r.GitDir, object.TypeBlob, []byte(mergedContent))
			if err != nil {
				return nil, false, err
			}
			hBytes, _ := hex.DecodeString(blobHash)
			var sha [20]byte
			copy(sha[:], hBytes)
			m := uint32(0100644)
			if mode == "100755" || mode == "0100755" {
				m = 0100755
			} else if mode == "120000" || mode == "0120000" {
				m = 0120000
			}
			idx.AddOrUpdate(index.IndexEntry{
				Mode:     m,
				FileSize: uint32(len(mergedContent)),
				Sha1:     sha,
				Path:     p,
			})
		}
	}

	return idx, hasConflict, nil
}
