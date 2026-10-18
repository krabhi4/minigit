package index

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"minigit/pkg/object"
)

type IndexEntry struct {
	CtimeSec  uint32
	CtimeNano uint32
	MtimeSec  uint32
	MtimeNano uint32
	Dev       uint32
	Ino       uint32
	Mode      uint32
	Uid       uint32
	Gid       uint32
	FileSize  uint32
	Sha1      [20]byte
	Flags     uint16
	Path      string
}

func (e IndexEntry) Stage() int {
	return int((e.Flags >> 12) & 0x3)
}

type Index struct {
	Entries []IndexEntry
}

func (idx *Index) HasConflicts() bool {
	for _, entry := range idx.Entries {
		if entry.Stage() != 0 {
			return true
		}
	}
	return false
}

func (idx *Index) Find(path string) (int, *IndexEntry) {
	for i := range idx.Entries {
		if idx.Entries[i].Path == path {
			return i, &idx.Entries[i]
		}
	}
	return -1, nil
}

func (idx *Index) AddOrUpdate(entry IndexEntry) {
	entryStage := entry.Stage()
	if entryStage == 0 {
		idx.Remove(entry.Path)
		idx.Entries = append(idx.Entries, entry)
		idx.Sort()
		return
	}
	for i := range idx.Entries {
		if idx.Entries[i].Path == entry.Path && idx.Entries[i].Stage() == entryStage {
			idx.Entries[i] = entry
			idx.Sort()
			return
		}
	}
	idx.Entries = append(idx.Entries, entry)
	idx.Sort()
}

func (idx *Index) Remove(path string) bool {
	found := false
	filtered := idx.Entries[:0]
	for _, entry := range idx.Entries {
		if entry.Path == path {
			found = true
			continue
		}
		filtered = append(filtered, entry)
	}
	idx.Entries = filtered
	return found
}

func (idx *Index) Sort() {
	sort.Slice(idx.Entries, func(i, j int) bool {
		if idx.Entries[i].Path != idx.Entries[j].Path {
			return idx.Entries[i].Path < idx.Entries[j].Path
		}
		stageI := (idx.Entries[i].Flags >> 12) & 0x3
		stageJ := (idx.Entries[j].Flags >> 12) & 0x3
		return stageI < stageJ
	})
}

func ReadIndex(indexPath string) (*Index, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Index{Entries: []IndexEntry{}}, nil
		}
		return nil, err
	}

	if len(data) < 32 {
		return nil, errors.New("index file too small")
	}

	expectedSha := sha1.Sum(data[:len(data)-20])
	actualSha := data[len(data)-20:]
	if !bytes.Equal(expectedSha[:], actualSha) {
		return nil, errors.New("index file checksum mismatch")
	}

	sig := string(data[:4])
	if sig != "DIRC" {
		return nil, fmt.Errorf("invalid index signature: %s", sig)
	}

	version := binary.BigEndian.Uint32(data[4:8])
	if version != 2 && version != 3 {
		return nil, fmt.Errorf("unsupported index version: %d", version)
	}

	entryCount := int(binary.BigEndian.Uint32(data[8:12]))
	idx := &Index{
		Entries: make([]IndexEntry, 0, entryCount),
	}

	pos := 12
	dataLen := len(data) - 20
	for i := 0; i < entryCount; i++ {
		if pos+62 > dataLen {
			return nil, errors.New("unexpected EOF reading index entry")
		}

		entryStart := pos
		entry := IndexEntry{
			CtimeSec:  binary.BigEndian.Uint32(data[pos : pos+4]),
			CtimeNano: binary.BigEndian.Uint32(data[pos+4 : pos+8]),
			MtimeSec:  binary.BigEndian.Uint32(data[pos+8 : pos+12]),
			MtimeNano: binary.BigEndian.Uint32(data[pos+12 : pos+16]),
			Dev:       binary.BigEndian.Uint32(data[pos+16 : pos+20]),
			Ino:       binary.BigEndian.Uint32(data[pos+20 : pos+24]),
			Mode:      binary.BigEndian.Uint32(data[pos+24 : pos+28]),
			Uid:       binary.BigEndian.Uint32(data[pos+28 : pos+32]),
			Gid:       binary.BigEndian.Uint32(data[pos+32 : pos+36]),
			FileSize:  binary.BigEndian.Uint32(data[pos+36 : pos+40]),
		}
		copy(entry.Sha1[:], data[pos+40:pos+60])
		entry.Flags = binary.BigEndian.Uint16(data[pos+60 : pos+62])

		pos += 62
		nullIdx := bytes.IndexByte(data[pos:dataLen], 0)
		if nullIdx == -1 {
			return nil, errors.New("unterminated path in index entry")
		}

		entry.Path = string(data[pos : pos+nullIdx])
		pos += nullIdx + 1

		rawLen := pos - entryStart
		pad := (8 - (rawLen % 8)) % 8
		pos += pad

		idx.Entries = append(idx.Entries, entry)
	}

	return idx, nil
}

func (idx *Index) Write(indexPath string) error {
	idx.Sort()
	var buf bytes.Buffer

	buf.WriteString("DIRC")
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], 2)
	binary.BigEndian.PutUint32(hdr[4:8], uint32(len(idx.Entries)))
	buf.Write(hdr[:])

	for _, entry := range idx.Entries {
		entryStart := buf.Len()
		var fixed [62]byte
		binary.BigEndian.PutUint32(fixed[0:4], entry.CtimeSec)
		binary.BigEndian.PutUint32(fixed[4:8], entry.CtimeNano)
		binary.BigEndian.PutUint32(fixed[8:12], entry.MtimeSec)
		binary.BigEndian.PutUint32(fixed[12:16], entry.MtimeNano)
		binary.BigEndian.PutUint32(fixed[16:20], entry.Dev)
		binary.BigEndian.PutUint32(fixed[20:24], entry.Ino)
		binary.BigEndian.PutUint32(fixed[24:28], entry.Mode)
		binary.BigEndian.PutUint32(fixed[28:32], entry.Uid)
		binary.BigEndian.PutUint32(fixed[32:36], entry.Gid)
		binary.BigEndian.PutUint32(fixed[36:40], entry.FileSize)
		copy(fixed[40:60], entry.Sha1[:])

		flags := entry.Flags
		pathLen := len(entry.Path)
		if pathLen > 0xFFF {
			pathLen = 0xFFF
		}
		flags = (flags & 0xF000) | uint16(pathLen)
		binary.BigEndian.PutUint16(fixed[60:62], flags)

		buf.Write(fixed[:])
		buf.WriteString(entry.Path)
		buf.WriteByte(0)

		rawLen := buf.Len() - entryStart
		pad := (8 - (rawLen % 8)) % 8
		for p := 0; p < pad; p++ {
			buf.WriteByte(0)
		}
	}

	sum := sha1.Sum(buf.Bytes())
	buf.Write(sum[:])

	dir := filepath.Dir(indexPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "index_*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmpFile, &buf); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, indexPath)
}

type treeBuilderNode struct {
	subdirs map[string]*treeBuilderNode
	files   map[string]IndexEntry
}

func newTreeBuilderNode() *treeBuilderNode {
	return &treeBuilderNode{
		subdirs: make(map[string]*treeBuilderNode),
		files:   make(map[string]IndexEntry),
	}
}

func (idx *Index) BuildTree(gitDir string) (string, error) {
	for _, entry := range idx.Entries {
		if entry.Stage() != 0 {
			return "", fmt.Errorf("cannot write tree with unmerged index entries: %s", entry.Path)
		}
	}
	root := newTreeBuilderNode()
	for _, entry := range idx.Entries {
		parts := strings.Split(entry.Path, "/")
		curr := root
		for i := 0; i < len(parts)-1; i++ {
			part := parts[i]
			if _, exists := curr.subdirs[part]; !exists {
				curr.subdirs[part] = newTreeBuilderNode()
			}
			curr = curr.subdirs[part]
		}
		curr.files[parts[len(parts)-1]] = entry
	}
	return writeTreeNode(gitDir, root)
}

func writeTreeNode(gitDir string, node *treeBuilderNode) (string, error) {
	var entries []object.TreeEntry

	for dirName, subNode := range node.subdirs {
		subHash, err := writeTreeNode(gitDir, subNode)
		if err != nil {
			return "", err
		}
		entries = append(entries, object.TreeEntry{
			Mode: "40000",
			Name: dirName,
			Hash: subHash,
		})
	}

	for fileName, entry := range node.files {
		modeStr := fmt.Sprintf("%o", entry.Mode)
		entries = append(entries, object.TreeEntry{
			Mode: modeStr,
			Name: fileName,
			Hash: hex.EncodeToString(entry.Sha1[:]),
		})
	}

	tree := &object.Tree{Entries: entries}
	data := tree.Serialize()
	return object.WriteObject(gitDir, object.TypeTree, data)
}
