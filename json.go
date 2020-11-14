package ent

import "github.com/rsms/go-json"

// JsonEncoder is an implementation of the Encoder interface
type JsonEncoder struct {
	json.Builder // Note: set Builder.Indent to enable pretty-printing
}

func (c *JsonEncoder) Err() error { return c.Builder.Err }

func (c *JsonEncoder) BeginEnt(version uint64) {
	c.StartObject()
	c.Key(FieldNameVersion)
	c.Uint(version, 64)
}

func (c *JsonEncoder) EndEnt() {
	c.EndObject()
}

func (c *JsonEncoder) BeginList(length int) { c.StartArray() }
func (c *JsonEncoder) EndList()             { c.EndArray() }
func (c *JsonEncoder) BeginDict(length int) { c.StartObject() }
func (c *JsonEncoder) EndDict()             { c.EndObject() }

// JsonDecoder is an implementation of the Decoder interface
type JsonDecoder struct {
	json.Reader
}

func NewJsonDecoder(data []byte) *JsonDecoder {
	c := &JsonDecoder{}
	c.ResetBytes(data)
	return c
}

func (c *JsonDecoder) DictHeader() int {
	if c.Reader.ObjectStart() {
		return -1
	}
	return 0
}

func (c *JsonDecoder) ListHeader() int {
	if c.Reader.ArrayStart() {
		return -1
	}
	return 0
}

// -------------

// JsonDecodeEntPartial is a utility function for decoding a partial ent.
// It calls e.EntDecodePartial and thus is limited to fields that participate in indexes.
func JsonDecodeEntPartial(e Ent, data []byte, fields uint64) (version uint64, err error) {
	c := NewJsonDecoder(data)
	if c.DictHeader() != 0 {
		version = e.EntDecodePartial(c, fields)
	}
	if err = c.Err(); err != nil {
		err = &JsonError{err}
	}
	return
}

// The two following functions are used by ent.JsonEncode and ent.JsonDecode to expose a general
// JSON codec as well as to implement MarshalJSON and UnmarshalJSON for Ent types.

func JsonEncodeEnt(e Ent, id, version, fieldmap uint64) ([]byte, error) {
	c := JsonEncoder{}
	// c.Builder.Indent = "  " // pretty print
	c.BeginEnt(version)

	// include the id so that jsonDecodeEnt works as expected
	c.Key(FieldNameId)
	c.Uint(id, 64)

	e.EntEncode(&c, fieldmap)
	c.EndEnt()
	return c.Bytes(), c.Err()
}

func JsonDecodeEnt(e Ent, data []byte) (id, version uint64, err error) {
	c := NewJsonDecoder(data)
	if c.DictHeader() != 0 {
		id, version = e.EntDecode(c)
	}
	if err = c.Err(); err != nil {
		err = &JsonError{err}
	}
	return
}

type JsonError struct {
	Underlying error
}

func (e *JsonError) Unwrap() error { return e.Underlying }
func (e *JsonError) Error() string { return "json error: " + e.Underlying.Error() }
