package redis

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
)

// uint64 max "18446744073709551615"
// int64 min  "-9223372036854775808"
const intBase10MaxLen = 20

// reads ARRAY_LEN (BYTESTR){ARRAY_LEN}
// e.g. "*3\r\n$3foo\r\n$3bar\r\n$6lolcat\r\n" => ["foo", "bar", "lolcat"]
// If buf is not nil, data is stored in buf instead of individually-allocated arrays.
func respReadBlobArray(r *bufio.Reader, buf []byte) ([][]byte, error) {
	alen, err := respReadLengthMsg('*', r)
	if err != nil {
		return nil, err
	}
	if alen < 1 { // nil (-1) or empty (0) array
		return nil, nil
	}
	blobs := make([][]byte, alen)
	for i := 0; i < alen && err == nil; i++ {
		var bloblen, n int
		if bloblen, err = respReadLengthMsg('$', r); err == nil {
			if buf != nil {
				bufgrow(&buf, bloblen)
				blobs[i] = buf[len(buf) : len(buf)+bloblen]
				buf = buf[:len(buf)+bloblen]
			} else {
				blobs[i] = make([]byte, bloblen)
			}
			n, err = r.Read(blobs[i])
			if err == nil && n < bloblen {
				err = errors.New("i/o short read")
			} else {
				r.Discard(2) // \r\n
			}
		}
	}
	return blobs, err
}

// reads PREFIX LEN => LEN (e.g. "*3\r\n")
func respReadLengthMsg(prefix byte, r *bufio.Reader) (int, error) {
	// array header is "*N"
	b, err := r.ReadByte()
	if err != nil {
		return -1, err
	}
	if b != prefix {
		if b == '-' {
			// read error message
			msg, err := respReadLine(r)
			if err == nil {
				// Note: redis errors are of the format "CODE message"
				// We don't parse the code here, but we could.
				err = errors.New(string(msg))
			}
			return -1, err
		}
		r.UnreadByte()
		return -1, fmt.Errorf("expected resp prefix %q; got %q", prefix, b)
	}
	alen, err := readIntLine(r)
	return int(alen), err
}

// respReadLine reads the rest of the line, until "\r\n" ("\r\n" is discarded from the reader)
func respReadLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadSlice('\n')
	if err != nil {
		return nil, err
	}
	// sanity check: line ends in \r\n
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, fmt.Errorf("malformed resp %q", line)
	}
	return line[:len(line)-2], err
}

// readIntLine reads the rest of the line as an integer
func readIntLine(r *bufio.Reader) (int64, error) {
	line, err := respReadLine(r)
	if err != nil {
		return -1, err
	}
	return parseInt(line)
}

func respAppendArray(buf []byte, entries [][]byte) []byte {
	buf = respAppendArrayHeader(buf, len(entries))
	for _, entry := range entries {
		buf = respAppendBulkString(buf, entry)
	}
	return buf
}

func respAppendArrayHeader(buf []byte, length int) []byte {
	bufgrow(&buf, 1+intBase10MaxLen+2)
	buf = append(buf, '*')
	buf = strconv.AppendInt(buf, int64(length), 10)
	return append(buf, '\r', '\n')
}

func respAppendSimpleString(buf, data []byte) []byte {
	bufgrow(&buf, 1+len(data)+2)
	buf = append(buf, '+')
	buf = append(buf, data...)
	return append(buf, '\r', '\n')
}

func respAppendBulkString(buf, data []byte) []byte {
	// buf = respAppendBulkStringHeader(buf, len(data))
	bufgrow(&buf, 1+intBase10MaxLen+2+len(data)+2)
	buf = append(buf, '$')
	buf = strconv.AppendInt(buf, int64(len(data)), 10)
	buf = append(buf, '\r', '\n')
	buf = append(buf, data...)
	return append(buf, '\r', '\n')
}

func respAppendBulkStringHeader(buf []byte, length int) []byte {
	bufgrow(&buf, 1+intBase10MaxLen+2+length+2)
	buf = append(buf, '$')
	buf = strconv.AppendInt(buf, int64(length), 10)
	return append(buf, '\r', '\n')
}

// valuelen is expected to be base-10 encoded length of value
func respBulkStringLen(value, valuelen []byte) int {
	return 1 + len(valuelen) + 2 + len(value) + 2
}

// expects: len(buf)>=5+len(value)+len(valuelen)
// valuelen is expected to be base-10 encoded length of value
func respAddBulkString(buf, value, valuelen []byte) int {
	i := 0

	buf[i] = '$'
	i++
	i += copy(buf[i:], valuelen)
	buf[i] = '\r'
	buf[i+1] = '\n'
	i += 2

	i += copy(buf[i:], value)
	buf[i] = '\r'
	buf[i+1] = '\n'

	return i + 2
}

func splitRESPChunks(data []byte) [][]byte {
	var chunks [][]byte
	for i := 0; i < len(data); i++ {
		start := i
		b := data[i]
		i++
	findend:
		for ; i < len(data); i++ {
			switch data[i] {
			case '\r':
				chunks = append(chunks, data[start:i])
			case '\n':
				break findend
			}
		}
		if b == '$' {
			i++
			z, _ := parseUint(chunks[len(chunks)-1][1:])
			chunks = append(chunks, data[i:i+int(z)])
			i += int(z) + 1
		}
	}
	return chunks
}
