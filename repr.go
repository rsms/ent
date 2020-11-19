package ent

// ReprFlags are changes behavior of Repr
type ReprFlags int

const (
	// ReprOmitEmpty causes empty, non-numeric fields to be excluded
	ReprOmitEmpty = ReprFlags(1 << iota)
)

// Repr formats a human-readable representation of an ent. It only includes fields.
func Repr(e Ent, fields FieldSet, flags ReprFlags) ([]byte, error) {
	if (flags & ReprOmitEmpty) != 0 {
		fields &= ^FieldsWithEmptyValue(e)
	}
	c := JsonEncoder{BareKeys: true}
	c.Builder.Indent = "  "
	c.BeginEnt(e.Version())
	c.Key(FieldNameId)
	c.Uint(e.Id(), 64)
	e.EntEncode(&c, fields)
	c.EndEnt()
	return c.Bytes(), c.Err()
}
