package ignore

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Pattern struct {
	BaseDir  string
	Raw      string
	Negate   bool
	DirOnly  bool
	Anchored bool
	Regexp   *regexp.Regexp
}

type Matcher struct {
	patterns []Pattern
}

func NewMatcher() *Matcher {
	return &Matcher{}
}

func patternToRegex(pattern string, anchored bool) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("^")
	if !anchored {
		sb.WriteString("(?:.*/)?")
	}

	i := 0
	n := len(pattern)
	for i < n {
		c := pattern[i]
		if c == '*' {
			if i+1 < n && pattern[i+1] == '*' {
				if i+2 < n && pattern[i+2] == '/' {
					sb.WriteString("(?:.+/)?")
					i += 3
					continue
				} else if i > 0 && pattern[i-1] == '/' && i+2 == n {
					sb.WriteString(".*")
					i += 2
					continue
				} else {
					sb.WriteString(".*")
					i += 2
					continue
				}
			} else {
				sb.WriteString("[^/]*")
				i++
				continue
			}
		} else if c == '?' {
			sb.WriteString("[^/]")
			i++
			continue
		} else if c == '[' {
			j := i + 1
			if j < n && pattern[j] == '!' {
				j++
			}
			if j < n && pattern[j] == ']' {
				j++
			}
			for j < n && pattern[j] != ']' {
				j++
			}
			if j < n {
				sub := pattern[i : j+1]
				if len(sub) > 2 && sub[1] == '!' {
					sub = "[^" + sub[2:]
				}
				sb.WriteString(sub)
				i = j + 1
				continue
			} else {
				sb.WriteString("\\[")
				i++
				continue
			}
		} else if c == '\\' {
			if i+1 < n {
				i++
				sb.WriteString(regexp.QuoteMeta(string(pattern[i])))
				i++
				continue
			}
			sb.WriteString("\\\\")
			i++
			continue
		} else if c == '.' || c == '+' || c == '(' || c == ')' || c == '{' || c == '}' || c == '^' || c == '$' || c == '|' {
			sb.WriteString("\\" + string(c))
			i++
			continue
		} else {
			sb.WriteByte(c)
			i++
			continue
		}
	}

	sb.WriteString("$")
	return regexp.Compile(sb.String())
}

func ParseLine(line, baseDir string) (*Pattern, error) {
	line = strings.TrimRight(line, "\r")
	if len(line) == 0 {
		return nil, nil
	}

	for len(line) > 0 && (line[len(line)-1] == ' ' || line[len(line)-1] == '\t') {
		if len(line) >= 2 && line[len(line)-2] == '\\' {
			line = line[:len(line)-2] + " "
			break
		}
		line = line[:len(line)-1]
	}

	if len(line) == 0 || line[0] == '#' {
		return nil, nil
	}

	negate := false
	if line[0] == '!' {
		negate = true
		line = line[1:]
	} else if strings.HasPrefix(line, "\\!") || strings.HasPrefix(line, "\\#") {
		line = line[1:]
	}

	if len(line) == 0 {
		return nil, nil
	}

	dirOnly := false
	if strings.HasSuffix(line, "/") {
		dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	anchored := false
	if strings.HasPrefix(line, "/") {
		anchored = true
		line = strings.TrimPrefix(line, "/")
	} else if strings.Contains(line, "/") {
		anchored = true
	}

	rx, err := patternToRegex(line, anchored)
	if err != nil {
		return nil, err
	}

	return &Pattern{
		BaseDir:  baseDir,
		Raw:      line,
		Negate:   negate,
		DirOnly:  dirOnly,
		Anchored: anchored,
		Regexp:   rx,
	}, nil
}

func LoadMatcher(workDir string) (*Matcher, error) {
	m := NewMatcher()
	err := filepath.WalkDir(workDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == ".gitignore" {
			relDir, err := filepath.Rel(workDir, filepath.Dir(path))
			if err != nil {
				return err
			}
			if relDir == "." {
				relDir = ""
			} else {
				relDir = filepath.ToSlash(relDir)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			lines := strings.Split(string(data), "\n")
			for _, l := range lines {
				pat, err := ParseLine(l, relDir)
				if err == nil && pat != nil {
					m.patterns = append(m.patterns, *pat)
				}
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.SliceStable(m.patterns, func(i, j int) bool {
		depthI := 0
		if m.patterns[i].BaseDir != "" {
			depthI = strings.Count(m.patterns[i].BaseDir, "/") + 1
		}
		depthJ := 0
		if m.patterns[j].BaseDir != "" {
			depthJ = strings.Count(m.patterns[j].BaseDir, "/") + 1
		}
		return depthI < depthJ
	})

	return m, nil
}

func (m *Matcher) AddPattern(p Pattern) {
	m.patterns = append(m.patterns, p)
}

func (m *Matcher) matchSingle(gitPath string, isDir bool) (bool, bool) {
	matched := false
	ignored := false

	for _, pat := range m.patterns {
		if pat.DirOnly && !isDir {
			continue
		}

		target := gitPath
		if pat.BaseDir != "" {
			if gitPath != pat.BaseDir && !strings.HasPrefix(gitPath, pat.BaseDir+"/") {
				continue
			}
			target = strings.TrimPrefix(gitPath, pat.BaseDir+"/")
		}

		if pat.Regexp.MatchString(target) {
			matched = true
			ignored = !pat.Negate
		}
	}

	return matched, ignored
}

func (m *Matcher) IsIgnored(gitPath string, isDir bool) bool {
	gitPath = strings.Trim(gitPath, "/")
	if gitPath == "" || gitPath == "." {
		return false
	}

	parts := strings.Split(gitPath, "/")
	for i := 1; i < len(parts); i++ {
		parentDir := strings.Join(parts[:i], "/")
		matched, ignored := m.matchSingle(parentDir, true)
		if matched && ignored {
			return true
		}
	}

	matched, ignored := m.matchSingle(gitPath, isDir)
	if matched && ignored {
		return true
	}

	return false
}
