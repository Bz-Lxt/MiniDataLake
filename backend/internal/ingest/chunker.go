package ingest

import (
	"bytes"
	"io"
	"sync"
)

type Chunk struct {
	Index int
	Data  []byte
	Start int64
}

var bufPool = sync.Pool{New: func() any { return bytes.NewBuffer(make([]byte, 0, 64<<10)) }}

func GetBuf() *bytes.Buffer {
	b := bufPool.Get().(*bytes.Buffer)
	b.Reset()
	return b
}

func PutBuf(b *bytes.Buffer) { bufPool.Put(b) }

// SplitCSV splits r into record-aligned chunks. quote-aware; handles CRLF and quoted newlines.
func SplitCSV(r io.ReaderAt, size int64, window int, sep rune) ([]Chunk, error) {
	if window < 4096 {
		window = 4096
	}
	var chunks []Chunk
	var off int64
	idx := 0
	carry := []byte{}
	for off < size {
		need := window
		if off+int64(need) > size {
			need = int(size - off)
		}
		buf := make([]byte, need+len(carry))
		copy(buf, carry)
		n, err := r.ReadAt(buf[len(carry):], off)
		buf = buf[:len(carry)+n]
		if err != nil && err != io.EOF {
			return nil, err
		}
		end := len(buf)
		if off+int64(n) < size {
			cut := lastRecordEnd(buf, sep)
			if cut <= 0 {
				cut = len(buf)
			}
			carry = append([]byte{}, buf[cut:]...)
			buf = buf[:cut]
			end = cut
		} else {
			carry = nil
		}
		if end > 0 {
			chunks = append(chunks, Chunk{Index: idx, Data: buf, Start: off})
			idx++
		}
		off += int64(n)
		if n == 0 {
			break
		}
	}
	if len(carry) > 0 {
		chunks = append(chunks, Chunk{Index: idx, Data: carry, Start: off})
	}
	return chunks, nil
}

func lastRecordEnd(b []byte, sep rune) int {
	_ = sep
	inQ := false
	last := -1
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == '"' {
			if inQ && i+1 < len(b) && b[i+1] == '"' {
				i++
				continue
			}
			inQ = !inQ
			continue
		}
		if !inQ && (c == '\n') {
			last = i + 1
		}
	}
	return last
}

func ParseCSVLine(line []byte, sep rune) []string {
	var out []string
	var cur bytes.Buffer
	inQ := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' {
			if inQ && i+1 < len(line) && line[i+1] == '"' {
				cur.WriteByte('"')
				i++
				continue
			}
			inQ = !inQ
			continue
		}
		if !inQ && rune(c) == sep {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		if !inQ && (c == '\n' || c == '\r') {
			continue
		}
		cur.WriteByte(c)
	}
	out = append(out, cur.String())
	return out
}

func SplitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			line := data[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(data) {
		line := data[start:]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}
