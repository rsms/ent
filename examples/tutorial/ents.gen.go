// +build !entgen

// Code generated by entgen. DO NOT EDIT.
package main

import "github.com/rsms/ent"

// ----------------------------------------------------------------------------
// Account

// LoadAccountById loads Account with id from storage
func LoadAccountById(storage ent.Storage, id uint64) (*Account, error) {
	e := &Account{}
	return e, ent.LoadEntById(e, storage, id)
}

// LoadAccountByEmail loads Account with email
func LoadAccountByEmail(s ent.Storage, email string, fl ...ent.LookupFlags) (*Account, error) {
	e := &Account{}
	err := ent.LoadEntByIndexKey(s, e, &ent_Account_idx[0], []byte(email), fl)
	return e, err
}

// FindAccountByEmail looks up Account id with email
func FindAccountByEmail(s ent.Storage, email string, fl ...ent.LookupFlags) (uint64, error) {
	return ent.FindIdByIndexKey(s, "account", &ent_Account_idx[0], []byte(email), fl)
}

// LoadAccountByKind loads all Account ents with kind
func LoadAccountByKind(s ent.Storage, kind AccountKind, limit int, fl ...ent.LookupFlags) ([]*Account, error) {
	e := &Account{}
	r, err := ent.LoadEntsByIndex(s, e, &ent_Account_idx[1], limit, fl, 1, func(c ent.Encoder) {
		c.Int(int64(kind), 32)
	})
	return ent_Account_slice_cast(r), err
}

// FindAccountByKind looks up Account ids with kind
func FindAccountByKind(s ent.Storage, kind AccountKind, limit int, fl ...ent.LookupFlags) ([]uint64, error) {
	return ent.FindIdsByIndex(s, "account", &ent_Account_idx[1], limit, fl, 1, func(c ent.Encoder) {
		c.Int(int64(kind), 32)
	})
}

// EntTypeName returns the ent's storage name ("account")
func (e Account) EntTypeName() string { return "account" }

// EntStorage returns the storage this ent belongs to or nil if it doesn't belong anywhere.
func (e *Account) EntStorage() ent.Storage { return ent.GetStorage(e) }

// EntNew returns a new empty Account. Used by the ent package for loading ents.
func (e Account) EntNew() ent.Ent { return &Account{} }

// MarshalJSON returns a JSON representation of e. Conforms to json.Marshaler.
func (e *Account) MarshalJSON() ([]byte, error) { return ent.JsonEncode(e, "") }

// UnmarshalJSON populates the ent from JSON data. Conforms to json.Unmarshaler.
func (e *Account) UnmarshalJSON(b []byte) error { return ent.JsonDecode(e, b) }

// String returns a JSON representation of e.
func (e Account) String() string { return ent.EntString(&e) }

// Create a new account ent in storage
func (e *Account) Create(storage ent.Storage) error { return ent.CreateEnt(e, storage) }

// Save pending changes to whatever storage this ent was created or loaded from
func (e *Account) Save() error { return ent.SaveEnt(e) }

// Reload fields to latest values from storage, discarding any unsaved changes
func (e *Account) Reload() error { return ent.ReloadEnt(e) }

// PermanentlyDelete deletes this ent from storage. This can usually not be undone.
func (e *Account) PermanentlyDelete() error { return ent.DeleteEnt(e) }

// Iterator returns an iterator over all Account ents. Order is undefined.
func (e Account) Iterator(s ent.Storage) ent.EntIterator { return s.IterateEnts(&e) }

// ---- field accessor methods ----

func (e *Account) Name() string        { return e.name }
func (e *Account) DisplayName() string { return e.displayName }
func (e *Account) Email() string       { return e.email }
func (e *Account) Kind() AccountKind   { return e.kind }

func (e *Account) SetName(v string)        { e.name = v; e.EntBase.SetEntFieldChanged(0) }
func (e *Account) SetDisplayName(v string) { e.displayName = v; e.EntBase.SetEntFieldChanged(1) }
func (e *Account) SetEmail(v string)       { e.email = v; e.EntBase.SetEntFieldChanged(2) }
func (e *Account) SetKind(v AccountKind)   { e.kind = v; e.EntBase.SetEntFieldChanged(3) }

// SetNameIfDifferent sets name only if v is different from the current value.
func (e *Account) SetNameIfDifferent(v string) bool {
	if e.name == v {
		return false
	}
	e.SetName(v)
	return true
}

// SetDisplayNameIfDifferent sets displayName only if v is different from the current value.
func (e *Account) SetDisplayNameIfDifferent(v string) bool {
	if e.displayName == v {
		return false
	}
	e.SetDisplayName(v)
	return true
}

// SetEmailIfDifferent sets email only if v is different from the current value.
func (e *Account) SetEmailIfDifferent(v string) bool {
	if e.email == v {
		return false
	}
	e.SetEmail(v)
	return true
}

// SetKindIfDifferent sets kind only if v is different from the current value.
func (e *Account) SetKindIfDifferent(v AccountKind) bool {
	if e.kind == v {
		return false
	}
	e.SetKind(v)
	return true
}

// ---- encode & decode methods ----

func (e *Account) EntEncode(c ent.Encoder, fields ent.FieldSet) {
	if fields.Has(0) {
		c.Key("name")
		c.Str(e.name)
	}
	if fields.Has(1) {
		c.Key("alias")
		c.Str(e.displayName)
	}
	if fields.Has(2) {
		c.Key("email")
		c.Str(e.email)
	}
	if fields.Has(3) {
		c.Key("kind")
		c.Int(int64(e.kind), 32)
	}
}

// EntDecode populates fields from a decoder
func (e *Account) EntDecode(c ent.Decoder) (id, version uint64) {
	for {
		switch string(c.Key()) {
		case "":
			return
		case ent.FieldNameId:
			id = c.Uint(64)
		case ent.FieldNameVersion:
			version = c.Uint(64)
		case "name":
			e.name = c.Str()
		case "alias":
			e.displayName = c.Str()
		case "email":
			e.email = c.Str()
		case "kind":
			e.kind = AccountKind(c.Int(32))
		default:
			c.Discard()
		}
	}
	return
}

// EntDecodePartial is used internally by ent.Storage during updates.
func (e *Account) EntDecodePartial(c ent.Decoder, fields ent.FieldSet) (version uint64) {
	for n := 2; n > 0; {
		switch string(c.Key()) {
		case "":
			return
		case ent.FieldNameVersion:
			version = c.Uint(64)
			continue
		case "email":
			n--
			if fields.Has(2) {
				e.email = c.Str()
				continue
			}
		case "kind":
			n--
			if fields.Has(3) {
				e.kind = AccountKind(c.Int(32))
				continue
			}
		}
		c.Discard()
	}
	return
}

// Symbolic field indices, for use with ent.*FieldChanged methods
const (
	ent_Account_f_name        = 0
	ent_Account_f_displayName = 1
	ent_Account_f_email       = 2
	ent_Account_f_kind        = 3
)

// EntFields returns information about Account fields
var ent_Account_fields = ent.Fields{
	Names: []string{
		"name",
		"alias",
		"email",
		"kind",
	},
	FieldSet: 0b1111,
}

// EntFields returns information about Account fields
func (e Account) EntFields() ent.Fields { return ent_Account_fields }

// Indexes (Name, Fields, Flags)
var ent_Account_idx = []ent.EntIndex{
	{"email", 1 << ent_Account_f_email, ent.EntIndexUnique},
	{"kind", 1 << ent_Account_f_kind, 0},
}

// EntIndexes returns information about secondary indexes
func (e *Account) EntIndexes() []ent.EntIndex { return ent_Account_idx }

// ---- helpers ----

func ent_Account_slice_cast(s []ent.Ent) []*Account {
	v := make([]*Account, len(s))
	for i := 0; i < len(s); i++ {
		v[i] = s[i].(*Account)
	}
	return v
}
