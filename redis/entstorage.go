package redis

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/mediocregopher/radix/v3"
	"github.com/rsms/ent"
	"github.com/rsms/go-bits"
)

func debugTrace(format string, args ...interface{}) {
	// The Go compiler will strip all invocations of debugTrace when the function body is empty.
	// (un)comment the next line to toggle debug trace logging:
	//fmt.Printf("TRACE "+format+"\n", args...)
}

type Ent = ent.Ent

type EntStorage struct {
	r              *Redis
	backgroundJobs sync.WaitGroup
}

func NewEntStorage(r *Redis) *EntStorage {
	return &EntStorage{
		r: r,
	}
}

func (s *EntStorage) Close() error {
	s.backgroundJobs.Wait()
	return nil
}

func (s *EntStorage) goBackground(f func()) {
	s.backgroundJobs.Add(1)
	go func() {
		defer s.backgroundJobs.Done()
		f()
	}()
}

// LoadEntById is part of the ent.Storage interface, used by LoadTYPEById()
func (s *EntStorage) LoadEntById(e Ent, id uint64) (version uint64, err error) {
	err = s.r.doRead(s.makeEntLoadCmd(e, id, &version))
	return
}

func (s *EntStorage) makeEntLoadCmd(e Ent, id uint64, versionOut *uint64) *RCmd {
	return &RCmd{
		func(w *RIOWriter) error {
			// encode query
			key := makeEntKey(e.EntTypeName(), id)
			w.ArrayHeader(2)
			w.Str("HGETALL")
			w.Blob(key)
			return nil
		},
		func(r *RReader) error {
			_, version, err := decodeEnt(e, r) // yields ErrNotFound if not found
			ent.SetEntBaseFieldsAfterLoad(e, s, id, version)
			if versionOut != nil {
				*versionOut = version
			}
			return err
		},
	}
}

// FindEntIdsByIndex is part of the ent.Storage interface, used by FindTYPEByINDEX
func (s *EntStorage) FindEntIdsByIndex(
	entType string, x *ent.EntIndex, key []byte, limit int,
) (ids []uint64, err error) {
	indexKey := makeIndexKey(entType, x, key)
	debugTrace("FindEntIdsByIndex %s.%s %q indexKey=%q", entType, x.Name, key, indexKey)

	if x.IsUnique() {
		var id uint64
		cmd := makeGETEntIdCmd(indexKey, &id)
		if err := s.r.doRead(cmd); err != nil {
			return nil, err
		}
		if id == 0 {
			return nil, ent.ErrNotFound
		}
		ids = []uint64{id}
		return
	}

	// ZRANGEBYLEX "type#index" "[value\xfe" "(value\xff"
	cmd := makeZRangeEntIdsCmd(indexKey, key, limit)
	err = s.r.doRead(cmd)
	ids = cmd.Result
	return
}

// LoadEntsByIndex is part of the ent.Storage interface, used by LoadTYPEByINDEX
func (s *EntStorage) LoadEntsByIndex(
	e Ent, x *ent.EntIndex, key []byte, limit int,
) ([]Ent, error) {
	entType := e.EntTypeName()
	debugTrace("LoadEntsByIndex %s.%s %q", entType, x.Name, key)

	ids, err := s.FindEntIdsByIndex(entType, x, key, limit)
	if err != nil || len(ids) == 0 {
		// Note: for x.IsUnique(), FindEntIdsByIndex returns ErrNotFound in case nothing is found
		return nil, err
	}
	debugTrace("FindEntIdsByIndex => %v", ids)
	ents := make([]Ent, 0, len(ids))

	cmds := make([]radix.CmdAction, len(ids))
	for i, id := range ids {
		e2 := e
		if i > 0 {
			// must use e for one of the results; ent system depends on this behavior
			e2 = e.EntNew()
		}
		ents = append(ents, e2)
		cmds[i] = s.makeEntLoadCmd(e2, id, nil)
	}

	if err = s.r.doRead(radix.Pipeline(cmds...)); err != nil {
		err2 := errors.Unwrap(err)
		if err2 == ent.ErrNotFound {
			err = err2
		}
	}

	return ents, err
}

// SaveEnt is part of the ent.Storage interface, used by TYPE.Save()
func (s *EntStorage) SaveEnt(e Ent, fieldmap uint64) (nextVersion uint64, err error) {
	prevVersion := e.Version()
	nextVersion = prevVersion + 1
	err = s.putEnt(e, e.Id(), prevVersion, nextVersion, fieldmap)
	return
}

// CreateEnt is part of the ent.Storage interface, used by TYPE.Create()
func (s *EntStorage) CreateEnt(e ent.Ent, fieldmap uint64) (id uint64, err error) {
	id = e.Id()
	if id == 0 {
		// generate new ent id
		// note: HINCRBY never yields 0, so we can use 0 to signify "no id"
		if err = s.r.doWrite(radix.FlatCmd(&id, "HINCRBY", "entid", e.EntTypeName(), 1)); err != nil {
			return
		}
	}
	return id, s.putEnt(e, id, 0, 1, fieldmap)
}

func (s *EntStorage) putEnt(e ent.Ent, id, prevVersion, nextVersion, fieldmap uint64) error {
	entType := e.EntTypeName()
	entKey := makeEntKey(entType, id)

	debugTrace("putEnt %q key=%q fieldmap=%b (version %d -> %d)",
		entType, entKey, fieldmap, prevVersion, nextVersion)

	// HSET fields
	// respWriter := RWriter{buf: make([]byte, 0, 128)}
	respData, err := encodeEntHSET(e, make([]byte, 0, 128), entKey, nextVersion, fieldmap)
	if err != nil {
		return err
	}

	// cmds holds all "write" commands, to be run inside a MULTI (pipelined)
	cmds := make([]radix.CmdAction, 2, 16) // commands to perform in MULTI
	cmds[0] = &CmdMULTI
	cmds[1] = &RawCmd{respData}

	// // addCmd appends cmd to cmds
	// addCmd := func(cmd *RawCmd) {
	// 	cmds = append(cmds, cmd)
	// }

	// watchKeys contains all keys watched
	watchKeys := make([][]byte, 1, 16)
	watchKeys[0] = entKey

	// begin redis communication
	err = s.r.Batch(func(c radix.Conn) (err error) {
		ok := false

		// WATCH the ent entry key for changes by other clients (e.g. "typename:id")
		debugTrace(">> WATCH %s", entKey)
		if err = c.Do(makeSingleKeyCmd("WATCH", entKey)); err != nil {
			return
		}

		// UNWATCH in case of error
		defer func() {
			if !ok {
				// Note: EXEC implicitly UNWATCH'es
				debugTrace(">> UNWATCH")
				c.Do(&CmdUNWATCH)
			}
		}()

		// In case we are performing an update (e.g. SaveEnt) load current version of the ent
		var currEnt ent.Ent
		if prevVersion != 0 {
			currEnt = e.EntNew()
			currVersion, err := s.loadEntPartial(c, currEnt, entKey, fieldmap)
			debugTrace("loadEntPartial %q => version=%v %+v", entKey, currVersion, currEnt)
			if err != nil {
				return err
			} else if currVersion == 0 {
				// Ent has been deleted since the receiver was loaded.
				// Caller should either call Create() to re-create the ent or abort the Save operation.
				return ent.ErrNotFound
			} else if prevVersion != currVersion {
				// ent has changed since the receiver was loaded.
				// The caller should Reload() and retry Save() (or Load() & merge.)
				return ent.ErrVersionConflict
			}
		}

		// update indexes
		err = s.computeIndexEdits(e, currEnt, id, fieldmap, &cmds, &watchKeys,
			func(key []byte, cmd radix.CmdAction) error {
				// Perform command right now. We watch the key since
				debugTrace(">> WATCH %s; %+v", key, cmd)
				return c.Do(radix.Pipeline(makeSingleKeyCmd("WATCH", key), cmd))
			})

		// prepend watch to cmds, skipping the first that we issued separately
		if len(watchKeys) > 1 {
			cmds2 := make([]radix.CmdAction, len(cmds)+1, len(cmds)+2) // extra space for WATCH and EXEC
			cmds2[0] = makeBulkStringCmd("WATCH", watchKeys...)
			for i := 1; i < len(cmds); i++ {
				for i, cmd := range cmds {
					cmds2[i+1] = cmd
				}
			}
			cmds = cmds2
		}

		// finally, append EXEC to cmds
		cmds = append(cmds, &CmdEXEC)

		// Perform cmds pipelined, meaning all commands are sent in one go, then all responses are
		// read in one go, instead of write,read,write,read...
		// First cmd is MULTI.
		debugTrace(">> %s", strings.ReplaceAll(fmt.Sprintf("%+v", cmds), "RawCmd[", "\n  RawCmd["))
		err = c.Do(radix.Pipeline(cmds...))
		ok = err == nil
		return
	}) // s.r.Batch

	if err != nil {
		// Note: In case an index changes from unique to non-unique, or vice versa, we get this error:
		//   "WRONGTYPE Operation against a key holding the wrong kind of value"
		// E.g. the "enttype:indexname" is either a Z or a H which are incompatible.
		// TODO: consider either migration code for this case, or at least detect it and produce a
		// better error message.
		return err
	}

	// If we use a separate reader redis server, write-through by applying the cmds locally.
	// This ensures immediate consistency, for example if the caller tries to load the ent
	// immediately after creating it.
	if s.r.RClient() != s.r.WClient() {
		s.goBackground(func() {
			// debugTrace("RClient >> %s",
			// 	strings.ReplaceAll(fmt.Sprintf("%+v", cmds), "RawCmd[", "\n  RawCmd["))
			err := s.r.RClient().Do(radix.Pipeline(cmds...))
			// debugTrace("RClient result => %v", err)
			// Fail with a warning; in case this fails the data is eventually consistent.
			// It is also possible that the replication won the race.
			if err != nil && s.r.Logger != nil {
				s.r.Logger.Warn("write-through cache failure %v", err)
			}
		})
	}

	return nil
}

// DeleteEnt is part of the ent.Storage interface, used by TYPE.PermanentlyDelete()
func (s *EntStorage) DeleteEnt(e ent.Ent) error {
	id := e.Id()
	entType := e.EntTypeName()
	if id == 0 {
		return fmt.Errorf("attempt to delete non-existing %s (id 0)", entType)
	}

	entKey := makeEntKey(entType, id)
	debugTrace("DeleteEnt #%d %s", e.Id(), entKey)

	// redis commands to be executed
	cmds := make([]radix.CmdAction, 3, 16)
	// cmds[0] = WATCH
	cmds[1] = &CmdMULTI
	cmds[2] = makeSingleKeyCmd("DEL", entKey)

	// indexes
	indexes := e.EntIndexes()
	if len(indexes) == 0 {
		cmds = cmds[2:]
	} else {
		watchKeys := make([][]byte, 1, 16)
		watchKeys[0] = entKey

		// compute index cleanup
		err := s.computeIndexEdits(nil, e, id, e.EntFields().Fieldmap, &cmds, &watchKeys, nil)
		if err != nil {
			return err
		}

		cmds[0] = makeBulkStringCmd("WATCH", watchKeys...)
		cmds = append(cmds, &CmdEXEC)
	}

	debugTrace(">> %s",
		strings.ReplaceAll(fmt.Sprintf("%+v", cmds), "RawCmd[", "\n  RawCmd["))

	err := s.r.WClient().Do(radix.Pipeline(cmds...))
	if err != nil {
		return err
	}

	if s.r.WClient() != s.r.RClient() {
		// update write-through cache
		s.goBackground(func() {
			// debugTrace("RClient >> %s",
			// 	strings.ReplaceAll(fmt.Sprintf("%+v", cmds), "RawCmd[", "\n  RawCmd["))
			err := s.r.RClient().Do(radix.Pipeline(cmds...))
			if err != nil && s.r.Logger != nil {
				s.r.Logger.Warn("write-through cache failure %v", err)
			}
		})
	}

	return nil
}

func (s *EntStorage) computeIndexEdits(
	nextEnt, prevEnt Ent,
	id, fieldmap uint64,
	cmdsPtr *[]radix.CmdAction,
	watchKeysPtr *[][]byte,
	doNow func(key []byte, cmd radix.CmdAction) error, // only needed when nextEnt!=nil
) error {
	indexEdits, err := ent.ComputeIndexEdits(nil, nextEnt, prevEnt, id, fieldmap)
	if err != nil {
		return err
	}

	cmds := *cmdsPtr
	watchKeys := *watchKeysPtr

	var entType string
	if nextEnt != nil {
		entType = nextEnt.EntTypeName()
	} else {
		entType = prevEnt.EntTypeName()
	}

	// debugTrace("indexEdits: %+v", indexEdits)
	for _, ed := range indexEdits {
		indexKey := makeIndexKey(entType, ed.Index, []byte(ed.Key))
		if ed.IsCleanup {
			if ed.Index.IsUnique() {
				// DEL "foo#email:robin@gmail.com"
				// Note: DEL returns an integer of the number of entries deleted (0 or 1) but we
				// don't care about that.
				cmds = append(cmds, makeSingleKeyCmd("DEL", indexKey))
			} else {
				// ZREM foo#email 0 "robin@gmail.com\xfe123"
				cmds = append(cmds, makeZREMIdCmd(indexKey, []byte(ed.Key), id))
			}
		} else {
			if ed.Index.IsUnique() {
				// WATCH unique index keys
				watchKeys = append(watchKeys, indexKey)
				var existingId uint64
				GETEntId := makeGETEntIdCmd(indexKey, &existingId)
				if err := doNow(indexKey, GETEntId); err != nil {
					return err
				}
				if existingId == 0 {
					cmds = append(cmds, makeSETNXIdCmd(indexKey, id))
				} else if existingId != id {
					// TODO: a semantic error; something the caller can easily identify as "index conflict"
					return fmt.Errorf("unique index conflict %s.%s with ent #%d",
						entType, ed.Index.Name, existingId)
				}
			} else {
				cmds = append(cmds, makeZADDIdCmd(indexKey, []byte(ed.Key), id))
			}
		}
	} // end of update index

	*cmdsPtr = cmds
	*watchKeysPtr = watchKeys
	return nil
}

// loadEntPartial
// Note: If an ent is not found, this returns version=0 (it does NOT return ent.ErrNotFound)
func (s *EntStorage) loadEntPartial(
	c radix.Conn, e Ent, entKey []byte, fieldmap uint64) (version uint64, err error) {
	// list of keys to fetch
	keys := make([]string, 1, bits.PopcountUint64(fieldmap)+1)
	keys[0] = ent.FieldNameVersion
	for fieldIndex, fieldName := range e.EntFields().Names {
		if (fieldmap & (1 << fieldIndex)) != 0 {
			keys = append(keys, fieldName)
		}
	}
	// communicate with redis
	err = c.Do(&RCmd{
		func(w *RIOWriter) error {
			// encode query
			w.ArrayHeader(len(keys) + 2)
			w.buf = respAppendBulkString(w.buf, []byte("HMGET"))
			w.buf = respAppendBulkString(w.buf, entKey)
			for _, k := range keys {
				w.buf = respAppendBulkString(w.buf, []byte(k))
			}
			return nil
		},
		func(r *RReader) error {
			// decode response
			n := r.ListHeader()
			if n < len(keys) {
				// This is not supposed to happen. Redis always returns N values for N keys from HMGET,
				// even when the entry does not exist (nil values.)
				return fmt.Errorf("unexpected response from redis")
			}
			c := ArrayEntDecoder{
				RReader: r,
				keys:    keys,
			}
			version = e.EntDecodePartial(&c, fieldmap)
			return nil
		},
	})
	return
}

// makeEntKey returns the canonical redis storage key for an ent
func makeEntKey(entTypeName string, id uint64) []byte {
	// Zero padded ID so that ents are ordered by creation time.
	// We could do something fancy here like base-62 encoding but this way, using hexadecimal
	// encoding, we make it easier for a human to construct the key. The length of base-62 is
	// just very slightly shorter for uint64 than hexadecimal (16 vs 11 bytes) so the win in
	// data would really have no meaningful effect.
	if id == 0 {
		panic("zero id")
	}
	var scratch [16]byte
	idstr := fmtint(scratch[:], id, 16)
	b := make([]byte, len(entTypeName)+1+len(idstr))
	i := copy(b, entTypeName)
	b[i] = ':'
	i++
	copy(b[i:], idstr)
	return b
	// return fmt.Sprintf("%s:%016x", entTypeName, id)
}

func makeIndexKey(entTypeName string, x *ent.EntIndex, entryKey []byte) []byte {
	z := len(entTypeName) + 1 + len(x.Name)
	if x.IsUnique() {
		z += 1 + len(entryKey)
	}
	b := make([]byte, z)
	i := copy(b, entTypeName)
	b[i] = '#'
	i++
	i += copy(b[i:], x.Name)
	if x.IsUnique() {
		b[i] = ':'
		i++
		copy(b[i:], entryKey)
	}
	return b
}

func makeGETEntIdCmd(key []byte, idOut *uint64) *RawCmdHexUint {
	return &RawCmdHexUint{
		RawCmd:    RawCmd{respMakeStringArray2("GET", key)},
		ResultPtr: idOut,
	}
	// return &RCmd{
	// 	func(w *RIOWriter) error {
	// 		w.ArrayHeader(2)
	// 		w.buf = respAppendBulkString(w.buf, []byte("GET"))
	// 		w.buf = respAppendBulkString(w.buf, key)
	// 		return nil
	// 	},
	// 	func(r *RReader) error {
	// 		*idOut = r.HexUint(64)
	// 		return nil
	// 	},
	// }
}

// encodeEntHSET writes a HSET command on w with all fields in fieldmap for e.
// If version is not zero, then the ent.FieldNameVersion field is written as well.
func encodeEntHSET(e Ent, buf, entKey []byte, version, fieldmap uint64) ([]byte, error) {
	nfields := bits.PopcountUint64(fieldmap)
	if version != 0 {
		nfields++
	}
	c := EntEncoder{buf: buf[:0]}
	c.BeginHSET(entKey, nfields)
	if version != 0 {
		c.Str(ent.FieldNameVersion)
		c.Uint(version, 64)
	}
	e.EntEncode(&c, fieldmap)
	respData := c.Buffer()
	if c.err == nil {
		debugTrace("encodeEntHSET %q", respData)
		// _, c.err = w.w.Write(respData)
	}
	return respData, c.err
}

// decodeEnt reads the result of a HGETALL command, populating e, id and version
func decodeEnt(e Ent, r *RReader) (id, version uint64, err error) {
	// decode result
	n := r.ListHeader()
	if n <= 0 {
		// HGETALL returns an empty list in case there's no key
		return 0, 0, ent.ErrNotFound
	}

	// if we did not get an even number of results (key,value, ...), then discard
	if n%2 != 0 {
		for i := 0; i < n; i++ {
			r.Discard()
		}
		// HGETALL should return a list of key-value tuples
		return 0, 0, fmt.Errorf("redis error (hgetall n%%2!=0; n=%d)", n)
	}

	// before we continue reading, check the reader for errors
	if r.Err() != nil {
		return 0, 0, r.Err()
	}

	// decode ent
	c := DictEntDecoder{
		RReader: r,
		nfields: n / 2,
	}
	id, version = e.EntDecode(&c)
	return
}

// ————————————————————————————————————————————————————————————————————————————————————————————

// EntEncoder is an implementation of ent.Encoder
// Since we store ents in Redis hashes (HSET, HGET, et al) values must all be strings, which
// is why this is not really using RWriter.
type EntEncoder struct {
	buf []byte
	err error
}

var hsetCmdSlice = []byte("HSET")

func (c *EntEncoder) BeginHSET(key []byte, nfields int) {
	bufgrow(&c.buf, ((1+intBase10MaxLen+2)*3)+len(hsetCmdSlice)+len(key))
	// note: keys and values in HSET must not be simple strings
	buf := respAppendArrayHeader(c.buf, 2+nfields*2)
	buf = respAppendBulkString(buf, hsetCmdSlice)
	c.buf = respAppendBulkString(buf, key)
}

func (c *EntEncoder) Buffer() []byte { return c.buf }

func (c *EntEncoder) Err() error    { return c.err }
func (c *EntEncoder) Key(k string)  { c.Str(k) }
func (c *EntEncoder) Str(v string)  { c.buf = respAppendBulkString(c.buf, []byte(v)) }
func (c *EntEncoder) Blob(v []byte) { c.buf = respAppendBulkString(c.buf, v) }

const (
	respBoolBulkStrTrue  = "$1\r\n1\r\n"
	respBoolBulkStrFalse = "$1\r\n0\r\n"
)

func (c *EntEncoder) Bool(v bool) {
	bufgrow(&c.buf, 7)
	if v {
		c.buf = append(c.buf, respBoolBulkStrTrue...)
	} else {
		c.buf = append(c.buf, respBoolBulkStrFalse...)
	}
}

func (c *EntEncoder) Int(v int64, bitsize int) {
	var tmp [intBase10MaxLen]byte
	b := strconv.AppendInt(tmp[:0], v, 10)
	c.buf = respAppendBulkString(c.buf, b)
}

func (c *EntEncoder) Uint(v uint64, bitsize int) {
	var tmp [intBase10MaxLen]byte
	b := strconv.AppendUint(tmp[:0], v, 10)
	c.buf = respAppendBulkString(c.buf, b)
}

func (c *EntEncoder) Float(v float64, bitsize int) {
	var tmp [128]byte
	b := appendFloat(tmp[:0], v, bitsize)
	c.buf = respAppendBulkString(c.buf, b)
}

func (c *EntEncoder) BeginEnt(version uint64) {} // unused
func (c *EntEncoder) EndEnt()                 {} // unused

func (c *EntEncoder) BeginList(length int) {
	if c.err == nil {
		c.err = fmt.Errorf("nested lists are not yet supported")
	}
}
func (c *EntEncoder) EndList() {} // unused

func (c *EntEncoder) BeginDict(length int) {
	if c.err == nil {
		c.err = fmt.Errorf("nested dicts are not yet supported")
	}
}
func (c *EntEncoder) EndDict() {} // unused

// ————————————————————————————————————————————————————————————————————————————————————————————

// DictEntDecoder is an implementation of ent.Decoder which reads keys and values interleaved.
// E.g. "key1" "value1" "key2" "value2" ...
type DictEntDecoder struct {
	*RReader
	nfields int // number of fields to read (counts down)
}

func (r *DictEntDecoder) More() bool { return false } // unused
func (r *DictEntDecoder) Key() string {
	// an ent.Decoder returns the empty string when it is done
	if r.nfields == 0 {
		// all fields have been read
		return ""
	}
	r.nfields--
	return r.Str()
}

// ———————————————————————————————————————————————————————

// ArrayEntDecoder is an implementation of ent.Decoder that decodes values in a known order.
// E.g. with keys=["key1", "key2", "key3"], reads "value1" "value2" "value3".
type ArrayEntDecoder struct {
	*RReader
	keys []string // keys, in predetermined order that matches values to be decoded
}

func (r *ArrayEntDecoder) More() bool { return false } // unused
func (r *ArrayEntDecoder) Key() string {
	// an ent.Decoder returns the empty string when it is done
	if len(r.keys) == 0 {
		// all fields have been read
		return ""
	}
	key := r.keys[0]
	r.keys = r.keys[1:]
	return key
}
