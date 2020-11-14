package redis

import (
	"fmt"
)

func bufgrow(buf *[]byte, addlSizeNeeded int) {
	if cap(*buf)-len(*buf) < addlSizeNeeded {
		_bufgrow(buf, addlSizeNeeded)
	}
}

func _bufgrow(buf *[]byte, z int) {
	l := len(*buf)
	buf2 := make([]byte, l, cap(*buf)*2+z)
	copy(buf2, *buf)
	*buf = buf2
}

// parseInt is a specialized version of strconv.ParseInt
func parseInt(b []byte) (int64, error) {
	if len(b) == 0 {
		return 0, nil
	}
	var neg bool
	if b[0] == '-' || b[0] == '+' {
		neg = b[0] == '-'
		b = b[1:]
	}
	n, err := parseUint(b)
	if err != nil {
		return 0, err
	}
	if neg {
		return -int64(n), nil
	}
	return int64(n), nil
}

// parseUint is a specialized version of strconv.ParseUint
func parseUint(b []byte) (uint64, error) {
	// inlineable fast path for small numbers, which is common in the redis protocol
	if len(b) == 1 {
		return uint64(b[0] - '0'), nil
	}
	return _parseUint(b)
}

// slow path of parseUint
func _parseUint(b []byte) (uint64, error) {
	var n uint64
	for i, c := range b {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid character %c at position %d in parseUint", c, i)
		}
		n *= 10
		n += uint64(c - '0')
	}
	return n, nil
}
