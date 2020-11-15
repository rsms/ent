// +build !entgen

// Code generated by entgen. DO NOT EDIT. by entgen. Edit with caution!
package main

import (
	"github.com/rsms/ent"
	"github.com/rsms/go-uuid"
)

// ----------------------------------------------------------------------------
// Account

// LoadAccountById loads Account with id from storage
func LoadAccountById(storage ent.Storage, id uint64) (*Account, error) {
	e := &Account{}
	return e, ent.LoadEntById(e, storage, id)
}

// LoadAccountByEmail loads Account with email
func LoadAccountByEmail(s ent.Storage, email string) (*Account, error) {
	e := &Account{}
	err := ent.LoadEntByIndexKey(s, e, &ent_Account_idx[0], []byte(email))
	return e, err
}

// FindAccountByEmail looks up Account id with email
func FindAccountByEmail(s ent.Storage, email string) (uint64, error) {
	return ent.FindEntIdByIndexKey(s, "account", &ent_Account_idx[0], []byte(email))
}

// LoadAccountByFlag loads all Account ents with flag
func LoadAccountByFlag(s ent.Storage, flag uint16) ([]*Account, error) {
	e := &Account{}
	r, err := ent.LoadEntsByIndex(s, e, &ent_Account_idx[1], 1, func(c ent.Encoder) {
		c.Uint(uint64(flag), 16)
	})
	return ent_Account_slice_cast(r), err
}

// FindAccountByFlag looks up Account ids with flag
func FindAccountByFlag(s ent.Storage, flag uint16) ([]uint64, error) {
	return ent.FindEntIdsByIndex(s, "account", &ent_Account_idx[1], 1, func(c ent.Encoder) {
		c.Uint(uint64(flag), 16)
	})
}

// LoadAccountByPicture loads all Account ents with picture
func LoadAccountByPicture(s ent.Storage, picture []byte) ([]*Account, error) {
	e := &Account{}
	r, err := s.LoadEntsByIndex(e, &ent_Account_idx[2], picture)
	return ent_Account_slice_cast(r), err
}

// FindAccountByPicture looks up Account ids with picture
func FindAccountByPicture(s ent.Storage, picture []byte) ([]uint64, error) {
	return s.FindEntIdsByIndex("account", &ent_Account_idx[2], picture)
}

// LoadAccountByScore loads all Account ents with score
func LoadAccountByScore(s ent.Storage, score float32) ([]*Account, error) {
	e := &Account{}
	r, err := ent.LoadEntsByIndex(s, e, &ent_Account_idx[3], 1, func(c ent.Encoder) {
		c.Float(float64(score), 32)
	})
	return ent_Account_slice_cast(r), err
}

// FindAccountByScore looks up Account ids with score
func FindAccountByScore(s ent.Storage, score float32) ([]uint64, error) {
	return ent.FindEntIdsByIndex(s, "account", &ent_Account_idx[3], 1, func(c ent.Encoder) {
		c.Float(float64(score), 32)
	})
}

// LoadAccountBySize loads all Account ents matching width AND height
func LoadAccountBySize(s ent.Storage, width, height int) ([]*Account, error) {
	e := &Account{}
	r, err := ent.LoadEntsByIndex(s, e, &ent_Account_idx[4], 2, func(c ent.Encoder) {
		c.Key("w")
		c.Int(int64(width), 64)
		c.Key("h")
		c.Int(int64(height), 64)
	})
	return ent_Account_slice_cast(r), err
}

// FindAccountBySize looks up Account ids matching width AND height
func FindAccountBySize(s ent.Storage, width, height int) ([]uint64, error) {
	return ent.FindEntIdsByIndex(s, "account", &ent_Account_idx[4], 2, func(c ent.Encoder) {
		c.Key("w")
		c.Int(int64(width), 64)
		c.Key("h")
		c.Int(int64(height), 64)
	})
}

// LoadAccountByUuid loads Account with uuid_
func LoadAccountByUuid(s ent.Storage, uuid_ uuid.UUID) (*Account, error) {
	e := &Account{}
	err := ent.LoadEntByIndex(s, e, &ent_Account_idx[5], func(c ent.Encoder) {
		c.Blob(uuid_[:])
	})
	return e, err
}

// FindAccountByUuid looks up Account id with uuid_
func FindAccountByUuid(s ent.Storage, uuid_ uuid.UUID) (uint64, error) {
	return ent.FindEntIdByIndex(s, "account", &ent_Account_idx[5], func(c ent.Encoder) {
		c.Blob(uuid_[:])
	})
}

// EntTypeName returns the ent's storage name ("account")
func (e Account) EntTypeName() string { return "account" }

// EntNew returns a new empty Account. Used by the ent package for loading ents.
func (e Account) EntNew() ent.Ent { return &Account{} }

// UnmarshalJSON populates the ent from JSON data. Conforms to json.Unmarshaler.
func (e *Account) UnmarshalJSON(b []byte) error { return ent.JsonDecode(e, b) }

// Create a new account ent in storage
func (e *Account) Create(storage ent.Storage) error { return ent.CreateEnt(e, storage) }

// Save pending changes to whatever storage this ent was created or loaded from
func (e *Account) Save() error { return ent.SaveEnt(e) }

// Reload fields to latest values from storage, discarding any unsaved changes
func (e *Account) Reload() error { return ent.ReloadEnt(e) }

// PermanentlyDelete deletes this ent from storage. This can usually not be undone.
func (e *Account) PermanentlyDelete() error { return ent.DeleteEnt(e) }

// ---- field accessor methods ----

func (e *Account) Name() string { return e.name }

// width in pixels
func (e *Account) Width() int { return e.width }

// width in pixels
func (e *Account) Height() int            { return e.height }
func (e *Account) Uuid() uuid.UUID        { return e.uuid }
func (e *Account) Flag() uint16           { return e.flag }
func (e *Account) Score() float32         { return e.score }
func (e *Account) Picture() []byte        { return e.picture }
func (e *Account) Email() string          { return e.email }
func (e *Account) EmailVerified() bool    { return e.emailVerified }
func (e *Account) PasswordHash() string   { return e.passwordHash }
func (e *Account) Thing() Thing3          { return e.thing }
func (e *Account) Foo() []int             { return e.foo }
func (e *Account) Foofoo() [][]int16      { return e.foofoo }
func (e *Account) Data() Data             { return e.data }
func (e *Account) Rgb() [3]int            { return e.rgb }
func (e *Account) Threebytes() [3]byte    { return e.threebytes }
func (e *Account) Things() map[string]int { return e.things }

func (e *Account) SetName(v string)           { e.name = v; ent.SetFieldChanged(&e.EntBase, 0) }
func (e *Account) setWidth(v int)             { e.width = v; ent.SetFieldChanged(&e.EntBase, 1) }
func (e *Account) SetHeight(v int)            { e.height = v; ent.SetFieldChanged(&e.EntBase, 2) }
func (e *Account) SetUuid(v uuid.UUID)        { e.uuid = v; ent.SetFieldChanged(&e.EntBase, 3) }
func (e *Account) SetFlag(v uint16)           { e.flag = v; ent.SetFieldChanged(&e.EntBase, 4) }
func (e *Account) SetScore(v float32)         { e.score = v; ent.SetFieldChanged(&e.EntBase, 5) }
func (e *Account) SetPicture(v []byte)        { e.picture = v; ent.SetFieldChanged(&e.EntBase, 6) }
func (e *Account) SetEmail(v string)          { e.email = v; ent.SetFieldChanged(&e.EntBase, 7) }
func (e *Account) SetEmailVerified(v bool)    { e.emailVerified = v; ent.SetFieldChanged(&e.EntBase, 8) }
func (e *Account) SetDeleted(v bool)          { e.Deleted = v; ent.SetFieldChanged(&e.EntBase, 9) }
func (e *Account) SetPasswordHash(v string)   { e.passwordHash = v; ent.SetFieldChanged(&e.EntBase, 10) }
func (e *Account) SetThing(v Thing3)          { e.thing = v; ent.SetFieldChanged(&e.EntBase, 11) }
func (e *Account) SetFoo(v []int)             { e.foo = v; ent.SetFieldChanged(&e.EntBase, 12) }
func (e *Account) SetFoofoo(v [][]int16)      { e.foofoo = v; ent.SetFieldChanged(&e.EntBase, 13) }
func (e *Account) SetData(v Data)             { e.data = v; ent.SetFieldChanged(&e.EntBase, 14) }
func (e *Account) SetRgb(v [3]int)            { e.rgb = v; ent.SetFieldChanged(&e.EntBase, 15) }
func (e *Account) SetThreebytes(v [3]byte)    { e.threebytes = v; ent.SetFieldChanged(&e.EntBase, 16) }
func (e *Account) SetThings(v map[string]int) { e.things = v; ent.SetFieldChanged(&e.EntBase, 17) }

func (e *Account) SetUuidChanged()       { ent.SetFieldChanged(&e.EntBase, 3) }
func (e *Account) SetPictureChanged()    { ent.SetFieldChanged(&e.EntBase, 6) }
func (e *Account) SetFooChanged()        { ent.SetFieldChanged(&e.EntBase, 12) }
func (e *Account) SetFoofooChanged()     { ent.SetFieldChanged(&e.EntBase, 13) }
func (e *Account) SetDataChanged()       { ent.SetFieldChanged(&e.EntBase, 14) }
func (e *Account) SetRgbChanged()        { ent.SetFieldChanged(&e.EntBase, 15) }
func (e *Account) SetThreebytesChanged() { ent.SetFieldChanged(&e.EntBase, 16) }
func (e *Account) SetThingsChanged()     { ent.SetFieldChanged(&e.EntBase, 17) }

// ---- encode & decode methods ----

func (e *Account) EntEncode(c ent.Encoder, fields uint64) {
	if (fields & (1 << 0)) != 0 {
		c.Key("name")
		c.Str(e.name)
	}
	if (fields & (1 << 1)) != 0 {
		c.Key("w")
		c.Int(int64(e.width), 64)
	}
	if (fields & (1 << 2)) != 0 {
		c.Key("h")
		c.Int(int64(e.height), 64)
	}
	if (fields & (1 << 3)) != 0 {
		c.Key("uuid")
		c.Blob(e.uuid[:])
	}
	if (fields & (1 << 4)) != 0 {
		c.Key("flag")
		c.Uint(uint64(e.flag), 16)
	}
	if (fields & (1 << 5)) != 0 {
		c.Key("score")
		c.Float(float64(e.score), 32)
	}
	if (fields & (1 << 6)) != 0 {
		c.Key("picture")
		c.Blob(e.picture)
	}
	if (fields & (1 << 7)) != 0 {
		c.Key("email")
		c.Str(e.email)
	}
	if (fields & (1 << 8)) != 0 {
		c.Key("email_verified")
		c.Bool(e.emailVerified)
	}
	if (fields & (1 << 9)) != 0 {
		c.Key("deleted")
		c.Bool(e.Deleted)
	}
	if (fields & (1 << 10)) != 0 {
		c.Key("pwhash")
		c.Str(e.passwordHash)
	}
	if (fields & (1 << 11)) != 0 {
		c.Key("thing")
		c.Int(int64(e.thing), 64)
	}
	if (fields & (1 << 12)) != 0 {
		c.Key("foo")
		ent_encode_Vi00(c, e.foo)
	}
	if (fields & (1 << 13)) != 0 {
		c.Key("foofoo")
		ent_encode_VVi02(c, e.foofoo)
	}
	if (fields & (1 << 14)) != 0 {
		c.Key("data")
		c.Blob(e.data)
	}
	if (fields & (1 << 15)) != 0 {
		c.Key("rgb")
		ent_encode_Vi00(c, e.rgb[:])
	}
	if (fields & (1 << 16)) != 0 {
		c.Key("threebytes")
		c.Blob(e.threebytes[:])
	}
	if (fields & (1 << 17)) != 0 {
		c.Key("things")
		ent_encode_Msi00(c, e.things)
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
		case "w":
			e.width = int(c.Int(64))
		case "h":
			e.height = int(c.Int(64))
		case "uuid":
			e.uuid = uuid.UUID(ent_slice_to_A16_u01(c.Blob()))
		case "flag":
			e.flag = uint16(c.Uint(16))
		case "score":
			e.score = float32(c.Float(32))
		case "picture":
			e.picture = c.Blob()
		case "email":
			e.email = c.Str()
		case "email_verified":
			e.emailVerified = c.Bool()
		case "deleted":
			e.Deleted = c.Bool()
		case "pwhash":
			e.passwordHash = c.Str()
		case "thing":
			e.thing = Thing3(c.Int(64))
		case "foo":
			e.foo = ent_decode_Vi00(c)
		case "foofoo":
			e.foofoo = ent_decode_VVi02(c)
		case "data":
			e.data = Data(c.Blob())
		case "rgb":
			e.rgb = ent_slice_to_A3_i00(ent_decode_Vi00(c))
		case "threebytes":
			e.threebytes = ent_slice_to_A3_u01(c.Blob())
		case "things":
			e.things = ent_decode_Msi00(c)
		default:
			c.Discard()
		}
	}
	return
}

// EntDecodePartial is used internally by ent.Storage during updates.
func (e *Account) EntDecodePartial(c ent.Decoder, fields uint64) (version uint64) {
	for n := 7; n > 0; {
		switch string(c.Key()) {
		case "":
			return
		case ent.FieldNameVersion:
			version = c.Uint(64)
			continue
		case "w":
			n--
			if (fields & (1 << 1)) != 0 {
				e.width = int(c.Int(64))
				continue
			}
		case "h":
			n--
			if (fields & (1 << 2)) != 0 {
				e.height = int(c.Int(64))
				continue
			}
		case "uuid":
			n--
			if (fields & (1 << 3)) != 0 {
				e.uuid = uuid.UUID(ent_slice_to_A16_u01(c.Blob()))
				continue
			}
		case "flag":
			n--
			if (fields & (1 << 4)) != 0 {
				e.flag = uint16(c.Uint(16))
				continue
			}
		case "score":
			n--
			if (fields & (1 << 5)) != 0 {
				e.score = float32(c.Float(32))
				continue
			}
		case "picture":
			n--
			if (fields & (1 << 6)) != 0 {
				e.picture = c.Blob()
				continue
			}
		case "email":
			n--
			if (fields & (1 << 7)) != 0 {
				e.email = c.Str()
				continue
			}
		}
		c.Discard()
	}
	return
}

// Symbolic field indices, for use with ent.*FieldChanged methods
const (
	ent_Account_f_name          = 0
	ent_Account_f_width         = 1
	ent_Account_f_height        = 2
	ent_Account_f_uuid          = 3
	ent_Account_f_flag          = 4
	ent_Account_f_score         = 5
	ent_Account_f_picture       = 6
	ent_Account_f_email         = 7
	ent_Account_f_emailVerified = 8
	ent_Account_f_Deleted       = 9
	ent_Account_f_passwordHash  = 10
	ent_Account_f_thing         = 11
	ent_Account_f_foo           = 12
	ent_Account_f_foofoo        = 13
	ent_Account_f_data          = 14
	ent_Account_f_rgb           = 15
	ent_Account_f_threebytes    = 16
	ent_Account_f_things        = 17
)

// EntFields returns information about Account fields
var ent_Account_fields = ent.Fields{
	Names: []string{
		"name",
		"w",
		"h",
		"uuid",
		"flag",
		"score",
		"picture",
		"email",
		"email_verified",
		"deleted",
		"pwhash",
		"thing",
		"foo",
		"foofoo",
		"data",
		"rgb",
		"threebytes",
		"things",
	},
	Fieldmap: 0b111111111111111111,
}

// EntFields returns information about Account fields
func (e Account) EntFields() ent.Fields { return ent_Account_fields }

// Indexes (Name, Fields, Flags)
var ent_Account_idx = []ent.EntIndex{
	{"email", 1 << ent_Account_f_email, ent.EntIndexUnique},
	{"flag", 1 << ent_Account_f_flag, 0},
	{"picture", 1 << ent_Account_f_picture, 0},
	{"score", 1 << ent_Account_f_score, 0},
	{"size", (1 << ent_Account_f_width) | (1 << ent_Account_f_height), 0},
	{"uuid", 1 << ent_Account_f_uuid, ent.EntIndexUnique},
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

func ent_encode_Vi00(c ent.Encoder, v []int) {
	c.BeginList(len(v))
	for _, val := range v {
		c.Int(int64(val), 64)
	}
	c.EndList()
}

func ent_encode_Vi02(c ent.Encoder, v []int16) {
	c.BeginList(len(v))
	for _, val := range v {
		c.Int(int64(val), 16)
	}
	c.EndList()
}

func ent_encode_VVi02(c ent.Encoder, v [][]int16) {
	c.BeginList(len(v))
	for _, val := range v {
		ent_encode_Vi02(c, val)
	}
	c.EndList()
}

func ent_encode_Msi00(c ent.Encoder, v map[string]int) {
	c.BeginDict(len(v))
	for k, val := range v {
		c.Key(k)
		c.Int(int64(val), 64)
	}
	c.EndDict()
}

func ent_slice_to_A16_u01(s []byte) (r [16]byte) {
	copy(r[:], s)
	return
}

func ent_decode_Vi00(c ent.Decoder) (r []int) {
	n := c.ListHeader()
	if n > -1 {
		r = make([]int, 0, n)
		for i := 0; i < n; i++ {
			r = append(r, int(c.Int(64)))
		}
	} else {
		for c.More() {
			r = append(r, int(c.Int(64)))
		}
	}
	return
}

func ent_decode_Vi02(c ent.Decoder) (r []int16) {
	n := c.ListHeader()
	if n > -1 {
		r = make([]int16, 0, n)
		for i := 0; i < n; i++ {
			r = append(r, int16(c.Int(16)))
		}
	} else {
		for c.More() {
			r = append(r, int16(c.Int(16)))
		}
	}
	return
}

func ent_decode_VVi02(c ent.Decoder) (r [][]int16) {
	n := c.ListHeader()
	if n > -1 {
		r = make([][]int16, 0, n)
		for i := 0; i < n; i++ {
			r = append(r, ent_decode_Vi02(c))
		}
	} else {
		for c.More() {
			r = append(r, ent_decode_Vi02(c))
		}
	}
	return
}

func ent_slice_to_A3_i00(s []int) (r [3]int) {
	copy(r[:], s)
	return
}

func ent_slice_to_A3_u01(s []byte) (r [3]byte) {
	copy(r[:], s)
	return
}

func ent_decode_Msi00(c ent.Decoder) (r map[string]int) {
	n := c.DictHeader()
	r = make(map[string]int, n)
	if n > -1 {
		for i := 0; i < n; i++ {
			k := c.Key()
			r[k] = int(c.Int(64))
		}
	} else {
		for c.More() {
			k := c.Key()
			r[k] = int(c.Int(64))
		}
	}
	return
}
