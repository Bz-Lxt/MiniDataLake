package persist

import (
	"encoding/binary"
	"hash/crc32"
)

const (
	Magic    = "MDL1"
	MagicEnd = "MDL$"
	Version  = uint16(1)
	HeaderN  = 32
)

func crc(b []byte) uint32 { return crc32.ChecksumIEEE(b) }

func putU16(b []byte, v uint16) { binary.LittleEndian.PutUint16(b, v) }
func putU32(b []byte, v uint32) { binary.LittleEndian.PutUint32(b, v) }
func putU64(b []byte, v uint64) { binary.LittleEndian.PutUint64(b, v) }
func u16(b []byte) uint16       { return binary.LittleEndian.Uint16(b) }
func u32(b []byte) uint32       { return binary.LittleEndian.Uint32(b) }
func u64(b []byte) uint64       { return binary.LittleEndian.Uint64(b) }
