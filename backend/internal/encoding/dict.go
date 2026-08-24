package encoding

import (
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

// DictVec: dictionary (arena) + uint32 codes. O(1) random access.
type DictVec struct {
	dict    *storage.StrVec
	ids     []uint32
	null    *storage.Bitmap
	inverse map[string]uint32
}

func NewDict(dict *storage.StrVec, ids []uint32, null *storage.Bitmap) *DictVec {
	if null == nil {
		null = storage.NewBitmap(len(ids))
	}
	d := &DictVec{dict: dict, ids: ids, null: null, inverse: map[string]uint32{}}
	for i := 0; i < dict.Len(); i++ {
		d.inverse[dict.At(i)] = uint32(i)
	}
	return d
}

func EncodeDict(src *storage.StrVec) *DictVec {
	seen := map[string]uint32{}
	var keys []string
	ids := make([]uint32, src.Len())
	for i := 0; i < src.Len(); i++ {
		if src.IsNull(i) {
			continue
		}
		s := src.At(i)
		id, ok := seen[s]
		if !ok {
			id = uint32(len(keys))
			seen[s] = id
			keys = append(keys, s)
		}
		ids[i] = id
	}
	return NewDict(storage.BuildStr(keys, nil), ids, src.Nulls())
}

func (v *DictVec) Type() types.DataType     { return types.String }
func (v *DictVec) Encoding() types.Encoding { return types.Dict }
func (v *DictVec) Len() int                 { return len(v.ids) }
func (v *DictVec) IsNull(i int) bool        { return v.null.Get(i) }
func (v *DictVec) NullCount() int           { return v.null.Count() }
func (v *DictVec) IDs() []uint32            { return v.ids }
func (v *DictVec) Dict() *storage.StrVec    { return v.dict }
func (v *DictVec) Nulls() *storage.Bitmap   { return v.null }
func (v *DictVec) Cardinality() int         { return v.dict.Len() }
func (v *DictVec) LookupID(s string) (uint32, bool) {
	id, ok := v.inverse[s]
	return id, ok
}
func (v *DictVec) RawBytes() int64 {
	return int64(len(v.ids)*4) + int64(v.dict.Len())*8
}
func (v *DictVec) MemBytes() int64 {
	return int64(len(v.ids)*4) + v.dict.MemBytes() + int64(len(v.null.Bytes()))
}
func (v *DictVec) Get(i int) types.Value {
	if v.null.Get(i) {
		return types.Null(types.String)
	}
	return types.VStr(v.dict.At(int(v.ids[i])))
}
func (v *DictVec) Take(sel []int) storage.Vector {
	ids := make([]uint32, len(sel))
	nb := storage.NewBitmap(len(sel))
	for i, s := range sel {
		ids[i] = v.ids[s]
		if v.null.Get(s) {
			nb.Set(i)
		}
	}
	return NewDict(v.dict, ids, nb)
}
