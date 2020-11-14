package redis

import (
	"io"
	"strconv"
)

type RWriter struct {
	w   io.Writer
	buf []byte
	err error
}

// Err returns the error state of the writer
func (w *RWriter) Err() error { return w.err }

func (w *RWriter) StringArray(sv ...string) {
	w.ArrayHeader(len(sv))
	for _, s := range sv {
		w.buf = respAppendBulkString(w.buf, []byte(s))
	}
	w.flushIfNeeded()
}

func (w *RWriter) ArrayHeader(length int) {
	w.buf = respAppendArrayHeader(w.buf, length)
}

func (w *RWriter) Blob(data []byte) {
	w.buf = respAppendBulkString(w.buf, data)
	w.flushIfNeeded()
}

func (w *RWriter) Str(s string) {
	w.Blob([]byte(s))
}

func (w *RWriter) Int(v int64, bitsize int) {
	bufgrow(&w.buf, 1+intBase10MaxLen+2)
	w.buf = append(w.buf, ':')
	w.buf = strconv.AppendInt(w.buf, v, 10)
	w.buf = append(w.buf, '\r', '\n')
}

func (w *RWriter) Uint(v uint64, bitsize int) {
	bufgrow(&w.buf, 1+intBase10MaxLen+2)
	w.buf = append(w.buf, ':')
	w.buf = strconv.AppendUint(w.buf, v, 10)
	w.buf = append(w.buf, '\r', '\n')
}

func (w *RWriter) Float(v float64, bitsize int) {
	bufgrow(&w.buf, 32)
	w.buf = append(w.buf, ':')
	w.buf = appendFloat(w.buf, v, bitsize)
	w.buf = append(w.buf, '\r', '\n')
}

func (w *RWriter) Bool(v bool) {
	// true  = ":1\r\n"
	// false = ":0\r\n"
	b := byte('0')
	if v {
		b = '1'
	}
	bufgrow(&w.buf, 4)
	w.buf = append(w.buf, ':', b, '\r', '\n')
}

// -----

func (w *RWriter) flushIfNeeded() {
	if len(w.buf) >= 1024 {
		w.Flush()
	}
}

func (w *RWriter) Flush() {
	// debugTrace("RWriter flush %q", w.buf)
	if w.err == nil {
		_, w.err = w.w.Write(w.buf)
	}
	w.buf = w.buf[:0]
}
