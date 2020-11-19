package ent

import (
	"reflect"

	"github.com/rsms/go-bits"
)

type FieldSet uint64

func (f FieldSet) Len() int { return bits.PopcountUint64(uint64(f)) }

func (f FieldSet) With(fieldIndex int) FieldSet {
	return f | (1 << fieldIndex)
}

func (f FieldSet) Without(fieldIndex int) FieldSet {
	return f & ^(1 << fieldIndex)
}

func (f FieldSet) Has(fieldIndex int) bool {
	return (f & (1 << fieldIndex)) != 0
}

func (f FieldSet) Contains(other FieldSet) bool {
	return (f & other) != 0
}

func (f FieldSet) Union(other FieldSet) FieldSet {
	return f | other
}

// FieldsWithEmptyValue returns a FieldSet of all non-numeric non-bool fields
// which has a zero value for its type.
func FieldsWithEmptyValue(e Ent) FieldSet {
	v := reflect.ValueOf(e).Elem()
	n := v.NumField()
	var fields FieldSet
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
				fields |= (1 << (i - 1))
			}
		}
	}
	return fields
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
