package ent

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/rsms/go-bits"
)

// IndexGetter is used to look up an entry in an index
type IndexGetter = func(entTypeName, indexName, key string) ([]uint64, error)

// StorageIndexEdit represents a modification to an index
type StorageIndexEdit struct {
	Index *EntIndex
	Key   string
	Value []uint64

	// IsCleanup is true when the edit represents removing an id for an index entry.
	// A Storage implementation can choose to perform these edits independently from or outside of
	// the logical transaction of modifying an ent.
	IsCleanup bool
}

// CalcStorageIndexEdits calculates changes to secondary indexes.
//
// When a new ent is created, prevEnt should be nil.
// When an ent is deleted, nextEnt should be nil.
// When an ent is modified, both nextEnt and prevEnt should be provided.
//
// When a new ent is modified or deleted, prevEnt should be the "current" version of the ent.
// In these cases prevEnt's field values are used to determine what index entries needs to be
// cleaned up.
//
// prevEnt only needs to loaded fields which indices are the union of all the prevEnt.EntIndexes()
// Fields values. Example:
//
//   var fieldsToLoad uint64
//   for _, x := range nextEnt.EntIndexes() {
//     fieldsToLoad |= x.Fields
//   }
//   DecodePartialEnt(prevEnt, fieldsToLoad)
//
func CalcStorageIndexEdits(
	indexGet IndexGetter,
	nextEnt, prevEnt Ent,
	id, changedFields uint64,
) ([]StorageIndexEdit, error) {
	// Example:
	//
	// database:
	//   ent1 => {name "Alan"}
	//   ent2 => {name "Alan"}
	//   ent3 => {name "Cat"}
	//
	// main index: (non-unique)
	//   index:name Alan => [ent1, ent2]
	//   index:name Cat  => [ent3]
	//
	// reverse index:
	//   index:name ent1 => Alan
	//   index:name ent2 => Alan
	//   index:name ent3 => Cat
	//
	// When name of ent1 changes to "Ali" we need to UPDATE the index:
	//   Alt 1: Using reverse index:
	//     1. lookup ent1 in the reverse index: "index:name ent1"="Alan"
	//     2. remove ent1 from the main index:  "index:name Alan" [ent1, ent2] => [ent2]
	//     3. add entry to the main index:      "index:name Ali" [] => [ent1]
	//     4. add entry to the reverse index:   "index:name ent1"="Ali"
	//
	//   Alt 2: Get past value by lookup before writing new value:
	//     1. lookup pastValue in database:              "ent1"["name"] = "Alan"
	//     2. remove pastValue+ent1 from the main index: "index:name Alan" [ent1, ent2] => [ent2]
	//     3. add entry to the main index:               "index:name Ali" [] => [ent1]
	//

	entTypeName := nextEnt.EntTypeName()

	// if there is no previous ent, mark all fields as changed to ensure that a newly
	// created ent's indexes are all properly created.
	if prevEnt == nil {
		changedFields = fieldmapAll
	} else if prevEnt.EntTypeName() != entTypeName {
		return nil, fmt.Errorf("different ent types (%s, %s)", prevEnt.EntTypeName(), entTypeName)
	}

	// allocate the max number of edits we may need up front
	edits := make([]StorageIndexEdit, 0, bits.PopcountUint64(changedFields)*2)

	// reusable index key encoder
	var indexKeyEncoder IndexKeyEncoder

	// for each index...
	indexes := nextEnt.EntIndexes()
	for i := range indexes {
		x := &indexes[i] // *EntIndex

		if (changedFields & x.Fields) == 0 {
			// none of the fields that this index depends on has changed
			// fmt.Printf("[CalcStorageIndexEdits] index %s unaffected (not in changedFields)\n", x.Name)
			continue
		}
		// fmt.Printf("[CalcStorageIndexEdits] index %s is affected\n", x.Name)

		isUnique := (x.Flags & EntIndexUnique) != 0

		// build index entry keys
		var prevValueKey, nextValueKey string
		if prevEnt != nil {
			data, err := indexKeyEncoder.EncodeKey(prevEnt, x.Fields)
			if err != nil {
				return nil, err
			}
			prevValueKey = string(data)
		}
		if nextEnt != nil {
			data, err := indexKeyEncoder.EncodeKey(nextEnt, x.Fields)
			if err != nil {
				return nil, err
			}
			nextValueKey = string(data)
		}

		// fmt.Printf("[CalcStorageIndexEdits] prevValueKey %q\n", prevValueKey)
		// fmt.Printf("[CalcStorageIndexEdits] nextValueKey %q\n", nextValueKey)

		// remove old entry
		if prevValueKey != "" {
			// identical keys? skip index changes.
			// This happens if the same value is written to the field, which isn't uncommon.
			if prevValueKey == nextValueKey {
				// fmt.Printf("index keys are identical; skip. (%q, %q)\n", prevValueKey, nextValueKey)
				continue
			}
			var ids idSet
			var err error
			ids, err = indexGet(entTypeName, x.Name, prevValueKey)
			if err != nil {
				return nil, err
			}
			if len(ids) > 0 {
				if isUnique || len(ids) == 1 {
					ids = nil
				} else {
					ids.Del(id)
				}
				// prevValueKey is borrowed from indexKeyEncoder
				edits = append(edits, StorageIndexEdit{
					Index:     x,
					Key:       prevValueKey,
					Value:     ids,
					IsCleanup: true,
				})
			}
		}

		// add new entry
		if nextValueKey != "" {
			var ids idSet
			if isUnique {
				ids = idSet{id}
			} else {
				var err error
				ids, err = indexGet(entTypeName, x.Name, nextValueKey)
				if err != nil {
					return nil, err
				}
				ids.Add(id)
			}
			edits = append(edits, StorageIndexEdit{
				Index:     x,
				Key:       nextValueKey,
				Value:     ids,
				IsCleanup: false,
			})
		}
	} // for each index

	return edits, nil
}

// ———————————————————————————————————————————————————————————————————————————————————
// Helper functions which job is to reduce amount of boiler-plate code for
// generated ent index lookup functions.

func FindEntIdsByIndex(
	s Storage, entTypeName, indexName string, nfields int, keyEncoder func(Encoder),
) ([]uint64, error) {
	var c IndexKeyEncoder
	c.Reset(nfields)
	keyEncoder(&c)
	if c.err != nil {
		return nil, c.err
	}
	c.EndEnt()
	return s.FindEntIdsByIndex(entTypeName, indexName, c.b.Bytes())
}

func FindEntIdByIndex(
	s Storage, entTypeName, indexName string, keyEncoder func(Encoder),
) (uint64, error) {
	v, err := FindEntIdsByIndex(s, entTypeName, indexName, 1, keyEncoder)
	if err == nil {
		if len(v) > 0 {
			return v[0], nil
		}
		err = ErrNotFound
	}
	return 0, err
}

func FindEntIdByIndexKey(
	s Storage, entTypeName, indexName string, key []byte,
) (uint64, error) {
	v, err := s.FindEntIdsByIndex(entTypeName, indexName, key)
	if err == nil {
		if len(v) > 0 {
			return v[0], nil
		}
		err = ErrNotFound
	}
	return 0, err
}

func LoadEntsByIndex(
	s Storage, e Ent, indexName string, nfields int, keyEncoder func(Encoder),
) ([]Ent, error) {
	var c IndexKeyEncoder
	c.Reset(nfields)
	keyEncoder(&c)
	if c.err != nil {
		return nil, c.err
	}
	c.EndEnt()
	return s.LoadEntsByIndex(e, indexName, c.b.Bytes())
}

func LoadEntByIndex(s Storage, e Ent, indexName string, keyEncoder func(Encoder)) error {
	v, err := LoadEntsByIndex(s, e, indexName, 1, keyEncoder)
	if err == nil {
		if len(v) == 0 {
			err = ErrNotFound
		} else if v[0] != e {
			// sanity check, in case a storage implementation does not use e for its first result
			err = fmt.Errorf("internal storage error: LoadEntsByIndex did not update e")
		}
	}
	return err
}

func LoadEntByIndexKey(s Storage, e Ent, indexName string, key []byte) error {
	v, err := s.LoadEntsByIndex(e, indexName, key)
	if err == nil {
		if len(v) == 0 {
			err = ErrNotFound
		} else if v[0] != e {
			// sanity check, in case a storage implementation does not use e for its first result
			err = fmt.Errorf("internal storage error: LoadEntsByIndex did not update e")
		}
	}
	return err
}

// ———————————————————————————————————————————————————————————————————————————————————

// IndexKeyEncoder is an implementation of the Encoder interface, used to encode index keys
type IndexKeyEncoder struct {
	b    Buffer
	err  error
	nest int

	nfields int
	keys    []string
	values  []string
}

func (c *IndexKeyEncoder) EncodeKey(e Ent, fieldmap uint64) ([]byte, error) {
	c.Reset(bits.PopcountUint64(fieldmap))
	e.EntEncode(c, fieldmap)
	c.EndEnt()
	return c.b.Bytes(), c.err
}

func (c *IndexKeyEncoder) Reset(nfields int) {
	c.nfields = nfields
	c.b.Reset()
	if c.values != nil {
		c.values = c.values[:0]
		c.keys = c.keys[:0]
	}
	c.nest = 0
}

func (c *IndexKeyEncoder) Err() error { return c.err }
func (c *IndexKeyEncoder) setErr(err error) {
	if c.err == nil {
		c.err = err
	}
}

func (c *IndexKeyEncoder) BeginEnt(version uint64) {}

func (c *IndexKeyEncoder) EndEnt() {
	if c.nfields > 1 && c.err == nil {
		c.flush()
	}
}

type keysValuesSort IndexKeyEncoder

func (a *keysValuesSort) Len() int { return len(a.keys) }
func (a *keysValuesSort) Swap(i, j int) {
	a.keys[i], a.keys[j] = a.keys[j], a.keys[i]
	a.values[i], a.values[j] = a.values[j], a.values[i]
}
func (a *keysValuesSort) Less(i, j int) bool { return a.keys[i] < a.keys[j] }

func (c *IndexKeyEncoder) flush() {
	if len(c.values) > 0 {
		if len(c.values) == 1 {
			c.b.WriteString(c.values[0])
		} else {
			// sorted key-value pairs
			if len(c.keys) != len(c.values) {
				c.setErr(fmt.Errorf("unbalanced key-value: %d keys, %d values",
					len(c.keys), len(c.values)))
				return
			}
			sort.Sort((*keysValuesSort)(c))
			for i, k := range c.keys {
				if i > 0 {
					c.b.WriteByte('\xff')
				}
				c.b.WriteString(k)
				c.b.WriteByte('\xff')
				c.b.WriteString(c.values[i])
			}
			// // sorted values
			// sort.Strings(c.values)
			// c.b.WriteString(strings.Join(c.values, "\xff"))
		}
	}
}

func (c *IndexKeyEncoder) Key(k string) {
	if c.nest == 0 {
		if c.nfields > 1 {
			c.keys = append(c.keys, k)
		}
	} else {
		c.b.WriteString(k)
		c.b.WriteByte(':')
		// TODO write key
	}
}

func (c *IndexKeyEncoder) BeginList(length int) {
	// TODO: append "[" + varint(length) to c.values
	c.setErr(fmt.Errorf("can't index lists"))
	c.nest++
}
func (c *IndexKeyEncoder) EndList() {
	c.nest--
}
func (c *IndexKeyEncoder) BeginDict(length int) {
	// TODO: append "[" + varint(length) to c.values
	c.setErr(fmt.Errorf("can't index dicts"))
	c.nest++
}
func (c *IndexKeyEncoder) EndDict() {
	c.nest--
}

func (c *IndexKeyEncoder) Str(v string) {
	if c.nfields == 1 && c.nest == 0 {
		c.b.WriteString(v)
	} else {
		// if c.nest > 0 {
		//   // TODO: append "s" + varint(length) & v to c.values
		// }
		for _, c := range v {
			if c == '\xff' {
				v = strings.ReplaceAll(v, "\xff", "\\xff")
				break
			}
		}
		c.values = append(c.values, v)
	}
}

func (c *IndexKeyEncoder) Blob(v []byte) {
	if c.nfields == 1 && c.nest == 0 {
		c.b.WriteString(string(v))
	} else {
		c.setErr(fmt.Errorf("can't index nested blobs"))
	}
}

func (c *IndexKeyEncoder) Int(v int64, bitsize int) {
	c.Uint(uint64(v), bitsize)
}

func (c *IndexKeyEncoder) Uint(v uint64, bitsize int) {
	if c.nfields == 1 && c.nest == 0 {
		switch bitsize {
		case 8:
			c.b.WriteByte(uint8(v))
		case 16:
			i := c.b.Grow(2)
			writeUint16BE(c.b[i:i+2], uint16(v))
		case 32:
			i := c.b.Grow(4)
			writeUint32BE(c.b[i:i+4], uint32(v))
		default:
			i := c.b.Grow(8)
			writeUint64BE(c.b[i:i+8], v)
		}
		// fmt.Printf("c.b.Bytes(): %#v\n", c.b.Bytes())
	} else {
		c.values = append(c.values, strconv.FormatUint(v, 36))
	}
}

func (c *IndexKeyEncoder) Float(v float64, bitsize int) {
	if c.nfields == 1 && c.nest == 0 {
		c.b = c.appendFloatValue(c.b, v, bitsize)
	} else {
		var buf [32]byte
		b := c.appendFloatValue(buf[:], v, bitsize)
		c.values = append(c.values, string(b))
	}
}

func (c *IndexKeyEncoder) Bool(v bool) {
	b := uint8(0)
	if v {
		b = 1
	}
	if c.nfields == 1 && c.nest == 0 {
		c.b.WriteByte(b)
	} else {
		c.values = append(c.values, string([]byte{0x30 + b})) // "0" or "1"
	}
}

// appendFloatValue appends a JavaScript-style float64 number of bits size to b
func (c *IndexKeyEncoder) appendFloatValue(b []byte, f float64, bits int) []byte {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		c.setErr(fmt.Errorf("unsupported float64 value %s",
			strconv.FormatFloat(f, 'g', -1, bits)))
		return b
	}
	// Convert as if by ES6 number to string conversion.
	// This matches most other JSON generators.
	// See golang.org/issue/6384 and golang.org/issue/14135.
	// Like fmt %g, but the exponent cutoffs are different
	// and exponents themselves are not padded to two digits.
	abs := math.Abs(f)
	fmt := byte('f')
	// Note: Must use float32 comparisons for underlying float32 value to get precise cutoffs right.
	if abs != 0 {
		if bits == 64 && (abs < 1e-6 || abs >= 1e21) ||
			bits == 32 && (float32(abs) < 1e-6 ||
				float32(abs) >= 1e21) {
			fmt = 'e'
		}
	}
	b = strconv.AppendFloat(b, f, fmt, -1, int(bits))
	if fmt == 'e' {
		// clean up e-09 to e-9
		n := len(b)
		if n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	return b
}

// writeUint64BE writes a uint64 in big-endian encoding
func writeUint64BE(b []byte, v uint64) {
	b[7] = byte(v) // early bounds check
	b[6] = byte(v >> 8)
	b[5] = byte(v >> 16)
	b[4] = byte(v >> 24)
	b[3] = byte(v >> 32)
	b[2] = byte(v >> 40)
	b[1] = byte(v >> 48)
	b[0] = byte(v >> 56)
}

func writeUint32BE(b []byte, v uint32) {
	b[3] = byte(v)
	b[2] = byte(v >> 8)
	b[1] = byte(v >> 16)
	b[0] = byte(v >> 24)
}

func writeUint16BE(b []byte, v uint16) {
	b[1] = byte(v)
	b[0] = byte(v >> 8)
}
