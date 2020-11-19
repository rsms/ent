package ent

type FieldSet uint64

func (f FieldSet) With(fieldIndex int) FieldSet {
	return f | (1 << fieldIndex)
}

func (f FieldSet) Without(fieldIndex int) FieldSet {
	return f & ^(1 << fieldIndex)
}

func (f FieldSet) Has(fieldIndex int) bool {
	return (f & (1 << fieldIndex)) != 0
}

func (f FieldSet) Union(other FieldSet) FieldSet {
	return f | other
}
