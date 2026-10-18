package pack

import (
	"bytes"
	"testing"
)

func TestApplyDelta(t *testing.T) {
	base := []byte("The quick brown fox jumps over the lazy dog")

	var deltaBuf bytes.Buffer
	deltaBuf.WriteByte(byte(len(base)))
	target := "The fast brown fox jumps over the lazy dog and runs away"
	deltaBuf.WriteByte(byte(len(target)))

	deltaBuf.WriteByte(0x91)
	deltaBuf.WriteByte(0)
	deltaBuf.WriteByte(4)

	deltaBuf.WriteByte(5)
	deltaBuf.WriteString("fast ")

	deltaBuf.WriteByte(0x91)
	deltaBuf.WriteByte(10)
	deltaBuf.WriteByte(33)

	deltaBuf.WriteByte(14)
	deltaBuf.WriteString(" and runs away")

	out, err := applyDelta(base, deltaBuf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if string(out) != target {
		t.Fatalf("expected %q, got %q", target, string(out))
	}
}

func TestApplyDeltaOutOfBounds(t *testing.T) {
	base := []byte("hello")
	var deltaBuf bytes.Buffer
	deltaBuf.WriteByte(byte(len(base)))
	deltaBuf.WriteByte(byte(10))

	deltaBuf.WriteByte(0x91)
	deltaBuf.WriteByte(10)
	deltaBuf.WriteByte(5)

	_, err := applyDelta(base, deltaBuf.Bytes())
	if err == nil {
		t.Fatalf("expected error for out of bounds copy")
	}
}

func TestReadVarInt(t *testing.T) {
	buf := bytes.NewReader([]byte{0x80 | 0x05, 0x02})
	val, err := readVarInt(buf)
	if err != nil {
		t.Fatal(err)
	}
	expected := int64(5 | (2 << 7))
	if val != expected {
		t.Fatalf("expected %d, got %d", expected, val)
	}
}
