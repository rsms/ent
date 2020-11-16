package redis

import (
	"bufio"
	"fmt"
	"io"
	"time"

	"github.com/mediocregopher/radix/v3"
	"github.com/rsms/go-log"
)

type Redis struct {
	Logger *log.Logger

	rwc *radix.Pool // read-write redis server connection
	roc *radix.Pool // read-only redis server connection (if nil, use rwc for reads)
}

func (r *Redis) Open(rwaddr, roaddr string, connPoolSize int) error {
	if roaddr == "" {
		roaddr = rwaddr
	} else if rwaddr == "" {
		rwaddr = roaddr
	}

	// connect to read-write server (LEADER)
	rwc, err := radix.NewPool("tcp", rwaddr, connPoolSize)
	if err != nil {
		return err
	}

	// if a different address is provided for roc, connect to read-only server (FOLLOWER)
	var roc *radix.Pool
	if rwaddr != roaddr {
		roc, err = radix.NewPool("tcp", roaddr, connPoolSize)
		if err != nil {
			rwc.Close()
			return err
		}
	}

	if r.Logger != nil {
		if rwaddr != roaddr {
			r.Logger.Info("connected to rw=%s, ro=%s", rwaddr, roaddr)
		} else {
			r.Logger.Info("connected to %s", rwaddr)
		}
	}

	return r.SetConnections(rwc, roc)
}

// OpenRetry calls Open until it succeeds, with a second delay in between
func (r *Redis) OpenRetry(rwaddr, roaddr string, connPoolSize int) {
	for {
		err := r.Open(rwaddr, roaddr, connPoolSize)
		if err == nil {
			break
		}
		if r.Logger != nil {
			r.Logger.Warn("%s; retrying in 1s", err)
		}
		time.Sleep(time.Second)
	}
}

func (r *Redis) SetConnections(rwc, roc *radix.Pool) error {
	if r.rwc != nil {
		return fmt.Errorf("already connected")
	}
	r.rwc = rwc
	r.roc = roc

	if r.Logger != nil {
		// initialize logging for the connection(s)
		r.initErrLogging(rwc)
		if roc != nil {
			r.initErrLogging(roc)
		}
	}
	return nil
}

func (r *Redis) initErrLogging(c *radix.Pool) {
	c.ErrCh = make(chan error)
	go func(ch chan error, l *log.Logger) {
		for {
			// Note: ErrCh closes when c.Close() is called
			err, ok := <-c.ErrCh
			if !ok {
				break
			}
			l.Warn("recovered error %v (%v)", err, c)
		}
		l.Debug("closed connection (%v)", c)
	}(c.ErrCh, r.Logger)
}

func (r *Redis) Close() error {
	err := r.rwc.Close()
	if r.roc != nil {
		if err2 := r.roc.Close(); err2 != nil && err == nil {
			err = err2
		}
	}
	r.rwc = nil
	r.roc = nil
	return err
}

// RClient returns a redis connection for reading
func (r *Redis) RClient() *radix.Pool {
	if r.roc != nil {
		return r.roc
	}
	return r.rwc
}

// WClient returns a redis connection for writing (can also read)
func (r *Redis) WClient() *radix.Pool {
	return r.rwc
}

// doRead runs action a on the most suitable redis server for reading.
// "a" should NOT be a mutating action -- if it is, the modification may get lost from
// data replication effects later on.
func (r *Redis) doRead(a radix.Action) error {
	c := r.rwc
	if r.roc != nil {
		c = r.roc
	}
	return c.Do(a)
}

// doReadImportant reads data from the leader redis server.
// This is much slower than doRead on follower servers but
// is always consistent following a doWrite call.
func (r *Redis) doReadImportant(a radix.Action) error {
	return r.rwc.Do(a)
}

// doWrite runs action a on the read-write redis server
func (r *Redis) doWrite(a radix.Action) error {
	return r.rwc.Do(a)
}

// doWriteIdempotent runs action a on the read-write redis server AND the read-only server.
// Only idempotent actions like "SET" can use this.
func (r *Redis) doWriteIdempotent(a radix.Action) error {
	err := r.rwc.Do(a)
	if err == nil && r.roc != nil {
		// write-through cache
		if err := r.roc.Do(a); err != nil && r.Logger != nil {
			// failure only in local cache; log but don't return the error
			r.Logger.Warn("write-through cache failure %v (likely harmless)", err)
		}
	}
	return err
}

func (r *Redis) GetBytes(key string) (value []byte, err error) {
	err = r.doRead(radix.Cmd(&value, "GET", key))
	return
}

func (r *Redis) HGet(key, field string, value_out interface{}) error {
	return r.doRead(radix.FlatCmd(value_out, "HGET", key, field))
}

// func (r *Redis) GetAny(key string, value_out interface{}) error {
//  return r.doRead(radix.Cmd(value_out, "GET", key))
// }

func (r *Redis) Set(key string, value interface{}) error {
	return r.doWriteIdempotent(radix.FlatCmd(nil, "SET", key, value))
}

func (r *Redis) SetExpiring(key string, ttl time.Duration, value interface{}) error {
	ttlf := float64(ttl) / float64(time.Second) // convert time.Duration to seconds
	return r.doWriteIdempotent(radix.FlatCmd(nil, "SETEX", key, ttlf, value))
}

func (r *Redis) Del(key string) error {
	return r.doWriteIdempotent(radix.Cmd(nil, "DEL", key))
}

func (r *Redis) UpdateExpire(key string, ttl time.Duration) error {
	ttlf := float64(ttl) / float64(time.Second) // convert time.Duration to seconds
	return r.doWriteIdempotent(radix.FlatCmd(nil, "EXPIRE", key, ttlf))
}

func (r *Redis) GetString(key string) (value string, err error) {
	err = r.doRead(radix.Cmd(&value, "GET", key))
	return
}

func (r *Redis) Batch(f func(c radix.Conn) error) error {
	// https://godoc.org/github.com/mediocregopher/radix#WithConn
	// Note: first arg is key which is only used for redis cluster
	return r.rwc.Do(radix.WithConn("", f))
}

func (r *Redis) BatchOnRClient(f func(c radix.Conn) error) error {
	return r.roc.Do(radix.WithConn("", f))
}

// constant commands without results
var (
	CmdDISCARD = RawCmd{[]byte("*1\r\n$7\r\nDISCARD\r\n")}
	CmdEXEC    = RawCmd{[]byte("*1\r\n$4\r\nEXEC\r\n")}
	CmdMULTI   = RawCmd{[]byte("*1\r\n$5\r\nMULTI\r\n")}
	CmdUNWATCH = RawCmd{[]byte("*1\r\n$7\r\nUNWATCH\r\n")}
)

// —————————————————————————————————————————————————————————————————————————————————————————————

// RawCmd sends verbatim bytes over a redis connection and discards any replies
type RawCmd struct { // conforms to radix.Marshaler
	Data []byte // never mutated
}

func (c *RawCmd) Keys() []string { return []string{} }

func (c *RawCmd) Run(conn radix.Conn) error {
	if err := conn.Encode(c); err != nil {
		return err
	}
	return conn.Decode(c)
}

func (c *RawCmd) MarshalRESP(w io.Writer) error {
	_, err := w.Write(c.Data)
	return err
}

func (c *RawCmd) UnmarshalRESP(r *bufio.Reader) error {
	var buf [32]byte
	reader := RReader{r: r, buf: buf[:]}
	reader.Discard()
	return reader.Err()
}

func (c *RawCmd) String() string {
	// return fmt.Sprintf("RawCmd{%q}", c.Data)
	return fmt.Sprintf("RawCmd%q", splitRESPChunks(c.Data))
}

type RawCmdHexUint struct {
	RawCmd
	ResultPtr *uint64
}

func (c *RawCmdHexUint) Run(conn radix.Conn) error {
	if err := conn.Encode(c); err != nil {
		return err
	}
	return conn.Decode(c)
}

func (c *RawCmdHexUint) UnmarshalRESP(r *bufio.Reader) error {
	reader := RReader{r: r, buf: make([]byte, 0, 16)}
	*c.ResultPtr = reader.HexUint(64)
	return reader.Err()
}

type ZRangeEntIdsCmd struct {
	RawCmd
	Result    []uint64
	prefixLen int
}

func makeZRangeEntIdsCmd(key, lookupKey []byte, limit int) *ZRangeEntIdsCmd {
	buf := make([]byte, len(lookupKey)*2+4)

	rangeStart := buf[:len(lookupKey)+2]
	rangeStart[0] = '['
	rangeStart[1+copy(rangeStart[1:], lookupKey)] = '\xfe'

	rangeEnd := buf[len(lookupKey)+2:]
	rangeEnd[0] = '('
	rangeEnd[1+copy(rangeEnd[1:], lookupKey)] = '\xff'

	argsa := [6][]byte{key, rangeStart, rangeEnd}
	args := argsa[:3]
	if limit > 0 {
		// add "LIMIT 0 limit"
		argsa[3] = []byte("LIMIT")
		argsa[4] = []byte("0")
		var scratch [intBase10MaxLen]byte
		argsa[5] = fmtint(scratch[:], uint64(limit), 10)
		args = argsa[:]
	}

	respData := respMakeStringArray("ZRANGEBYLEX", args...)
	return &ZRangeEntIdsCmd{RawCmd: RawCmd{respData}, prefixLen: len(rangeStart)}
}

func (c *ZRangeEntIdsCmd) Run(conn radix.Conn) error {
	if err := conn.Encode(c); err != nil {
		return err
	}
	return conn.Decode(c)
}

func (c *ZRangeEntIdsCmd) UnmarshalRESP(r *bufio.Reader) error {
	reader := RReader{r: r, buf: make([]byte, 0, 256)}
	n := reader.ListHeader()
	if n > 0 {
		c.Result = make([]uint64, int(n))
		// each entry is of the form "PREFIX\xfeIDIDIDID"
		// where IDIDIDID is the id in big-endian byte order.
		readbuf := make([]byte, c.prefixLen+8)
		for i := 0; i < n; i++ {
			b := reader.AnyData(readbuf)
			u := readUint64BE(b[c.prefixLen-1:])
			c.Result[i] = u
		}
	}
	return reader.Err()
}

func makeSingleKeyCmd(cmd string, key []byte) *RawCmd {
	return &RawCmd{respMakeStringArray2(cmd, key)}
}

func makeBulkStringCmd(cmd string, args ...[]byte) *RawCmd {
	return &RawCmd{respMakeStringArray(cmd, args...)}
}

// makeZADDId creates a RawCmd{ ZADD "foo#email" 0 "alan@bob.com\xfeIDIDIDID" }
// where IDIDIDID is the id in big-endian byte order.
func makeZADDIdCmd(indexKey, valueKey []byte, id uint64) *RawCmd {
	rangeKey := make([]byte, len(valueKey)+9)
	copy(rangeKey, valueKey)
	rangeKey[len(valueKey)] = '\xfe'
	writeUint64BE(rangeKey[len(valueKey)+1:], id)

	buf := respMakeStringArray("ZADD", indexKey, []byte{'0'}, rangeKey)
	return &RawCmd{buf}
}

func makeZREMIdCmd(indexKey, valueKey []byte, id uint64) *RawCmd {
	var scratch [16]byte
	idstr := fmtint(scratch[:], id, 16)
	return &RawCmd{[]byte(fmt.Sprintf(
		"*4\r\n"+
			"$4\r\nZREM\r\n"+
			"$%d\r\n%s\r\n"+
			"$1\r\n0\r\n"+
			"$%d\r\n%s\xfe%s\r\n",
		len(indexKey), indexKey,
		len(valueKey)+1+len(idstr), valueKey, idstr))}
}

func makeSETNXIdCmd(key []byte, id uint64) *RawCmd {
	var scratch [16]byte
	idstr := fmtint(scratch[:], id, 16)
	return &RawCmd{[]byte(fmt.Sprintf(
		"*3\r\n"+
			"$5\r\nSETNX\r\n"+
			"$%d\r\n%s\r\n"+
			"$%d\r\n%s\r\n",
		len(key), key,
		len(idstr), idstr))}
}
