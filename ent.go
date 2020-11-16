package ent

import (
	"errors"
	"unsafe"
)

type Ent interface {
	Id() uint64
	Version() uint64
	HasUnsavedChanges() bool

	EntTypeName() string
	EntNew() Ent
	EntEncode(c Encoder, fieldmap uint64)
	EntDecode(c Decoder) (id, version uint64)
	EntDecodePartial(c Decoder, fields uint64) (version uint64)
	EntIndexes() []EntIndex
	EntFields() Fields
}

// EntBase is the foundation for all ent types.
// Use it as the first embedded field in a struct to make the struct an ent.
type EntBase struct {
	id      uint64
	version uint64
	storage Storage

	// fieldmap is a bitmap where each bit represents a struct field index.
	// A set bit indicates that the field's value has changed since the last call to Load()
	fieldmap uint64
}

// Fields describes fields of an ent. Available via TYPE.EntFields()
type Fields struct {
	Names    []string // names of fields, ordered by field index
	Fieldmap uint64   // a bitmap with all fields set
}

var (
	ErrNoStorage       = errors.New("no ent storage")
	ErrNotFound        = errors.New("ent not found")
	ErrNotChanged      = errors.New("ent not changed")
	ErrVersionConflict = errors.New("version conflict")
	ErrUniqueConflict  = errors.New("unique index conflict")
	ErrDuplicateEnt    = errors.New("duplicate ent")
)

var (
	FieldNameVersion = "_ver"
	FieldNameId      = "_id"
)

type Id uint64

// emptyInterface is the header for an interface{} value. From go src/reflect/value.go
type emptyInterface struct {
	typ  *int
	word unsafe.Pointer
}

func entBase(e Ent) (eb *EntBase) {
	ef := (*emptyInterface)(unsafe.Pointer(&e))
	if ef.typ != nil {
		eb = (*EntBase)(ef.word)
	}
	return
}

func (e *EntBase) Id() uint64              { return e.id }
func (e *EntBase) Version() uint64         { return e.version }
func (e *EntBase) HasUnsavedChanges() bool { return e.fieldmap != 0 }

// these are just stubs; actual implementations generated by entgen
func (e *EntBase) EntTypeName() string       { return "_" }
func (e *EntBase) EntEncode(Encoder, uint64) {}
func (e *EntBase) EntDecode(c Decoder) (id, version uint64) {
	// Default implementation which discards any fields apart from "special" fields
	for {
		key := c.Key()
		if key == "" {
			break
		}
		switch string(key) {
		case FieldNameId:
			id = c.Uint(64)
		case FieldNameVersion:
			version = c.Uint(64)
		default:
			c.Discard()
		}
	}
	return
}

func (e *EntBase) EntIsFieldChanged(fieldIndex int) bool {
	return IsFieldChanged(e, fieldIndex)
}

func (e *EntBase) EntIndexes() []EntIndex { return nil }
func (e *EntBase) EntFields() Fields      { return Fields{} }

// SetFieldChanged marks the field fieldIndex as "having unsaved changes"
func SetFieldChanged(e *EntBase, fieldIndex int) { e.fieldmap |= (1 << fieldIndex) }

// ClearFieldChanged marks the field fieldIndex as not having and unsaved changes
func ClearFieldChanged(e *EntBase, fieldIndex int) { e.fieldmap &= ^(1 << fieldIndex) }

// IsFieldChanged returns true if the field fieldIndex is marked as "having unsaved changes"
func IsFieldChanged(e *EntBase, fieldIndex int) bool {
	return (e.fieldmap & (1 << fieldIndex)) != 0
}

// JsonEncode encodes the ent as JSON
func JsonEncode(e Ent, indent string) ([]byte, error) {
	// Note: Used by generated code to implement MarshalJSON
	return JsonEncodeEnt(e, e.Id(), e.Version(), e.EntFields().Fieldmap, indent)
}

// JsonEncodeUnsaved encodes the ent as JSON, only including fields with unsaved changes
func JsonEncodeUnsaved(e Ent, indent string) ([]byte, error) {
	eb := entBase(e)
	return JsonEncodeEnt(e, e.Id(), e.Version(), eb.fieldmap, indent)
}

// JsonEncode encodes the ent as JSON
func JsonDecode(e Ent, data []byte) error {
	// Note: Used by generated code to implement UnmarshalJSON
	id, version, err := JsonDecodeEnt(e, data)
	if err == nil {
		eb := entBase(e)
		eb.id = id
		eb.version = version
	}
	return err
}

func EntString(e Ent) string {
	b, _ := Repr(e, e.EntFields().Fieldmap, ReprOmitEmpty)
	return string(b)
}

// SetEntBaseFieldsAfterLoad sets values of EntBase fields.
// This function is meant to be used by Storage implementation, called after a new ent has been
// loaded or created.
func SetEntBaseFieldsAfterLoad(e Ent, s Storage, id, version uint64) {
	eb := entBase(e)
	eb.id = id
	eb.version = version
	eb.storage = s
	eb.fieldmap = 0
}

// —————————————————————————————————————————————————————————
// CRUD
// C = CreateEnt(Ent,Storage)
// R = LoadEntById(Ent,Storage,id), ReloadEnt(Ent)
// U = SaveEnt(Ent)
// D = DeleteEnt(Ent)

func CreateEnt(e Ent, storage Storage) error {
	if storage == nil {
		return ErrNoStorage
	}
	eb := entBase(e)
	id, err := storage.CreateEnt(e, e.EntFields().Fieldmap)
	if err == nil {
		eb.id = id
		eb.version = 1
		eb.storage = storage
		eb.fieldmap = 0
	}
	return err
}

func LoadEntById(e Ent, storage Storage, id uint64) error {
	if storage == nil {
		return ErrNoStorage
	}
	if id == 0 {
		return ErrNotFound
	}
	version, err := storage.LoadEntById(e, id)
	if err == nil {
		SetEntBaseFieldsAfterLoad(e, storage, id, version)
	}
	return err
}

func ReloadEnt(e Ent) error {
	eb := entBase(e)
	return LoadEntById(e, eb.storage, eb.id)
}

func SaveEnt(e Ent) error {
	eb := entBase(e)
	if eb.storage == nil {
		return ErrNoStorage
	}
	if eb.fieldmap == 0 {
		return ErrNotChanged
	}
	version, err := eb.storage.SaveEnt(e, eb.fieldmap)
	if err == nil {
		eb.version = version
		eb.fieldmap = 0
	}
	return err
}

func DeleteEnt(e Ent) error {
	eb := entBase(e)
	if eb.storage == nil {
		return ErrNoStorage
	}
	err := eb.storage.DeleteEnt(e)
	if err == nil {
		eb.id = 0
		eb.version = 0
		eb.storage = nil
		eb.fieldmap = 0
	}
	return err
}
