package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"minidatalake/internal/storage"
)

type colTrailer struct {
	Name     string `json:"name"`
	Type     uint8  `json:"type"`
	Encoding uint8  `json:"encoding"`
	Offset   uint64 `json:"offset"`
	Length   uint32 `json:"length"`
	CRC      uint32 `json:"crc"`
	Nulls    int    `json:"nulls"`
	RawBytes int64  `json:"raw_bytes"`
	EncBytes int64  `json:"enc_bytes"`
	Reason   string `json:"reason"`
	Card     int    `json:"card"`
	AvgRun   float64 `json:"avg_run"`
}

type trailer struct {
	Cols     []colTrailer `json:"cols"`
	Rows     int          `json:"rows"`
	Version  uint16       `json:"version"`
}

func WriteTable(path string, t *storage.Table) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			// Best-effort cleanup of the temp file. The removal error must not
			// overwrite the real error that caused ok to stay false (e.g. a
			// failed rename because the target path is an existing directory).
			_ = os.Remove(tmp)
		}
	}()

	hdr := make([]byte, HeaderN)
	copy(hdr[0:4], Magic)
	putU16(hdr[4:], Version)
	putU64(hdr[8:], uint64(t.Rows))
	putU32(hdr[16:], uint32(len(t.Cols)))
	if _, err := f.Write(hdr); err != nil {
		return err
	}

	tr := trailer{Rows: t.Rows, Version: Version}
	off := int64(HeaderN)
	for _, c := range t.Cols {
		typ, enc, payload, err := encodeVec(c.Vec)
		if err != nil {
			return err
		}
		seg := make([]byte, 12+len(payload))
		seg[0] = typ
		seg[1] = enc
		putU32(seg[4:], uint32(len(payload)))
		sum := crc(payload)
		putU32(seg[8:], sum)
		copy(seg[12:], payload)
		if _, err := f.Write(seg); err != nil {
			return err
		}
		tr.Cols = append(tr.Cols, colTrailer{
			Name: c.Meta.Name, Type: typ, Encoding: enc,
			Offset: uint64(off), Length: uint32(len(seg)), CRC: sum,
			Nulls: c.Meta.Nulls, RawBytes: c.Meta.RawBytes, EncBytes: c.Meta.EncBytes,
			Reason: c.Meta.Reason, Card: c.Meta.Card, AvgRun: c.Meta.AvgRun,
		})
		off += int64(len(seg))
	}

	body, err := json.Marshal(tr)
	if err != nil {
		return err
	}
	sum := crc(body)
	tail := make([]byte, len(body)+12)
	copy(tail, body)
	putU32(tail[len(body):], sum)
	putU32(tail[len(body)+4:], uint32(len(body)))
	copy(tail[len(body)+8:], MagicEnd)
	if _, err := f.Write(tail); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("atomic rename: %w", err)
	}
	ok = true
	return nil
}
