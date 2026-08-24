package types

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type DataType uint8

const (
	Invalid DataType = iota
	Int64
	Float64
	Bool
	Timestamp
	String
)

func (t DataType) String() string {
	switch t {
	case Int64:
		return "INT64"
	case Float64:
		return "FLOAT64"
	case Bool:
		return "BOOL"
	case Timestamp:
		return "TIMESTAMP"
	case String:
		return "STRING"
	default:
		return "INVALID"
	}
}

func Parse(s string) (DataType, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "INT", "INT64", "INTEGER", "BIGINT":
		return Int64, nil
	case "FLOAT", "FLOAT64", "DOUBLE", "REAL":
		return Float64, nil
	case "BOOL", "BOOLEAN":
		return Bool, nil
	case "TIMESTAMP", "DATETIME", "DATE":
		return Timestamp, nil
	case "STRING", "TEXT", "VARCHAR", "CHAR":
		return String, nil
	default:
		return Invalid, fmt.Errorf("unknown type %q", s)
	}
}

type Encoding uint8

const (
	Plain Encoding = iota
	Dict
	RLE
)

func (e Encoding) String() string {
	switch e {
	case Dict:
		return "DICT"
	case RLE:
		return "RLE"
	default:
		return "PLAIN"
	}
}

type Value struct {
	Type DataType
	I    int64
	F    float64
	B    bool
	S    string
	Null bool
}

func Null(t DataType) Value { return Value{Type: t, Null: true} }
func VInt(v int64) Value    { return Value{Type: Int64, I: v} }
func VFloat(v float64) Value {
	return Value{Type: Float64, F: v}
}
func VBool(v bool) Value   { return Value{Type: Bool, B: v} }
func VStr(v string) Value  { return Value{Type: String, S: v} }
func VTime(v int64) Value  { return Value{Type: Timestamp, I: v} }

func (v Value) String() string {
	if v.Null {
		return "NULL"
	}
	switch v.Type {
	case Int64:
		return strconv.FormatInt(v.I, 10)
	case Float64:
		return strconv.FormatFloat(v.F, 'f', -1, 64)
	case Bool:
		if v.B {
			return "true"
		}
		return "false"
	case Timestamp:
		return time.UnixMilli(v.I).In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
	default:
		return v.S
	}
}

func (v Value) AsFloat() (float64, bool) {
	if v.Null {
		return 0, false
	}
	switch v.Type {
	case Int64, Timestamp:
		return float64(v.I), true
	case Float64:
		return v.F, true
	default:
		return 0, false
	}
}

func Compare(a, b Value) int {
	if a.Null && b.Null {
		return 0
	}
	if a.Null {
		return -1
	}
	if b.Null {
		return 1
	}
	if a.Type != b.Type {
		fa, oka := a.AsFloat()
		fb, okb := b.AsFloat()
		if oka && okb {
			switch {
			case fa < fb:
				return -1
			case fa > fb:
				return 1
			default:
				return 0
			}
		}
		return strings.Compare(a.String(), b.String())
	}
	switch a.Type {
	case Int64, Timestamp:
		switch {
		case a.I < b.I:
			return -1
		case a.I > b.I:
			return 1
		}
	case Float64:
		switch {
		case a.F < b.F:
			return -1
		case a.F > b.F:
			return 1
		}
	case Bool:
		switch {
		case !a.B && b.B:
			return -1
		case a.B && !b.B:
			return 1
		}
	default:
		return strings.Compare(a.S, b.S)
	}
	return 0
}

func InferSample(s string) DataType {
	s = strings.TrimSpace(s)
	if s == "" {
		return Invalid
	}
	low := strings.ToLower(s)
	if low == "true" || low == "false" {
		return Bool
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return Int64
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return Float64
	}
	if parseTime(s) != nil {
		return Timestamp
	}
	return String
}

func ParseCell(s string, t DataType) (Value, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "null") || s == "\\N" {
		return Null(t), true
	}
	switch t {
	case Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			f, e2 := strconv.ParseFloat(s, 64)
			if e2 != nil {
				return Null(t), false
			}
			return VInt(int64(f)), true
		}
		return VInt(n), true
	case Float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return Null(t), false
		}
		return VFloat(f), true
	case Bool:
		switch strings.ToLower(s) {
		case "true", "1", "yes", "y", "t":
			return VBool(true), true
		case "false", "0", "no", "n", "f":
			return VBool(false), true
		default:
			return Null(t), false
		}
	case Timestamp:
		if tm := parseTime(s); tm != nil {
			return VTime(tm.UnixMilli()), true
		}
		return Null(t), false
	default:
		return VStr(s), true
	}
}

func parseTime(s string) *time.Time {
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
		"2006/01/02 15:04:05",
		"2006/01/02",
	}
	loc := time.FixedZone("CST", 8*3600)
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			return &t
		}
	}
	return nil
}

func Ident(s string) string {
	var b strings.Builder
	for i, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || r == '_' || (unicode.IsDigit(r) && i > 0) {
			b.WriteRune(r)
		} else if unicode.IsDigit(r) && i == 0 {
			b.WriteByte('_')
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "col"
	}
	return out
}
