package redis

import (
	"fmt"
	"math"
	"strconv"
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

// fmtint formats a uint64. buf must be len(buf)>=N where N is:
// base==16, N=16
// base==10, N=20
// base==8, N=
func fmtint(buf []byte, u uint64, base int) []byte {
	digits := "0123456789abcdef"
	i := len(buf)
	if base == 16 {
		for u >= 16 {
			i--
			buf[i] = digits[u&0xF]
			u >>= 4
		}
	} else {
		for u >= 10 {
			i--
			next := u / 10
			buf[i] = byte('0' + u - next*10)
			u = next
		}
	}
	i--
	buf[i] = digits[u]
	return buf[i:]
}

// func init() {
// 	var scratch [20]byte
// 	f := func(u uint64, base int) []byte { return fmtint(scratch[:], u, base) }
// 	fmt.Printf("fmtint 16 0x0 => %q\n", f(0x0, 16))
// 	fmt.Printf("fmtint 16 0x1 => %q\n", f(0x1, 16))
// 	fmt.Printf("fmtint 16 0xDEADBEEF => %q\n", f(0xDEADBEEF, 16))
// 	fmt.Printf("fmtint 16 0xFFFFFFFFFFFFFFFF => %q\n", f(0xFFFFFFFFFFFFFFFF, 16))
// 	fmt.Printf("fmtint 10 0 => %q\n", f(0, 10))
// 	fmt.Printf("fmtint 10 9 => %q\n", f(9, 10))
// 	fmt.Printf("fmtint 10 18446744073709551615 => %q\n", f(18446744073709551615, 10))
// }

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

func parseHexUint(b []byte) (uint64, error) {
	var u uint64
	for i, c := range b {
		switch c {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			c -= '0'
		case 'A', 'B', 'C', 'D', 'E', 'F':
			c -= 'A' - 10
		case 'a', 'b', 'c', 'd', 'e', 'f':
			c -= 'a' - 10
		default:
			return 0, fmt.Errorf("parseHexUint: invalid byte %c at %d", c, i)
		}
		u = (u << 4) | uint64(c&0xF)
	}
	return u, nil
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
			return 0, fmt.Errorf("parseUint: invalid byte %c at %d", c, i)
		}
		n *= 10
		n += uint64(c - '0')
	}
	return n, nil
}

func appendFloat(b []byte, v float64, bitsize int) []byte {
	fmt := byte('f')
	// Note: Must use float32 comparisons for underlying float32 value to get precise cutoffs right.
	abs := math.Abs(v)
	if abs != 0 {
		if bitsize == 64 && (abs < 1e-6 || abs >= 1e21) ||
			bitsize == 32 && (float32(abs) < 1e-6 ||
				float32(abs) >= 1e21) {
			fmt = 'e'
		}
	}
	return strconv.AppendFloat(b, v, fmt, -1, bitsize)
}
