package persist

import (
	"encoding/binary"
	"fmt"

	"minidatalake/internal/encoding"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func encodeVec(v storage.Vector) (typ, enc uint8, payload []byte, err error) {
	typ = uint8(v.Type())
	enc = uint8(v.Encoding())
	switch x := v.(type) {
	case *storage.Int64Vec:
		payload = packI64(x.Vals(), x.Nulls())
	case *storage.Float64Vec:
		payload = packF64(x.Vals(), x.Nulls())
	case *storage.BoolVec:
		payload = packU8(x.Vals(), x.Nulls())
	case *storage.TimeVec:
		payload = packI64(x.Vals(), x.Nulls())
	case *storage.StrVec:
		payload = packStr(x)
	case *encoding.DictVec:
		payload = packDict(x)
	case *encoding.RLEVec:
		payload = packRLE(x)
	default:
		err = fmt.Errorf("unknown vector type %T", v)
	}
	return
}

func decodeVec(typ, enc uint8, payload []byte, rows int) (storage.Vector, error) {
	t := types.DataType(typ)
	switch types.Encoding(enc) {
	case types.Dict:
		return unpackDict(payload, rows)
	case types.RLE:
		return unpackRLE(payload, t, rows)
	default:
		return unpackPlain(t, payload, rows)
	}
}

func packNull(n *storage.Bitmap) []byte {
	raw := n.Bytes()
	out := make([]byte, 4+len(raw))
	putU32(out, uint32(len(raw)))
	copy(out[4:], raw)
	return out
}

func unpackNull(b []byte, rows int) (*storage.Bitmap, []byte, error) {
	if len(b) < 4 {
		return nil, nil, fmt.Errorf("short null bitmap")
	}
	n := int(u32(b))
	if len(b) < 4+n {
		return nil, nil, fmt.Errorf("short null payload")
	}
	return storage.BitmapFromBytes(b[4:4+n], rows), b[4+n:], nil
}

func packI64(vals []int64, null *storage.Bitmap) []byte {
	buf := make([]byte, 4+len(vals)*8)
	putU32(buf, uint32(len(vals)))
	for i, v := range vals {
		binary.LittleEndian.PutUint64(buf[4+i*8:], uint64(v))
	}
	return append(buf, packNull(null)...)
}

func packF64(vals []float64, null *storage.Bitmap) []byte {
	tmp := make([]int64, len(vals))
	for i, v := range vals {
		tmp[i] = int64(binary.LittleEndian.Uint64(float64To(v)))
	}
	buf := make([]byte, 4+len(vals)*8)
	putU32(buf, uint32(len(vals)))
	for i, v := range vals {
		copy(buf[4+i*8:], float64To(v))
	}
	_ = tmp
	return append(buf, packNull(null)...)
}

func float64To(f float64) []byte {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], bitsFromFloat(f))
	return b[:]
}

func bitsFromFloat(f float64) uint64 {
	return mathFloat64bits(f)
}

func packU8(vals []uint8, null *storage.Bitmap) []byte {
	buf := make([]byte, 4+len(vals))
	putU32(buf, uint32(len(vals)))
	copy(buf[4:], vals)
	return append(buf, packNull(null)...)
}

func packStr(v *storage.StrVec) []byte {
	off := v.Offsets()
	data := v.Data()
	buf := make([]byte, 4+4+len(off)*4+len(data))
	putU32(buf[0:], uint32(len(off)))
	putU32(buf[4:], uint32(len(data)))
	for i, o := range off {
		binary.LittleEndian.PutUint32(buf[8+i*4:], uint32(o))
	}
	copy(buf[8+len(off)*4:], data)
	return append(buf, packNull(v.Nulls())...)
}

func packDict(v *encoding.DictVec) []byte {
	body := packStr(v.Dict())
	ids := v.IDs()
	buf := make([]byte, 4+len(ids)*4)
	putU32(buf, uint32(len(ids)))
	for i, id := range ids {
		binary.LittleEndian.PutUint32(buf[4+i*4:], id)
	}
	out := append(body, buf...)
	return append(out, packNull(v.Nulls())...)
}

func packRLE(v *encoding.RLEVec) []byte {
	starts := v.Starts()
	vals := v.Vals()
	var buf []byte
	tmp := make([]byte, 4)
	putU32(tmp, uint32(len(starts)))
	buf = append(buf, tmp...)
	for _, s := range starts {
		var b [4]byte
		putU32(b[:], uint32(s))
		buf = append(buf, b[:]...)
	}
	putU32(tmp, uint32(len(vals)))
	buf = append(buf, tmp...)
	for _, val := range vals {
		buf = append(buf, packValue(val)...)
	}
	var n [4]byte
	putU32(n[:], uint32(v.Len()))
	buf = append(buf, n[:]...)
	return buf
}
