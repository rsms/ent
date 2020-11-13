package ent

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
)

// MemoryStorage is a generic, goroutine-safe storage implementation which maintains
// ent data in memory, suitable for tests.
type MemoryStorage struct {
	idgen uint64 // id generator for creating new ents

	mu sync.RWMutex // protects the following fields
	m  scopedMap    // entkey => json
}

func NewMemoryStorage() *MemoryStorage {
	s := &MemoryStorage{}
	s.m.m = make(map[string][]byte)
	return s
}

func (s *MemoryStorage) CreateEnt(e Ent, fieldmap uint64) (id uint64, err error) {
	id = atomic.AddUint64(&s.idgen, 1)
	err = s.putEnt(e, id, 1, fieldmap)
	return
}

func (s *MemoryStorage) SaveEnt(e Ent, fieldmap uint64) (nextVersion uint64, err error) {
	nextVersion = e.Version() + 1
	err = s.putEnt(e, e.Id(), nextVersion, fieldmap)
	return
}

func (s *MemoryStorage) LoadEntById(e Ent, id uint64) (version uint64, err error) {
	key := s.entKey(e.EntTypeName(), id)
	s.mu.RLock()
	data := s.m.Get(key)
	s.mu.RUnlock()
	return s.loadEnt(e, data)
}

func (s *MemoryStorage) loadEnt(e Ent, data []byte) (version uint64, err error) {
	if data == nil {
		err = ErrNotFound
		return
	}
	_, version, err = jsonDecodeEnt(e, data) // note: ignore "id" return value
	return
}

func (s *MemoryStorage) DeleteEnt(e Ent) error {
	key := s.entKey(e.EntTypeName(), e.Id())
	s.mu.Lock()
	ok := s.m.Get(key) != nil
	s.m.Del(key)
	s.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (s *MemoryStorage) putEnt(e Ent, id, version, changedFields uint64) error {
	fmt.Printf("\n -- putEnt --\n")

	// encode
	// Note: fieldmapAll is used here instead of changedFields, since the JSON encoding we use
	// doesn't support patching. Storage that writes fields to individual cells, like an SQL table
	// or key-value store entry may make use of fieldmap to store/update only modified fields.
	json, err := jsonEncodeEnt(e, id, version, fieldmapAll)
	if err != nil {
		return err
	}

	// lock read & write access to s.m, which we will read from (and edit at the end)
	s.mu.Lock()
	defer s.mu.Unlock()

	// fork storage, creating a new map scope to hold changes queued up in this transaction
	m := s.m.NewScope()

	// storage key
	key := s.entKey(e.EntTypeName(), id)

	// load & verify that the current version is what we are expecting
	expectVersion := e.Version()
	var prevEnt Ent
	if expectVersion != 0 {
		prevData := m.Get(key)
		if prevData != nil {
			// Make a new ent instance of the same type as e, then load it.
			// Effectively the same as calling LoadTYPE(id) but
			prevEnt = e.EntNew()
			currentVersion, err := jsonDecodeEntIndexed(prevEnt, prevData, changedFields)
			if err != nil {
				return err
			}
			if expectVersion != currentVersion {
				return NewVersionConflictErr(expectVersion, currentVersion)
			}
		}
	}

	// update indexes
	indexEdits, err := CalcStorageIndexEdits(s.indexGet, e, prevEnt, id, changedFields)
	if err != nil {
		return err
	}
	entTypeName := e.EntTypeName()
	for _, ed := range indexEdits {
		key := s.indexKey(entTypeName, ed.Index.Name, ed.Key)
		if len(ed.Value) == 0 {
			fmt.Printf("index del %q\n", key)
			m.Del(key)
		} else {
			fmt.Printf("index put %q => %v\n", key, ed.Value)
			m.Put(key, encodeIds(ed.Value))
		}
	}

	// success; apply queued changes of the transaction to the outer "root" map.
	// This effectively "commits" the transaction.
	m.ApplyToOuter()

	// write value
	fmt.Printf("storage put %q => %s\n", key, json)
	s.m.Put(key, json)
	// note that s.mu is locked with deferred unlock
	return nil
}

func (s *MemoryStorage) FindEntIdsByIndex(
	entTypeName, indexName string,
	key []byte,
) ([]uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.indexGet(entTypeName, indexName, string(key))
}

func (s *MemoryStorage) LoadEntsByIndex(e Ent, indexName string, key []byte) ([]Ent, error) {
	//
	// TODO: document the following thing somewhere, maybe in ent.Storage:
	//
	// Important: the first ent in return value []Ent should be e.
	// Generated code relies on this to avoid unnecessary type checks.
	//
	s.mu.RLock()
	defer s.mu.RUnlock()
	entTypeName := e.EntTypeName()
	ids, err := s.indexGet(entTypeName, indexName, string(key))
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, ErrNotFound
	}
	ents := make([]Ent, len(ids))
	for i, id := range ids {
		e2 := e
		if i > 0 {
			e2 = e.EntNew()
		}
		version, err := s.loadEnt(e2, s.m.Get(s.entKey(entTypeName, id)))
		if err != nil {
			return nil, err
		}
		SetEntBaseFieldsAfterLoad(e2, s, id, version)
		ents[i] = e2
	}
	return ents, nil
}

func (s *MemoryStorage) indexGet(entTypeName, indexName, key string) ([]uint64, error) {
	value := s.m.Get(s.indexKey(entTypeName, indexName, key))
	if len(value) == 0 {
		return nil, nil
	}
	return parseIdSet(value), nil
}

func (s *MemoryStorage) indexKey(entTypeName, indexName, key string) string {
	return entTypeName + "#" + indexName + ":" + key
}

func (s *MemoryStorage) entKey(entTypeName string, id uint64) string {
	if id == 0 {
		panic("zero id")
	}
	return entTypeName + ":" + strconv.FormatUint(id, 36)
}

// ———————————————————————————————————————————————————————————————————————————————————

// scopedMap is like a map[string][]byte but prototypal in behaviour; local read misses causes
// a parent scopedMap to be tried, while writes are always local. Sort of like a hacky HAMT map.
type scopedMap struct {
	outer *scopedMap
	m     map[string][]byte
}

func (s scopedMap) Get(key string) []byte {
	if s.m != nil {
		v, ok := s.m[key]
		if ok {
			return v
		}
	}
	if s.outer != nil {
		return s.outer.Get(key)
	}
	return nil
}

func (s *scopedMap) Put(key string, value []byte) {
	if value == nil {
		s.Del(key)
	} else {
		if s.m == nil {
			s.m = make(map[string][]byte)
		}
		s.m[key] = value
	}
}

func (s *scopedMap) Del(key string) {
	if s.outer == nil {
		delete(s.m, key)
	} else {
		if s.m == nil {
			s.m = make(map[string][]byte)
		}
		s.m[key] = nil
	}
}

func (s *scopedMap) NewScope() *scopedMap {
	return &scopedMap{outer: s}
}

// ApplyToOuter applies all entries (including deletes) of this scope to its outer scope.
// This effectively moves changes from this scope to the outer scope, clearing this scope.
func (s *scopedMap) ApplyToOuter() {
	for k, v := range s.m {
		s.outer.Put(k, v)
	}
	s.m = nil
}

// func init() {
//  var m1 scopedMap
//  m1.Put("a", []byte{'a'})
//  m1.Put("b", []byte{'b'})
//  m2 := m1.NewScope()
//  m2.Put("c", []byte{'c'})
//  m2.Del("b")

//  a := m2.Get("a")
//  b := m2.Get("b")
//  c := m2.Get("c")
//  fmt.Printf("scopedMap mini test:--------\n")
//  fmt.Printf("m2: a, b, c = %#v, %#v, %#v\n", a, b, c)

//  a = m1.Get("a")
//  b = m1.Get("b")
//  c = m1.Get("c")
//  fmt.Printf("m1: a, b, c = %#v, %#v, %#v\n", a, b, c)

//  fmt.Printf("m2.ApplyToOuter()\n")
//  m2.ApplyToOuter()

//  a = m1.Get("a")
//  b = m1.Get("b")
//  c = m1.Get("c")
//  fmt.Printf("m1: a, b, c = %#v, %#v, %#v\n", a, b, c)
//  fmt.Printf("m1.m: %#v\n", m1.m)
// }

// ------------------------------

// // getVersion reads the current version of e. Does not lock s.mu!
// func (s *MemoryStorage) getVersionUnlocked(e Ent, key string) (uint64, error) {
//  data := s.m.Get(key)
//  if data == nil {
//    return 0, ErrNotFound
//  }
//  // TODO: consider changing Ent.EntDecode implementation to accept a fieldmap to allow
//  // loading partial ents. Alternatively introduce a new Ent.EntDecodePartial method which does.
//  //
//  // This code here is a hand-written variant of EntDecode that only reads FieldNameVersion
//  c := NewJsonDecoder(data)
//  if c.DictHeader() != 0 {
//    for {
//      key := c.Key()
//      if key == "" {
//        break
//      }
//      if string(key) == FieldNameVersion {
//        return c.Uint(64), c.Err()
//      }
//      c.Discard()
//    }
//  }
//  return 0, ErrNotFound
// }
