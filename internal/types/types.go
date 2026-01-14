package types

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// DataType represents the type of a column
type DataType int

const (
	TypeInteger DataType = iota
	TypeVarchar
	TypeText
	TypeBoolean
	TypeFloat
	TypeDate
	TypeTimestamp
)

// String returns the string representation of a DataType
func (dt DataType) String() string {
	switch dt {
	case TypeInteger:
		return "INTEGER"
	case TypeVarchar:
		return "VARCHAR"
	case TypeText:
		return "TEXT"
	case TypeBoolean:
		return "BOOLEAN"
	case TypeFloat:
		return "FLOAT"
	case TypeDate:
		return "DATE"
	case TypeTimestamp:
		return "TIMESTAMP"
	default:
		return "UNKNOWN"
	}
}

// ColumnDef represents a column definition
type ColumnDef struct {
	Name     string
	Type     DataType
	Size     int // For VARCHAR
	Nullable bool
}

// Value represents a typed value in the database
type Value struct {
	Type  DataType
	Data  interface{}
	IsNull bool
}

// NewValue creates a new Value from an interface{}
func NewValue(dataType DataType, data interface{}) (*Value, error) {
	if data == nil {
		return &Value{Type: dataType, Data: nil, IsNull: true}, nil
	}

	val := &Value{Type: dataType, IsNull: false}
	
	switch dataType {
	case TypeInteger:
		switch v := data.(type) {
		case int:
			val.Data = int64(v)
		case int64:
			val.Data = v
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to INTEGER: %w", v, err)
			}
			val.Data = i
		default:
			return nil, fmt.Errorf("cannot convert %T to INTEGER", data)
		}
	case TypeFloat:
		switch v := data.(type) {
		case float64:
			val.Data = v
		case float32:
			val.Data = float64(v)
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to FLOAT: %w", v, err)
			}
			val.Data = f
		default:
			return nil, fmt.Errorf("cannot convert %T to FLOAT", data)
		}
	case TypeVarchar, TypeText:
		switch v := data.(type) {
		case string:
			val.Data = v
		default:
			val.Data = fmt.Sprintf("%v", data)
		}
	case TypeBoolean:
		switch v := data.(type) {
		case bool:
			val.Data = v
		case string:
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to BOOLEAN: %w", v, err)
			}
			val.Data = b
		default:
			return nil, fmt.Errorf("cannot convert %T to BOOLEAN", data)
		}
	case TypeDate:
		switch v := data.(type) {
		case string:
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to DATE: %w", v, err)
			}
			val.Data = t
		case time.Time:
			val.Data = v
		default:
			return nil, fmt.Errorf("cannot convert %T to DATE", data)
		}
	case TypeTimestamp:
		switch v := data.(type) {
		case string:
			// Try multiple formats
			formats := []string{
				"2006-01-02 15:04:05",
				time.RFC3339,
				"2006-01-02T15:04:05Z07:00",
			}
			var t time.Time
			var err error
			for _, format := range formats {
				t, err = time.Parse(format, v)
				if err == nil {
					break
				}
			}
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to TIMESTAMP: %w", v, err)
			}
			val.Data = t
		case time.Time:
			val.Data = v
		default:
			return nil, fmt.Errorf("cannot convert %T to TIMESTAMP", data)
		}
	default:
		return nil, fmt.Errorf("unknown data type: %d", dataType)
	}

	return val, nil
}

// Compare compares two values. Returns -1 if v < other, 0 if equal, 1 if v > other
func (v *Value) Compare(other *Value) (int, error) {
	if v.IsNull || other.IsNull {
		if v.IsNull && other.IsNull {
			return 0, nil
		}
		if v.IsNull {
			return -1, nil
		}
		return 1, nil
	}

	if v.Type != other.Type {
		return 0, errors.New("cannot compare values of different types")
	}

	switch v.Type {
	case TypeInteger:
		vi := v.Data.(int64)
		oi := other.Data.(int64)
		if vi < oi {
			return -1, nil
		} else if vi > oi {
			return 1, nil
		}
		return 0, nil
	case TypeFloat:
		vf := v.Data.(float64)
		of := other.Data.(float64)
		if vf < of {
			return -1, nil
		} else if vf > of {
			return 1, nil
		}
		return 0, nil
	case TypeVarchar, TypeText:
		vs := v.Data.(string)
		os := other.Data.(string)
		if vs < os {
			return -1, nil
		} else if vs > os {
			return 1, nil
		}
		return 0, nil
	case TypeBoolean:
		vb := v.Data.(bool)
		ob := other.Data.(bool)
		if !vb && ob {
			return -1, nil
		} else if vb && !ob {
			return 1, nil
		}
		return 0, nil
	case TypeDate, TypeTimestamp:
		vt := v.Data.(time.Time)
		ot := other.Data.(time.Time)
		if vt.Before(ot) {
			return -1, nil
		} else if vt.After(ot) {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("comparison not supported for type %s", v.Type)
	}
}

// String returns the string representation of the value
func (v *Value) String() string {
	if v.IsNull {
		return "NULL"
	}

	switch v.Type {
	case TypeInteger:
		return fmt.Sprintf("%d", v.Data.(int64))
	case TypeFloat:
		return fmt.Sprintf("%g", v.Data.(float64))
	case TypeVarchar, TypeText:
		return v.Data.(string)
	case TypeBoolean:
		return fmt.Sprintf("%t", v.Data.(bool))
	case TypeDate:
		return v.Data.(time.Time).Format("2006-01-02")
	case TypeTimestamp:
		return v.Data.(time.Time).Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprintf("%v", v.Data)
	}
}

// Serialize converts a value to bytes for storage
func (v *Value) Serialize() ([]byte, error) {
	if v.IsNull {
		return []byte{0}, nil // 0 byte indicates NULL
	}

	buf := make([]byte, 0, 64)
	buf = append(buf, 1) // 1 byte indicates NOT NULL

	switch v.Type {
	case TypeInteger:
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, uint64(v.Data.(int64)))
		buf = append(buf, data...)
	case TypeFloat:
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, math.Float64bits(v.Data.(float64)))
		buf = append(buf, data...)
	case TypeVarchar, TypeText:
		str, ok := v.Data.(string)
		if !ok {
			return nil, fmt.Errorf("VARCHAR/TEXT value has invalid data type: %T", v.Data)
		}
		length := make([]byte, 4)
		binary.BigEndian.PutUint32(length, uint32(len(str)))
		buf = append(buf, length...)
		buf = append(buf, []byte(str)...)
	case TypeBoolean:
		if v.Data.(bool) {
			buf = append(buf, 1)
		} else {
			buf = append(buf, 0)
		}
	case TypeDate, TypeTimestamp:
		t := v.Data.(time.Time)
		unix := t.Unix()
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, uint64(unix))
		buf = append(buf, data...)
	default:
		return nil, fmt.Errorf("cannot serialize type %s", v.Type)
	}

	return buf, nil
}

// DeserializeValue creates a Value from bytes
func DeserializeValue(dataType DataType, data []byte) (*Value, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}

	isNull := data[0] == 0
	if isNull {
		return &Value{Type: dataType, Data: nil, IsNull: true}, nil
	}

	data = data[1:] // Skip null flag

	switch dataType {
	case TypeInteger:
		if len(data) < 8 {
			return nil, errors.New("insufficient data for INTEGER")
		}
		val := int64(binary.BigEndian.Uint64(data[:8]))
		return &Value{Type: dataType, Data: val, IsNull: false}, nil
	case TypeFloat:
		if len(data) < 8 {
			return nil, errors.New("insufficient data for FLOAT")
		}
		val := math.Float64frombits(binary.BigEndian.Uint64(data[:8]))
		return &Value{Type: dataType, Data: val, IsNull: false}, nil
	case TypeVarchar, TypeText:
		if len(data) < 4 {
			return nil, errors.New("insufficient data for string length")
		}
		length := binary.BigEndian.Uint32(data[:4])
		if len(data) < int(4+length) {
			return nil, errors.New("insufficient data for string")
		}
		val := string(data[4 : 4+length])
		return &Value{Type: dataType, Data: val, IsNull: false}, nil
	case TypeBoolean:
		if len(data) < 1 {
			return nil, errors.New("insufficient data for BOOLEAN")
		}
		val := data[0] != 0
		return &Value{Type: dataType, Data: val, IsNull: false}, nil
	case TypeDate, TypeTimestamp:
		if len(data) < 8 {
			return nil, errors.New("insufficient data for timestamp")
		}
		unix := int64(binary.BigEndian.Uint64(data[:8]))
		val := time.Unix(unix, 0)
		return &Value{Type: dataType, Data: val, IsNull: false}, nil
	default:
		return nil, fmt.Errorf("cannot deserialize type %s", dataType)
	}
}

// ParseDataType parses a data type string (e.g., "VARCHAR(50)", "INTEGER")
func ParseDataType(s string) (DataType, int, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	
	if strings.HasPrefix(s, "VARCHAR") {
		// Extract size from VARCHAR(n)
		if strings.Contains(s, "(") {
			start := strings.Index(s, "(")
			end := strings.Index(s, ")")
			if end == -1 {
				return 0, 0, fmt.Errorf("invalid VARCHAR syntax: missing closing parenthesis")
			}
			sizeStr := s[start+1 : end]
			size, err := strconv.Atoi(sizeStr)
			if err != nil {
				return 0, 0, fmt.Errorf("invalid VARCHAR size: %w", err)
			}
			return TypeVarchar, size, nil
		}
		return TypeVarchar, 0, nil
	}

	switch s {
	case "INTEGER", "INT":
		return TypeInteger, 0, nil
	case "TEXT":
		return TypeText, 0, nil
	case "BOOLEAN", "BOOL":
		return TypeBoolean, 0, nil
	case "FLOAT", "DOUBLE", "REAL":
		return TypeFloat, 0, nil
	case "DATE":
		return TypeDate, 0, nil
	case "TIMESTAMP":
		return TypeTimestamp, 0, nil
	default:
		return 0, 0, fmt.Errorf("unknown data type: %s", s)
	}
}
