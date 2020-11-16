package ent

import (
	"bytes"
	"reflect"
	"text/tabwriter"
)

// ReprFlags are changes behavior of Repr
type ReprFlags int

const (
	// ReprOmitEmpty causes empty, non-numeric fields to be excluded
	ReprOmitEmpty = ReprFlags(1 << iota)
)

// Repr formats a human-readable representation of an ent. It only includes fields in fieldmap.
func Repr(e Ent, fieldmap uint64, flags ReprFlags) ([]byte, error) {
	c := JsonEncoder{BareKeys: true}
	c.Builder.Indent = "\t"
	c.Builder.KeyTerm = []byte(":\t")

	if (flags & ReprOmitEmpty) != 0 {
		fieldmap &= ^FieldsWithEmptyValue(e)
	}

	// encode ent
	c.BeginEnt(e.Version())
	c.Key(FieldNameId)
	c.Uint(e.Id(), 64)
	e.EntEncode(&c, fieldmap)
	c.EndEnt()
	if err := c.Err(); err != nil {
		return nil, err
	}

	// thread the results through tabwriter for pretty columnar output
	var wbuf bytes.Buffer
	minwidth, tabwidth, padding := 2, 2, 1
	w := tabwriter.NewWriter(&wbuf, minwidth, tabwidth, padding, ' ', 0)
	w.Write(c.Bytes())
	err := w.Flush()
	return wbuf.Bytes(), err
}

// FieldsWithEmptyValue returns a fieldmap of all non-numeric non-bool fields which has a
// zero value for its type.
func FieldsWithEmptyValue(e Ent) uint64 {
	v := reflect.ValueOf(e).Elem()
	n := v.NumField()
	var fieldmap uint64
	for i := 1; i < n; i++ {
		fv := v.Field(i)
		if fv.IsZero() {
			switch fv.Kind() {
			case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
				// don't consider these types of fields as zero, since we can't tell if the actual
				// legitimate value is "0" or unset.
			default:
				fieldmap |= (1 << (i - 1))
			}
		}
	}
	return fieldmap
}

// // FieldsWithZeroValue returns a fieldmap of all fields which has a zero value for its type.
// func FieldsWithZeroValue(e Ent) uint64 {
// 	v := reflect.ValueOf(e).Elem()
// 	n := v.NumField()
// 	var fieldmap uint64
// 	for i := 1; i < n; i++ {
// 		if v.Field(i).IsZero() {
// 			fieldmap |= (1 << (i - 1))
// 		}
// 	}
// 	return fieldmap
// }
