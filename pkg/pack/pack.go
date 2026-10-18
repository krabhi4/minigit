package pack

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	objCommit   = 1
	objTree     = 2
	objBlob     = 3
	objTag      = 4
	objOfsDelta = 6
	objRefDelta = 7
)

type ObjectLocation struct {
	Hash       string
	PackPath   string
	PackOffset int64
}

func ListIdxFiles(gitDir string) ([]string, error) {
	packDir := filepath.Join(gitDir, "objects", "pack")
	entries, err := os.ReadDir(packDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var idxFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".idx") {
			idxFiles = append(idxFiles, filepath.Join(packDir, e.Name()))
		}
	}
	return idxFiles, nil
}

func FindObjectInPacks(gitDir string, hashPrefix string) ([]ObjectLocation, error) {
	idxFiles, err := ListIdxFiles(gitDir)
	if err != nil || len(idxFiles) == 0 {
		return nil, err
	}

	var results []ObjectLocation
	for _, idxPath := range idxFiles {
		packPath := strings.TrimSuffix(idxPath, ".idx") + ".pack"
		if _, err := os.Stat(packPath); err != nil {
			continue
		}

		locs, err := findInIdx(idxPath, packPath, hashPrefix)
		if err != nil {
			continue
		}
		results = append(results, locs...)
	}

	return results, nil
}

func findInIdx(idxPath, packPath, hashPrefix string) ([]ObjectLocation, error) {
	data, err := os.ReadFile(idxPath)
	if err != nil {
		return nil, err
	}

	if len(data) < 1032+20+20 {
		return nil, errors.New("idx file too small")
	}

	if !bytes.Equal(data[:4], []byte("\xfftOc")) {
		return nil, errors.New("unsupported idx magic")
	}

	version := binary.BigEndian.Uint32(data[4:8])
	if version != 2 {
		return nil, fmt.Errorf("unsupported idx version: %d", version)
	}

	totalObjects := int(binary.BigEndian.Uint32(data[1028:1032]))
	if totalObjects == 0 {
		return nil, nil
	}

	shaTableStart := 1032
	shaTableEnd := shaTableStart + totalObjects*20
	crcTableEnd := shaTableEnd + totalObjects*4
	offsetTableStart := crcTableEnd
	offsetTableEnd := offsetTableStart + totalObjects*4
	largeOffsetTableStart := offsetTableEnd

	if len(data) < offsetTableEnd+40 {
		return nil, errors.New("truncated idx file")
	}

	var matches []int

	if len(hashPrefix) == 40 {
		targetBytes, err := hex.DecodeString(hashPrefix)
		if err != nil {
			return nil, err
		}

		firstByte := int(targetBytes[0])
		low := 0
		if firstByte > 0 {
			low = int(binary.BigEndian.Uint32(data[8+(firstByte-1)*4 : 8+firstByte*4]))
		}
		high := int(binary.BigEndian.Uint32(data[8+firstByte*4 : 8+(firstByte+1)*4]))

		for low < high {
			mid := (low + high) / 2
			entrySha := data[shaTableStart+mid*20 : shaTableStart+(mid+1)*20]
			cmp := bytes.Compare(entrySha, targetBytes)
			if cmp == 0 {
				matches = append(matches, mid)
				break
			} else if cmp < 0 {
				low = mid + 1
			} else {
				high = mid
			}
		}
	} else {
		for i := 0; i < totalObjects; i++ {
			entryShaHex := hex.EncodeToString(data[shaTableStart+i*20 : shaTableStart+(i+1)*20])
			if strings.HasPrefix(entryShaHex, hashPrefix) {
				matches = append(matches, i)
			}
		}
	}

	var res []ObjectLocation
	for _, idx := range matches {
		entryShaHex := hex.EncodeToString(data[shaTableStart+idx*20 : shaTableStart+(idx+1)*20])
		offBytes := data[offsetTableStart+idx*4 : offsetTableStart+(idx+1)*4]
		offset32 := binary.BigEndian.Uint32(offBytes)

		var finalOffset int64
		if offset32&0x80000000 == 0 {
			finalOffset = int64(offset32)
		} else {
			largeIdx := int(offset32 & 0x7FFFFFFF)
			largeOffsetBytes := data[largeOffsetTableStart+largeIdx*8 : largeOffsetTableStart+(largeIdx+1)*8]
			finalOffset = int64(binary.BigEndian.Uint64(largeOffsetBytes))
		}

		res = append(res, ObjectLocation{
			Hash:       entryShaHex,
			PackPath:   packPath,
			PackOffset: finalOffset,
		})
	}

	return res, nil
}

func ReadObjectFromPacks(gitDir string, hash string) (string, []byte, error) {
	locs, err := FindObjectInPacks(gitDir, hash)
	if err != nil {
		return "", nil, err
	}
	if len(locs) == 0 {
		return "", nil, fmt.Errorf("object %s not found in packfiles", hash)
	}

	loc := locs[0]
	return ReadObjectFromPackFile(gitDir, loc.PackPath, loc.PackOffset, 0)
}

func ReadObjectFromPackFile(gitDir string, packPath string, offset int64, depth int) (string, []byte, error) {
	if depth > 50 {
		return "", nil, errors.New("delta resolution depth limit exceeded")
	}

	f, err := os.Open(packPath)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", nil, err
	}

	br := bufio.NewReader(f)
	b, err := br.ReadByte()
	if err != nil {
		return "", nil, err
	}

	objType := int((b >> 4) & 0x07)
	size := int64(b & 0x0F)
	shift := uint(4)
	for b&0x80 != 0 {
		b, err = br.ReadByte()
		if err != nil {
			return "", nil, err
		}
		size |= int64(b&0x7F) << shift
		shift += 7
	}

	switch objType {
	case objCommit, objTree, objBlob, objTag:
		zr, err := zlib.NewReader(br)
		if err != nil {
			return "", nil, err
		}
		defer zr.Close()
		content, err := io.ReadAll(zr)
		if err != nil {
			return "", nil, err
		}
		typeName := typeToString(objType)
		return typeName, content, nil

	case objOfsDelta:
		c, err := br.ReadByte()
		if err != nil {
			return "", nil, err
		}
		deltaOffset := int64(c & 0x7F)
		for c&0x80 != 0 {
			c, err = br.ReadByte()
			if err != nil {
				return "", nil, err
			}
			deltaOffset = ((deltaOffset + 1) << 7) | int64(c&0x7F)
		}

		baseOffset := offset - deltaOffset
		baseType, baseContent, err := ReadObjectFromPackFile(gitDir, packPath, baseOffset, depth+1)
		if err != nil {
			return "", nil, err
		}

		zr, err := zlib.NewReader(br)
		if err != nil {
			return "", nil, err
		}
		defer zr.Close()
		deltaData, err := io.ReadAll(zr)
		if err != nil {
			return "", nil, err
		}

		reconstructed, err := applyDelta(baseContent, deltaData)
		if err != nil {
			return "", nil, err
		}
		return baseType, reconstructed, nil

	case objRefDelta:
		var baseSha [20]byte
		if _, err := io.ReadFull(br, baseSha[:]); err != nil {
			return "", nil, err
		}
		baseHash := hex.EncodeToString(baseSha[:])

		zr, err := zlib.NewReader(br)
		if err != nil {
			return "", nil, err
		}
		defer zr.Close()
		deltaData, err := io.ReadAll(zr)
		if err != nil {
			return "", nil, err
		}

		baseType, baseContent, err := ReadObjectFromPacks(gitDir, baseHash)
		if err != nil {
			return "", nil, err
		}

		reconstructed, err := applyDelta(baseContent, deltaData)
		if err != nil {
			return "", nil, err
		}
		return baseType, reconstructed, nil

	default:
		return "", nil, fmt.Errorf("unknown pack object type: %d", objType)
	}
}

func typeToString(t int) string {
	switch t {
	case objCommit:
		return "commit"
	case objTree:
		return "tree"
	case objBlob:
		return "blob"
	case objTag:
		return "tag"
	default:
		return "unknown"
	}
}

func applyDelta(base []byte, delta []byte) ([]byte, error) {
	r := bytes.NewReader(delta)

	_, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	targetLen, err := readVarInt(r)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, targetLen)
	for {
		cmd, err := r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if cmd&0x80 != 0 {
			var offset uint32
			var length uint32
			if cmd&0x01 != 0 {
				b, _ := r.ReadByte()
				offset |= uint32(b)
			}
			if cmd&0x02 != 0 {
				b, _ := r.ReadByte()
				offset |= uint32(b) << 8
			}
			if cmd&0x04 != 0 {
				b, _ := r.ReadByte()
				offset |= uint32(b) << 16
			}
			if cmd&0x08 != 0 {
				b, _ := r.ReadByte()
				offset |= uint32(b) << 24
			}
			if cmd&0x10 != 0 {
				b, _ := r.ReadByte()
				length |= uint32(b)
			}
			if cmd&0x20 != 0 {
				b, _ := r.ReadByte()
				length |= uint32(b) << 8
			}
			if cmd&0x40 != 0 {
				b, _ := r.ReadByte()
				length |= uint32(b) << 16
			}
			if length == 0 {
				length = 0x10000
			}
			if int(offset+length) > len(base) {
				return nil, errors.New("delta copy out of bounds")
			}
			out = append(out, base[offset:offset+length]...)
		} else if cmd > 0 {
			buf := make([]byte, cmd)
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, err
			}
			out = append(out, buf...)
		} else {
			return nil, errors.New("invalid zero delta opcode")
		}
	}
	return out, nil
}

func readVarInt(r io.ByteReader) (int64, error) {
	var val int64
	var shift uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		val |= int64(b&0x7F) << shift
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	return val, nil
}

func HashBytes(objType string, data []byte) string {
	header := fmt.Sprintf("%s %d\x00", objType, len(data))
	full := append([]byte(header), data...)
	sum := sha1.Sum(full)
	return hex.EncodeToString(sum[:])
}
