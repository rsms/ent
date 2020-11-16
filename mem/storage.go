package mem

import (
	"fmt"
	"strconv"
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

func (s *EntStorage) CreateEnt(e Ent, fieldmap uint64) (id uint64, err error) {
	id = atomic.AddUint64(&s.idgen, 1)
	err = s.putEnt(e, id, 1, fieldmap)
	return
}

func (s *EntStorage) SaveEnt(e Ent, fieldmap uint64) (nextVersion uint64, err error) {
	nextVersion = e.Version() + 1
	err = s.putEnt(e, e.Id(), nextVersion, fieldmap)
	return
}

func (s *EntStorage) LoadEntById(e Ent, id uint64) (version uint64, err error) {
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

func (s *EntStorage) DeleteEnt(e Ent) error {
	key := s.entKey(e.EntTypeName(), e.Id())
	s.mu.Lock()
	ok := s.m.Get(key) != nil
	s.m.Del(key)
	s.mu.Unlock()
	if !ok {
		return ent.ErrNotFound
	}
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
			currentVersion, err := ent.JsonDecodeEntPartial(prevEnt, prevData, changedFields)
			if err != nil {
				return err
			}
			if expectVersion != currentVersion {
				return ent.ErrVersionConflict
			}
		}
	}

	// update indexes
	indexEdits, err := ent.ComputeIndexEdits(s.indexGet, e, prevEnt, id, changedFields)
	if err != nil {
		return err
	}
	debugTrace("indexEdits %+v\n", indexEdits)
	entTypeName := e.EntTypeName()
	for _, ed := range indexEdits {
		key := s.indexKey(entTypeName, ed.Index.Name, ed.Key)
		if len(ed.Value) == 0 {
			debugTrace("index del %q", key)
			m.Del(key)
		} else {
			debugTrace("index put %q => %v", key, ed.Value)
			if !ed.IsCleanup && ed.Index.IsUnique() {
				// check for collision
				ids, err := s.indexGet(entTypeName, ed.Index.Name, string(ed.Key))
				if err != nil {
					return err
				}
				if len(ids) > 0 {
					if ids[0] == id {
						continue
					}
					// TODO: a semantic error; something the caller can easily identify as "index conflict"
					return fmt.Errorf("unique index conflict %s.%s with ent #%d",
						entTypeName, ed.Index.Name, ids[0])
				}
			}
			m.Put(key, ent.IdSet(ed.Value).Encode())
		}
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

func (s *EntStorage) FindEntIdsByIndex(
	entTypeName string, x *ent.EntIndex, key []byte, limit int,
) ([]uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids, err := s.indexGet(entTypeName, x.Name, string(key))
	limitIds(&ids, limit)
	return ids, err
}

func (s *EntStorage) LoadEntsByIndex(
	e Ent, x *ent.EntIndex, key []byte, limit int,
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
	limitIds(&ids, limit)
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

func limitIds(ids *[]uint64, limit int) {
	if limit > 0 && limit < len(*ids) {
		*ids = (*ids)[:limit]
	}
}

func (s *EntStorage) indexGet(entTypeName, indexName, key string) ([]uint64, error) {
	value := s.m.Get(s.indexKey(entTypeName, indexName, key))
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
