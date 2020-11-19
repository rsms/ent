package redis

import (
	"bufio"
	"errors"
	"strconv"
)

type RESPType = byte

const (
	RESPTypeSimpleString = RESPType('+') // simple string
	RESPTypeError        = RESPType('-') // error
	RESPTypeInteger      = RESPType(':') // integer
	RESPTypeBulkString   = RESPType('$') // bulk string
	RESPTypeArray        = RESPType('*') // array header
)

type RReader struct {
	r   *bufio.Reader
	err error
	buf []byte
	typ RESPType // last read type
}

var ErrRespUnexpectedType = errors.New("unexpected resp type")

// Err returns the error state of the reader
func (r *RReader) Err() error { return r.err }

// Type returns the RESP type of the most recently read value
func (r *RReader) Type() RESPType { return r.typ }

func (r *RReader) SetErr(err error) {
	if r.err == nil {
		r.err = err
	}
}

// ListHeader reads an array header, returning the number of elements that follows.
// Returns -1 to signal "nil array" and 0 signals "empty array" since the RESP protocol
// makes that distinction (though Go does not.)
func (r *RReader) ListHeader() int {
	if r.err == nil {
		b := r.readNext(nil)
		if r.typ == RESPTypeArray {
			var i int64
			if r.err == nil {
				i, r.err = parseInt(b)
			}
			return int(i)
		} else if r.err == nil {
			r.err = ErrRespUnexpectedType
		}
	}
	return -1
}

func (r *RReader) DictHeader() int {
	// TODO add support for reading embedded dicts
	r.SetErr(errors.New("dicts not supported"))
	return 0
}

// BytesArray reads an array of raw byte arrays
func (r *RReader) BytesArray() [][]byte {
	n := r.ListHeader()
	if n < 1 {
		return nil
	}
	a := make([][]byte, n)
	for i := 0; i < n; i++ {
		a[i] = r.Blob()
	}
	return a
}

// StrArray reads an array of strings
func (r *RReader) StrArray() []string {
	n := r.ListHeader()
	if n < 1 {
		return nil
	}
	a := make([]string, n)
	for i := 0; i < n; i++ {
		a[i] = r.Str()
	}
	return a
}

// IntArray reads an array of integers
func (r *RReader) IntArray(bitsize int) []int64 {
	n := r.ListHeader()
	if n < 1 {
		return nil
	}
	a := make([]int64, n)
	for i := 0; i < n; i++ {
		a[i] = r.Int(bitsize)
	}
	return a
}

// Bool reads a boolean value
func (r *RReader) Bool() bool {
	// true  = "$1\r\n1\r\n" OR ":1\r\n" OR "+1\r\n"
	// false = "$1\r\n0\r\n" OR ":0\r\n" OR "+0\r\n"
	if r.err != nil {
		return false
	}
	b := r.readNextDiscardArray(nil)
	return len(b) > 0 && b[0] != '0'
}

// Int reads a signed integer
func (r *RReader) Int(bitsize int) int64 {
	var i int64
	if r.err == nil {
		b := r.readNextDiscardArray(nil)
		if r.err == nil {
			i, r.err = parseInt(b)
		}
	}
	return i
}

// Uint reads an unsigned integer
func (r *RReader) Uint(bitsize int) uint64 {
	var u uint64
	if r.err == nil {
		b := r.readNextDiscardArray(nil)
		if r.err == nil {
			u, r.err = parseUint(b)
		}
	}
	return u
}

func (r *RReader) HexUint(bitsize int) uint64 {
	var u uint64
	if r.err == nil {
		b := r.readNextDiscardArray(nil)
		if r.err == nil {
			u, r.err = parseHexUint(b)
		}
	}
	return u
}

// Float reads a floating point number
func (r *RReader) Float(bitsize int) float64 {
	var f float64
	if r.err == nil {
		b := r.readNextDiscardArray(nil)
		if r.err == nil {
			f, r.err = strconv.ParseFloat(string(b), bitsize)
		}
	}
	return f
}

// Str reads the next message as a string.
// If the message read is not a RESP string type, the empty string is returned.
// To read the next message's content as a string regardless of its type, use `string(r.Bytes())`
func (r *RReader) Str() string {
	if r.err == nil {
		b := r.readNextDiscardArray(nil)
		if r.typ == RESPTypeSimpleString || r.typ == RESPTypeBulkString {
			return string(b)
		}
	}
	return ""
}

// Blob reads the next message uninterpreted.
func (r *RReader) Blob() []byte {
	return r.AnyData(nil)
}

// AnyData reads the next message uninterpreted.
// If buf is not nil, it is used for reading the data and a slice of it is returned. If buf is
// nil or its cap is less than needed, a new byte array is allocated.
func (r *RReader) AnyData(buf []byte) []byte {
	if r.err == nil {
		b := r.readNextDiscardArray(buf)
		return b
	}
	return nil
}

// Scalar reads any scalar value. Compound types like arrays are skipped & discarded.
func (r *RReader) Scalar() (typ RESPType, data []byte) {
	if r.err == nil {
		data = r.readNextDiscardArray(nil)
		typ = r.typ
	}
	return
}

// AppendBlob reads the next message (uninterpreted) and appends it to buf
func (r *RReader) AppendBlob(buf []byte) []byte {
	if r.err != nil {
		return nil
	}
	r.typ, _ = r.r.ReadByte()
	if r.typ == RESPTypeBulkString {
		// optimization for bulk strings
		z, err := readIntLine(r.r)
		if err != nil {
			r.SetErr(err)
			return nil
		}
		if z < 1 {
			return buf
		}
		readz := int(z)
		l := len(buf)
		if cap(buf)-l < readz {
			buf2 := make([]byte, l, cap(buf)+readz)
			copy(buf2, buf)
			buf = buf2
		}
		if !r.readBytes(buf[l : l+readz]) {
			readz = 0
		}
		return buf[:l+readz]
	}
	// other messages types that are not "bulk string"
	r.r.UnreadByte()
	b := r.readNextDiscardArray(nil)
	if b == nil {
		return buf
	}
	return append(buf, b...)
}

// Next is a low-level read function which reads whatever RESP message comes next, without
// and interpretation. Note that in the case typ is RESPTypeArray the caller is responsible for
// reading ParseInt(data) more messages (array elements) to uphold the read stream integrity.
// When typ==RESPTypeError, r.Err() is set to reflect the error message.
// buf is optional.
// If nil new buffers are allocated for the response (data), else buf is used for data if it's
// large enough.
func (r *RReader) Next(buf []byte) []byte {
	if r.err != nil {
		return nil
	}
	return r.readNext(buf)
}

// Discard reads & discards the next message, including entire arrays
func (r *RReader) Discard() {
	typ, _ := r.r.ReadByte()

	// future, _ := r.r.Peek(r.r.Buffered())
	// fmt.Printf("RReader discard data of type '%c' %q ...\n", typ, future)

	if typ == RESPTypeBulkString {
		z, err := readIntLine(r.r)
		if err != nil {
			r.err = err
		} else if z >= 0 {
			// Handle "$0\r\n\r\n" and "$3\r\nfoo\r\n" (but NOT "$-1\r\n")
			//               ~~~~             ~~~~~~~
			_, r.err = r.r.Discard(int(z) + 2)
		}
	} else {
		var data []byte
		data, r.err = respReadLine(r.r)
		if typ == RESPTypeError {
			r.err = errors.New("redis: " + string(data))
		} else if typ == RESPTypeArray {
			r.discardArrayElements(data)
		}
	}
	return
}

// readNext reads the next RESP message and returns its type byte along with its content.
// Assumes that r.err==nil
//
func (r *RReader) readNext(buf []byte) (data []byte) {
	r.typ, _ = r.r.ReadByte()
	if r.typ == RESPTypeBulkString {
		// bulk string, e.g. "$5\r\nhello\r\n" or "$-1\r\n" (nil)
		z, err := readIntLine(r.r)
		if err != nil {
			r.err = err
		} else if z < 1 {
			if z == 0 {
				// empty string, i.e. "$0\r\n\r\n"
				// discard the last \r\n
				_, r.err = r.r.Discard(2) // \r\n
			} // else: nil, i.e. "$-1\r\n"
		} else {
			if cap(buf) >= int(z) {
				data = buf[:z]
			} else {
				data = make([]byte, z)
			}
			if !r.readBytes(data) {
				data = nil
			}
		}
	} else {
		// All other message types
		//   Simple string, e.g. "+hello\r\n" OR
		//   Error message, e.g. "-CODE message\r\n" OR
		//   Integer,       e.g. ":123\r\n" OR
		//   Array header,  e.g. "*3\r\n$3foo\r\n$3bar\r\n$6lolcat\r\n"
		//                        ~~~~~~
		data, r.err = respReadLine(r.r)
		if r.typ == RESPTypeError {
			r.err = errors.New("redis: " + string(data))
		}
	}
	return
}

// readNextDiscardArray is like readNext but in the case of an array header, reads & discards
// all array items.
func (r *RReader) readNextDiscardArray(buf []byte) []byte {
	data := r.readNext(buf)
	if r.typ == RESPTypeArray {
		r.discardArrayElements(data)
	}
	return data
}

func (r *RReader) discardArrayElements(arrayHeader []byte) {
	z, err := parseInt(arrayHeader)
	if err != nil && r.err == nil {
		r.err = err
		return
	}
	for i := 0; i < int(z) && r.err == nil; i++ {
		r.Discard()
	}
}

func (r *RReader) readBytes(buf []byte) bool {
	n, err := r.r.Read(buf)
	if err != nil {
		r.err = err
		return false
	}
	if n < len(buf) {
		r.err = errors.New("i/o short read")
		return false
	}
	_, r.err = r.r.Discard(2) // \r\n
	return r.err == nil
}
