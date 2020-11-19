package redis

import (
	"bufio"
	"fmt"
	"reflect"

	"github.com/mediocregopher/radix/v3"
	"github.com/rsms/ent"
)

type IdIterator struct {
	RawCmd
	r       *Redis
	cursor  []byte   // nil when done
	match   []byte   // entTypeName ":" "*"
	idbuf   []uint64 // read, buffered ids to be iterated over next
	readbuf []byte
	err     error
}

func MakeEntIterator(e Ent, s *EntStorage) *EntIterator {
	it := &EntIterator{s: s, etype: reflect.TypeOf(e).Elem()}
	it.init(e.EntTypeName(), s.Redis)
	return it
}

func MakeIdIterator(entType string, r *Redis) *IdIterator {
	it := &IdIterator{}
	it.init(entType, r)
	return it
}

func (it *IdIterator) init(entType string, r *Redis) {
	it.r = r
	it.cursor = make([]byte, 1, 20)
	it.cursor[0] = '0'
	it.match = append([]byte(entType), entKeySep, '*')
	it.idbuf = make([]uint64, 0, 32)
	it.readbuf = make([]byte, len(it.match)+15)
}

func (it *IdIterator) setErr(err error) {
	if it.err == nil {
		it.err = err
	}
}

func (it *IdIterator) Err() error { return it.err }

func (it *IdIterator) Next(id *uint64) bool {
	if len(it.idbuf) == 0 {
		if it.cursor != nil {
			it.fetchMore()
			if len(it.idbuf) == 0 {
				return false
			}
		} else {
			return false
		}
	}
	l := len(it.idbuf) - 1
	*id = it.idbuf[l]
	it.idbuf = it.idbuf[:l]
	return true
}

func (it *IdIterator) fetchMore() {
	for it.err == nil {
		// SCAN cursor MATCH pattern
		it.RawCmd.Data = respMakeStringArray("SCAN", it.cursor, []byte("MATCH"), it.match)
		if err := it.r.doRead(it); err != nil {
			it.setErr(err)
			it.cursor = nil
			it.idbuf = nil
			break
		}
		if it.cursor == nil || len(it.idbuf) > 0 {
			// done or got results
			break
		}
		// fetch more
	}
}

func (it *IdIterator) UnmarshalRESP(rs *bufio.Reader) error {
	// reply from SCAN:
	// 1. cursor string
	// 2. keys   array<string>
	//
	r := RReader{r: rs, buf: make([]byte, 0, 256)}
	n := r.ListHeader()
	if n != 2 {
		if r.Type() == RESPTypeArray {
			for i := 0; i < n; i++ {
				r.Discard()
			}
			return fmt.Errorf("bad response from SCAN (n=%d) %v", n, r.Err())
		} else {
			return fmt.Errorf("bad response from SCAN (t=%q) %v", r.Type(), r.Err())
		}
	}

	it.cursor = r.Blob()
	n = r.ListHeader()

	if cap(it.idbuf) < n {
		it.idbuf = make([]uint64, n)
	} else {
		it.idbuf = it.idbuf[:n]
	}

	for i := 0; i < n; i++ {
		// each ent key is of the form "typename:XXXXXXXXXXXXXXXX" (XX = hex byte)
		b := r.AnyData(it.readbuf)
		// fmt.Printf(">> read %q -> %q\n", b, b[len(it.match)-1:])
		id, err := parseHexUint(b[len(it.match)-1:])
		if err != nil {
			i++
			for ; i < n; i++ {
				r.Discard()
			}
			return err
		}
		it.idbuf[i] = id
	}

	if len(it.cursor) == 1 && it.cursor[0] == '0' {
		// DONE
		it.cursor = nil
	}

	return r.Err()
}

func (it *IdIterator) Run(conn radix.Conn) error {
	if err := conn.Encode(it); err != nil {
		return err
	}
	return conn.Decode(it)
}

// ------
type EntIterator struct {
	IdIterator
	s     *EntStorage
	etype reflect.Type
}

func (it *EntIterator) Next(e ent.Ent) bool {
	et := reflect.TypeOf(e).Elem()
	if et != it.etype {
		it.setErr(fmt.Errorf("mixing ent types: iterator on %v but Next() got %v", it.etype, et))
		return false
	}
	for it.err == nil {
		var id uint64
		if !it.IdIterator.Next(&id) || it.err != nil {
			return false
		}
		version, err := it.s.LoadById(e, id)
		if err == nil {
			ent.SetEntBaseFieldsAfterLoad(e, it.s, id, version)
			break
		}
		if err != ent.ErrNotFound {
			it.setErr(err)
			return false
		}
		// if not found, just keep going
	}
	return true
}
