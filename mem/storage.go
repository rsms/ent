package mem

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rsms/ent"
)

type Ent = ent.Ent

// EntStorage is a generic, goroutine-safe storage implementation of ent.Storage which maintains
// ent data in memory, suitable for tests.
type EntStorage struct {
	idgen uint64 // id generator for creating new ents

	mu sync.RWMutex // protects the following fields
	m  ScopedMap    // entkey => json
}

func NewEntStorage() *EntStorage {
	s := &EntStorage{}
	s.m.m = make(map[string][]byte)
	return s
}

func debugTrace(format string, args ...interface{}) {
	// The Go compiler will strip all invocations of debugTrace when the function body is empty.
	// (un)comment the next line to toggle debug trace logging:
	//fmt.Printf("TRACE "+format+"\n", args...)
}

func (s *EntStorage) Create(e Ent, fieldmap uint64) (id uint64, err error) {
	id = atomic.AddUint64(&s.idgen, 1)
	err = s.putEnt(e, id, 1, fieldmap)
	return
}

func (s *EntStorage) Save(e Ent, fieldmap uint64) (nextVersion uint64, err error) {
	nextVersion = e.Version() + 1
	err = s.putEnt(e, e.Id(), nextVersion, fieldmap)
	return
}

func (s *EntStorage) LoadById(e Ent, id uint64) (version uint64, err error) {
	key := s.entKey(e.EntTypeName(), id)
	s.mu.RLock()
	data := s.m.Get(key)
	s.mu.RUnlock()
	return s.loadEnt(e, data)
}

func (s *EntStorage) loadEnt(e Ent, data []byte) (version uint64, err error) {
	if data == nil {
		err = ent.ErrNotFound
		return
	}
	_, version, err = ent.JsonDecodeEnt(e, data) // note: ignore "id" return value
	return
}

func (s *EntStorage) Delete(e Ent, id uint64) error {
	allfields := e.EntFields().Fieldmap
	key := s.entKey(e.EntTypeName(), id)

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(e.EntIndexes()) == 0 {
		if s.m.Get(key) == nil {
			return ent.ErrNotFound
		}
	} else {
		// load latest data that indexes depends on
		if _, err := ent.JsonDecodeEntPartial(e, s.m.Get(key), allfields); err != nil {
			return err
		}

		// fork storage
		m := s.m.NewScope()

		// update indexes
		if err := s.updateIndexes(e, nil, id, allfields, m); err != nil {
			return err
		}

		// commit changes
		m.ApplyToOuter()
	}

	// remove ent
	s.m.Del(key)

	return nil
}

func (s *EntStorage) putEnt(e Ent, id, version, changedFields uint64) error {
	debugTrace("putEnt ent %q id=%d version=%d fieldmap=%b",
		e.EntTypeName(), id, version, changedFields)

	// encode
	// Note: EntFields().Fieldmap is used here instead of changedFields, since the JSON encoding
	// we use doesn't support patching. Storage that writes fields to individual cells,
	// like an SQL table or key-value store entry may make use of fieldmap to store/update
	// only modified fields.
	json, err := ent.JsonEncodeEnt(e, id, version, e.EntFields().Fieldmap, "")
	if err != nil {
		return err
	}

	// lock read & write access to s.m, which we will read from (and edit at the end)
	s.mu.Lock()
	defer s.mu.Unlock()

	// storage key
	key := s.entKey(e.EntTypeName(), id)

	// load & verify that the current version is what we are expecting
	expectVersion := e.Version()
	var prevEnt Ent
	if expectVersion != 0 {
		prevData := s.m.Get(key)
		if prevData != nil {
			// Make a new ent instance of the same type as e, then load it.
			// Effectively the same as calling LoadTYPE(id) but
			prevEnt = e.EntNew()
			currentVersion, err := ent.JsonDecodeEntPartial(prevEnt, prevData, changedFields)
			if err != nil {
				return err
			}
			if expectVersion != currentVersion {
				return ent.ErrVersionConflict
			}
		}
	}

	// fork storage, creating a new map scope to hold changes queued up in this transaction
	m := s.m.NewScope()

	// update indexes
	if err := s.updateIndexes(prevEnt, e, id, changedFields, m); err != nil {
		return err
	}

	// success; apply queued changes of the transaction to the outer "root" map.
	// This effectively "commits" the transaction.
	m.ApplyToOuter()

	// write value
	debugTrace("storage put %q => %s", key, json)
	s.m.Put(key, json)
	// note that s.mu is locked with deferred unlock
	return nil
}

func (s *EntStorage) updateIndexes(prevEnt, nextEnt Ent, id, fieldmap uint64, m *ScopedMap) error {
	// update indexes
	indexEdits, err := ent.ComputeIndexEdits(s.indexGet, prevEnt, nextEnt, id, fieldmap)
	if err != nil {
		return err
	}
	debugTrace("indexEdits %#v", indexEdits)

	var entType string
	if prevEnt != nil {
		entType = prevEnt.EntTypeName()
	} else {
		entType = nextEnt.EntTypeName()
	}

	for _, ed := range indexEdits {
		key := s.indexKey(entType, ed.Index.Name, ed.Key)
		if len(ed.Value) == 0 {
			debugTrace("index del %q", key)
			m.Del(key)
		} else {
			if !ed.IsCleanup && ed.Index.IsUnique() {
				// check for collision
				ids, err := s.indexGet(entType, ed.Index.Name, string(ed.Key))
				if err != nil {
					return err
				}
				if len(ids) > 0 {
					if ids[0] == id {
						continue
					}
					return &ent.IndexConflictErr{
						Underlying:  ent.ErrUniqueConflict,
						EntTypeName: entType,
						IndexName:   ed.Index.Name,
					}
				}
			}
			valdata := ent.IdSet(ed.Value).Encode()
			debugTrace("index put %q => %v (%q)", key, ed.Value, valdata)
			m.Put(key, valdata)
		}
	}
	return nil
}

func (s *EntStorage) FindByIndex(
	entTypeName string, x *ent.EntIndex, key []byte, limit int, flags ent.LookupFlags,
) ([]uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids, err := s.indexGet(entTypeName, x.Name, string(key))
	limitIds(&ids, limit, (flags&ent.Reverse) != 0)
	return ids, err
}

func (s *EntStorage) LoadByIndex(
	e Ent, x *ent.EntIndex, key []byte, limit int, flags ent.LookupFlags,
) ([]Ent, error) {
	//
	// TODO: document the following thing somewhere, maybe in ent.Storage:
	//
	// Important: the first ent in return value []Ent should be e.
	// Generated code relies on this to avoid unnecessary type checks.
	//
	s.mu.RLock()
	defer s.mu.RUnlock()
	entTypeName := e.EntTypeName()
	ids, err := s.indexGet(entTypeName, x.Name, string(key))
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		if x.IsUnique() {
			return nil, ent.ErrNotFound
		}
		return nil, nil
	}
	limitIds(&ids, limit, (flags&ent.Reverse) != 0)
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
		ent.SetEntBaseFieldsAfterLoad(e2, s, id, version)
		ents[i] = e2
	}
	return ents, nil
}

func (s *EntStorage) IterateIds(entType string) ent.IdIterator {
	it := &IdIterator{}
	it.init(s, entType)
	return it
}

func (s *EntStorage) IterateEnts(proto Ent) ent.EntIterator {
	it := &EntIterator{s: s, etype: reflect.TypeOf(proto).Elem()}
	it.init(s, proto.EntTypeName())
	return it
}

type IdIterator struct {
	ids []uint64
}
type EntIterator struct {
	IdIterator
	err   error
	s     *EntStorage
	etype reflect.Type
}

func (it *IdIterator) init(s *EntStorage, entType string) {
	keyPrefix := entType + ":"
	// this could be fancier... for now, KISS -- full scan of all ids into memory
	ids := make([]uint64, 0, 32)
	s.mu.RLock()
	for k, _ := range s.m.m {
		if strings.HasPrefix(k, keyPrefix) {
			id, _ := strconv.ParseUint(k[len(keyPrefix):], 10, 64)
			ids = append(ids, id)
		}
	}
	it.ids = ids
	s.mu.RUnlock()
}

func (it *IdIterator) Err() error { return nil }

func (it *IdIterator) Next(id *uint64) bool {
	if len(it.ids) == 0 {
		return false
	}
	*id = it.ids[len(it.ids)-1]
	it.ids = it.ids[:len(it.ids)-1]
	return true
}

func (it *EntIterator) Err() error { return it.err }

func (it *EntIterator) Next(e Ent) bool {
	et := reflect.TypeOf(e).Elem()
	if et != it.etype {
		if it.err == nil {
			it.err = fmt.Errorf("mixing ent types: iterator on %v but Next() got %v", it.etype, et)
		}
		return false
	}
	for it.err == nil {
		var id uint64
		if !it.IdIterator.Next(&id) {
			return false
		}
		version, err := it.s.LoadById(e, id)
		if err == nil {
			ent.SetEntBaseFieldsAfterLoad(e, it.s, id, version)
			break
		}
		if err != ent.ErrNotFound {
			it.err = err
			return false
		}
		// if not found, just keep going
	}
	return true
}

// -------

func limitIds(ids *[]uint64, limit int, reverse bool) {
	if reverse {
		v := *ids
		for i, j := 0, len(v)-1; i < j; i, j = i+1, j-1 {
			v[i], v[j] = v[j], v[i]
		}
	}
	if limit > 0 && limit < len(*ids) {
		*ids = (*ids)[:limit]
	}
}

func (s *EntStorage) indexGet(entTypeName, indexName, key string) ([]uint64, error) {
	indexKey := s.indexKey(entTypeName, indexName, key)
	value := s.m.Get(indexKey)
	debugTrace("index get %q => %q", indexKey, value)
	if len(value) == 0 {
		return nil, nil
	}
	return ent.ParseIdSet(value), nil
}

func (s *EntStorage) indexKey(entTypeName, indexName, key string) string {
	return entTypeName + "#" + indexName + ":" + key
}

func (s *EntStorage) entKey(entTypeName string, id uint64) string {
	if id == 0 {
		panic("zero id")
	}
	return entTypeName + ":" + strconv.FormatUint(id, 36)
}
