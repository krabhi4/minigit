package object

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"minigit/pkg/pack"
)

const (
	TypeBlob   = "blob"
	TypeTree   = "tree"
	TypeCommit = "commit"
	TypeTag    = "tag"
)

type TreeEntry struct {
	Mode string
	Name string
	Hash string
}

type Tree struct {
	Entries []TreeEntry
}

func (t *Tree) Sort() {
	sort.Slice(t.Entries, func(i, j int) bool {
		nameI := t.Entries[i].Name
		if t.Entries[i].Mode == "40000" || t.Entries[i].Mode == "040000" {
			nameI += "/"
		}
		nameJ := t.Entries[j].Name
		if t.Entries[j].Mode == "40000" || t.Entries[j].Mode == "040000" {
			nameJ += "/"
		}
		return nameI < nameJ
	})
}

func (t *Tree) Serialize() []byte {
	t.Sort()
	var buf bytes.Buffer
	for _, entry := range t.Entries {
		hashBytes, err := hex.DecodeString(entry.Hash)
		if err != nil {
			continue
		}
		buf.WriteString(entry.Mode)
		buf.WriteByte(' ')
		buf.WriteString(entry.Name)
		buf.WriteByte(0)
		buf.Write(hashBytes)
	}
	return buf.Bytes()
}

func ParseTree(data []byte) (*Tree, error) {
	var entries []TreeEntry
	idx := 0
	for idx < len(data) {
		spaceIdx := bytes.IndexByte(data[idx:], ' ')
		if spaceIdx == -1 {
			return nil, errors.New("malformed tree entry: missing space")
		}
		mode := string(data[idx : idx+spaceIdx])
		idx += spaceIdx + 1

		nullIdx := bytes.IndexByte(data[idx:], 0)
		if nullIdx == -1 {
			return nil, errors.New("malformed tree entry: missing null byte")
		}
		name := string(data[idx : idx+nullIdx])
		idx += nullIdx + 1

		if idx+20 > len(data) {
			return nil, errors.New("malformed tree entry: truncated hash")
		}
		hash := hex.EncodeToString(data[idx : idx+20])
		idx += 20

		entries = append(entries, TreeEntry{
			Mode: mode,
			Name: name,
			Hash: hash,
		})
	}
	return &Tree{Entries: entries}, nil
}

type Commit struct {
	TreeHash      string
	ParentHashes  []string
	Author        string
	AuthorTime    time.Time
	Committer     string
	CommitterTime time.Time
	Message       string
}

func (c *Commit) Serialize() []byte {
	var buf bytes.Buffer
	buf.WriteString("tree ")
	buf.WriteString(c.TreeHash)
	buf.WriteByte('\n')

	for _, parent := range c.ParentHashes {
		buf.WriteString("parent ")
		buf.WriteString(parent)
		buf.WriteByte('\n')
	}

	authorTime := c.AuthorTime
	if authorTime.IsZero() {
		authorTime = time.Now()
	}
	committerTime := c.CommitterTime
	if committerTime.IsZero() {
		committerTime = authorTime
	}

	_, authorOffset := authorTime.Zone()
	authorTZ := FormatTimezone(authorOffset)

	_, committerOffset := committerTime.Zone()
	committerTZ := FormatTimezone(committerOffset)

	buf.WriteString(fmt.Sprintf("author %s %d %s\n", c.Author, authorTime.Unix(), authorTZ))
	buf.WriteString(fmt.Sprintf("committer %s %d %s\n", c.Committer, committerTime.Unix(), committerTZ))
	buf.WriteByte('\n')
	buf.WriteString(strings.TrimRight(c.Message, "\n"))
	buf.WriteByte('\n')

	return buf.Bytes()
}

func ParseCommit(data []byte) (*Commit, error) {
	parts := bytes.SplitN(data, []byte("\n\n"), 2)
	headerLines := strings.Split(string(parts[0]), "\n")

	c := &Commit{}
	if len(parts) > 1 {
		c.Message = string(parts[1])
	}

	for _, line := range headerLines {
		if strings.HasPrefix(line, "tree ") {
			c.TreeHash = strings.TrimPrefix(line, "tree ")
		} else if strings.HasPrefix(line, "parent ") {
			c.ParentHashes = append(c.ParentHashes, strings.TrimPrefix(line, "parent "))
		} else if strings.HasPrefix(line, "author ") {
			authorStr := strings.TrimPrefix(line, "author ")
			name, t, err := parseIdentity(authorStr)
			if err == nil {
				c.Author = name
				c.AuthorTime = t
			}
		} else if strings.HasPrefix(line, "committer ") {
			committerStr := strings.TrimPrefix(line, "committer ")
			name, t, err := parseIdentity(committerStr)
			if err == nil {
				c.Committer = name
				c.CommitterTime = t
			}
		}
	}

	return c, nil
}

func FormatTimezone(offsetSeconds int) string {
	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}
	hours := offsetSeconds / 3600
	minutes := (offsetSeconds % 3600) / 60
	return fmt.Sprintf("%s%02d%02d", sign, hours, minutes)
}

func parseIdentity(identity string) (string, time.Time, error) {
	lastAngle := strings.LastIndex(identity, ">")
	if lastAngle == -1 {
		return identity, time.Time{}, errors.New("missing email in identity")
	}
	nameAndEmail := identity[:lastAngle+1]
	rest := strings.TrimSpace(identity[lastAngle+1:])
	timeParts := strings.Fields(rest)
	if len(timeParts) < 1 {
		return nameAndEmail, time.Time{}, nil
	}
	unixSec, err := strconv.ParseInt(timeParts[0], 10, 64)
	if err != nil {
		return nameAndEmail, time.Time{}, err
	}
	t := time.Unix(unixSec, 0)
	return nameAndEmail, t, nil
}

func HashObject(objType string, data []byte) (string, []byte) {
	header := fmt.Sprintf("%s %d\x00", objType, len(data))
	full := append([]byte(header), data...)
	sum := sha1.Sum(full)
	hash := hex.EncodeToString(sum[:])
	return hash, full
}

func WriteObject(gitDir string, objType string, data []byte) (string, error) {
	hash, fullData := HashObject(objType, data)
	objDir := filepath.Join(gitDir, "objects", hash[:2])
	objPath := filepath.Join(objDir, hash[2:])

	if _, err := os.Stat(objPath); err == nil {
		return hash, nil
	}

	if err := os.MkdirAll(objDir, 0755); err != nil {
		return "", err
	}

	tmpFile, err := os.CreateTemp(objDir, "tmpobj_*")
	if err != nil {
		return "", err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	zw := zlib.NewWriter(tmpFile)
	if _, err := zw.Write(fullData); err != nil {
		tmpFile.Close()
		return "", err
	}
	if err := zw.Close(); err != nil {
		tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	if err := os.Rename(tmpName, objPath); err != nil {
		return "", err
	}
	return hash, nil
}

func ReadObject(gitDir string, hashPrefix string) (string, []byte, string, error) {
	fullHash, err := ResolveObjectHash(gitDir, hashPrefix)
	if err != nil {
		return "", nil, "", err
	}

	objPath := filepath.Join(gitDir, "objects", fullHash[:2], fullHash[2:])
	f, err := os.Open(objPath)
	if err != nil {
		typeName, content, packErr := pack.ReadObjectFromPacks(gitDir, fullHash)
		if packErr == nil {
			return typeName, content, fullHash, nil
		}
		return "", nil, "", err
	}
	defer f.Close()

	zr, err := zlib.NewReader(f)
	if err != nil {
		return "", nil, "", err
	}
	defer zr.Close()

	decompressed, err := io.ReadAll(zr)
	if err != nil {
		return "", nil, "", err
	}

	nullIdx := bytes.IndexByte(decompressed, 0)
	if nullIdx == -1 {
		return "", nil, "", errors.New("malformed object: missing null header separator")
	}

	header := string(decompressed[:nullIdx])
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return "", nil, "", errors.New("malformed object header")
	}

	objType := parts[0]
	content := decompressed[nullIdx+1:]
	return objType, content, fullHash, nil
}

func ResolveObjectHash(gitDir string, prefix string) (string, error) {
	if len(prefix) < 4 {
		return "", fmt.Errorf("prefix %s is too short", prefix)
	}

	matchMap := make(map[string]bool)

	if len(prefix) == 40 {
		objPath := filepath.Join(gitDir, "objects", prefix[:2], prefix[2:])
		if _, err := os.Stat(objPath); err == nil {
			return prefix, nil
		}
	} else {
		objDir := filepath.Join(gitDir, "objects", prefix[:2])
		entries, err := os.ReadDir(objDir)
		if err == nil {
			rest := prefix[2:]
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), rest) {
					matchMap[prefix[:2]+entry.Name()] = true
				}
			}
		}
	}

	packMatches, _ := pack.FindObjectInPacks(gitDir, prefix)
	for _, m := range packMatches {
		matchMap[m.Hash] = true
	}

	if len(matchMap) == 0 {
		return "", fmt.Errorf("object with prefix %s not found", prefix)
	}
	if len(matchMap) > 1 {
		return "", fmt.Errorf("ambiguous object prefix %s (%d matches)", prefix, len(matchMap))
	}

	for k := range matchMap {
		return k, nil
	}
	return "", fmt.Errorf("object with prefix %s not found", prefix)
}
