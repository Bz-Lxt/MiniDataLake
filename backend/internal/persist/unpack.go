package persist

import (
	"encoding/binary"
	"fmt"
	"math"

	"minidatalake/internal/encoding"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func mathFloat64bits(f float64) uint64 { return math.Float64bits(f) }

func unpackPlain(t types.DataType, b []byte, rows int) (storage.Vector, error) {
	switch t {
	case types.Int64:
		vals, rest, err := unpackI64(b)
		if err != nil {
			return nil, err
		}
		nb, _, err := unpackNull(rest, len(vals))
		if err != nil {
			return nil, err
		}
		return storage.NewInt64(vals, nb), nil
	case types.Timestamp:
		vals, rest, err := unpackI64(b)
		if err != nil {
			return nil, err
		}
		nb, _, err := unpackNull(rest, len(vals))
		if err != nil {
			return nil, err
		}
		return storage.NewTime(vals, nb), nil
	case types.Float64:
		vals, rest, err := unpackF64(b)
		if err != nil {
			return nil, err
		}
		nb, _, err := unpackNull(rest, len(vals))
		if err != nil {
			return nil, err
		}
		return storage.NewFloat64(vals, nb), nil
	case types.Bool:
		vals, rest, err := unpackU8(b)
		if err != nil {
			return nil, err
		}
		nb, _, err := unpackNull(rest, len(vals))
		if err != nil {
			return nil, err
		}
		return storage.NewBool(vals, nb), nil
	default:
		sv, rest, err := unpackStr(b)
		if err != nil {
			return nil, err
		}
		nb, _, err := unpackNull(rest, sv.Len())
		if err != nil {
			return nil, err
		}
		return storage.NewStr(sv.Data(), sv.Offsets(), nb), nil
	}
}

func unpackI64(b []byte) ([]int64, []byte, error) {
	if len(b) < 4 {
		return nil, nil, fmt.Errorf("short i64")
	}
	n := int(u32(b))
	need := 4 + n*8
	if len(b) < need {
		return nil, nil, fmt.Errorf("short i64 payload")
	}
	vals := make([]int64, n)
	for i := 0; i < n; i++ {
		vals[i] = int64(binary.LittleEndian.Uint64(b[4+i*8:]))
	}
	return vals, b[need:], nil
}

func unpackF64(b []byte) ([]float64, []byte, error) {
	if len(b) < 4 {
		return nil, nil, fmt.Errorf("short f64")
	}
	n := int(u32(b))
	need := 4 + n*8
	if len(b) < need {
		return nil, nil, fmt.Errorf("short f64 payload")
	}
	vals := make([]float64, n)
	for i := 0; i < n; i++ {
		vals[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[4+i*8:]))
	}
	return vals, b[need:], nil
}

func unpackU8(b []byte) ([]uint8, []byte, error) {
	if len(b) < 4 {
		return nil, nil, fmt.Errorf("short u8")
	}
	n := int(u32(b))
	if len(b) < 4+n {
		return nil, nil, fmt.Errorf("short u8 payload")
	}
	vals := append([]uint8{}, b[4:4+n]...)
	return vals, b[4+n:], nil
}

func unpackStr(b []byte) (*storage.StrVec, []byte, error) {
	if len(b) < 8 {
		return nil, nil, fmt.Errorf("short str")
	}
	noff := int(u32(b[0:]))
	ndat := int(u32(b[4:]))
	need := 8 + noff*4 + ndat
	if len(b) < need {
		return nil, nil, fmt.Errorf("short str payload")
	}
	off := make([]int32, noff)
	for i := 0; i < noff; i++ {
		off[i] = int32(binary.LittleEndian.Uint32(b[8+i*4:]))
	}
	data := append([]byte{}, b[8+noff*4:need]...)
	return storage.NewStr(data, off, storage.NewBitmap(noff-1)), b[need:], nil
}

func unpackDict(b []byte, rows int) (storage.Vector, error) {
	sv, rest, err := unpackStr(b)
	if err != nil {
		return nil, err
	}
	dnull, rest, err := unpackNull(rest, sv.Len())
	if err != nil {
		return nil, err
	}
	sv = storage.NewStr(sv.Data(), sv.Offsets(), dnull)
	if len(rest) < 4 {
		return nil, fmt.Errorf("short dict ids")
	}
	n := int(u32(rest))
	if len(rest) < 4+n*4 {
		return nil, fmt.Errorf("short dict id payload")
	}
	ids := make([]uint32, n)
	for i := 0; i < n; i++ {
		ids[i] = binary.LittleEndian.Uint32(rest[4+i*4:])
	}
	rest = rest[4+n*4:]
	nb, _, err := unpackNull(rest, rows)
	if err != nil {
		return nil, err
	}
	return encoding.NewDict(sv, ids, nb), nil
}

func unpackRLE(b []byte, t types.DataType, rows int) (storage.Vector, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("short rle")
	}
	ns := int(u32(b))
	off := 4
	starts := make([]int32, ns)
	for i := 0; i < ns; i++ {
		if off+4 > len(b) {
			return nil, fmt.Errorf("short rle starts")
		}
		starts[i] = int32(u32(b[off:]))
		off += 4
	}
	if off+4 > len(b) {
		return nil, fmt.Errorf("short rle vals")
	}
	nv := int(u32(b[off:]))
	off += 4
	vals := make([]types.Value, nv)
	for i := 0; i < nv; i++ {
		v, n, err := unpackValue(b[off:], t)
		if err != nil {
			return nil, err
		}
		vals[i] = v
		off += n
	}
	if off+4 <= len(b) {
		rows = int(u32(b[off:]))
	}
	return encoding.NewRLE(t, starts, vals, rows), nil
}

func packValue(v types.Value) []byte {
	out := []byte{uint8(v.Type), 0}
	if v.Null {
		out[1] = 1
		return out
	}
	switch v.Type {
	case types.Int64, types.Timestamp:
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], uint64(v.I))
		out = append(out, b[:]...)
	case types.Float64:
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(v.F))
		out = append(out, b[:]...)
	case types.Bool:
		if v.B {
			out = append(out, 1)
		} else {
			out = append(out, 0)
		}
	default:
		var n [4]byte
		putU32(n[:], uint32(len(v.S)))
		out = append(out, n[:]...)
		out = append(out, v.S...)
	}
	return out
}

func unpackValue(b []byte, hint types.DataType) (types.Value, int, error) {
	if len(b) < 2 {
		return types.Value{}, 0, fmt.Errorf("short value")
	}
	t := types.DataType(b[0])
	if t == types.Invalid {
		t = hint
	}
	if b[1] == 1 {
		return types.Null(t), 2, nil
	}
	off := 2
	switch t {
	case types.Int64:
		if len(b) < off+8 {
			return types.Value{}, 0, fmt.Errorf("short i")
		}
		return types.VInt(int64(binary.LittleEndian.Uint64(b[off:]))), off + 8, nil
	case types.Timestamp:
		if len(b) < off+8 {
			return types.Value{}, 0, fmt.Errorf("short ts")
		}
		return types.VTime(int64(binary.LittleEndian.Uint64(b[off:]))), off + 8, nil
	case types.Float64:
		if len(b) < off+8 {
			return types.Value{}, 0, fmt.Errorf("short f")
		}
		return types.VFloat(math.Float64frombits(binary.LittleEndian.Uint64(b[off:]))), off + 8, nil
	case types.Bool:
		if len(b) < off+1 {
			return types.Value{}, 0, fmt.Errorf("short b")
		}
		return types.VBool(b[off] != 0), off + 1, nil
	default:
		if len(b) < off+4 {
			return types.Value{}, 0, fmt.Errorf("short s")
		}
		n := int(u32(b[off:]))
		off += 4
		if len(b) < off+n {
			return types.Value{}, 0, fmt.Errorf("short s body")
		}
		return types.VStr(string(b[off : off+n])), off + n, nil
	}
}
