package persist

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"minidatalake/internal/apperr"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func ReadTable(path string) (*storage.Table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() < int64(HeaderN+12) {
		return nil, apperr.New(apperr.TableCorrupted, 409, "mdl file too small")
	}

	hdr := make([]byte, HeaderN)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return nil, err
	}
	if string(hdr[0:4]) != Magic {
		return nil, apperr.New(apperr.TableCorrupted, 409, "bad mdl magic")
	}
	ver := u16(hdr[4:])
	if ver != Version {
		return nil, apperr.New(apperr.TableCorrupted, 409, fmt.Sprintf("unsupported mdl version %d", ver))
	}
	rows := int(u64(hdr[8:]))

	tr, err := readTrailer(f, st.Size())
	if err != nil {
		return nil, err
	}

	t := &storage.Table{Rows: rows, Status: "READY"}
	for _, c := range tr.Cols {
		seg := make([]byte, c.Length)
		if _, err := f.ReadAt(seg, int64(c.Offset)); err != nil {
			return nil, err
		}
		if len(seg) < 12 {
			return nil, apperr.New(apperr.TableCorrupted, 409, "short column segment")
		}
		payload := seg[12:]
		if crc(payload) != c.CRC || u32(seg[8:]) != c.CRC {
			return nil, apperr.New(apperr.TableCorrupted, 409, "column crc mismatch: "+c.Name)
		}
		vec, err := decodeVec(c.Type, c.Encoding, payload, rows)
		if err != nil {
			return nil, apperr.New(apperr.TableCorrupted, 409, err.Error())
		}
		t.Cols = append(t.Cols, &storage.Column{
			Meta: storage.ColumnMeta{
				Name: c.Name, Type: types.DataType(c.Type), Encoding: types.Encoding(c.Encoding),
				Rows: rows, Nulls: c.Nulls, RawBytes: c.RawBytes, EncBytes: c.EncBytes,
				Reason: c.Reason, Card: c.Card, AvgRun: c.AvgRun,
			},
			Vec: vec,
		})
	}
	return t, nil
}

func ProbeHeader(path string) (version uint16, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	hdr := make([]byte, HeaderN)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return 0, err
	}
	if string(hdr[0:4]) != Magic {
		return 0, fmt.Errorf("bad magic")
	}
	return u16(hdr[4:]), nil
}

func readTrailer(f *os.File, size int64) (*trailer, error) {
	tail := make([]byte, 12)
	if _, err := f.ReadAt(tail, size-12); err != nil {
		return nil, err
	}
	if string(tail[8:12]) != MagicEnd {
		return nil, apperr.New(apperr.TableCorrupted, 409, "bad mdl trailer magic")
	}
	n := int(u32(tail[4:8]))
	if n <= 0 || int64(n)+12 > size {
		return nil, apperr.New(apperr.TableCorrupted, 409, "bad trailer length")
	}
	body := make([]byte, n)
	if _, err := f.ReadAt(body, size-12-int64(n)); err != nil {
		return nil, err
	}
	if crc(body) != u32(tail[0:4]) {
		return nil, apperr.New(apperr.TableCorrupted, 409, "trailer crc mismatch")
	}
	var tr trailer
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, err
	}
	return &tr, nil
}
