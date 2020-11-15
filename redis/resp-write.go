package redis

import (
	"io"
	"strconv"
)

type RWriter struct {
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
}

func (w *RWriter) ArrayHeader(length int) {
	w.buf = respAppendArrayHeader(w.buf, length)
}

func (w *RWriter) Blob(data []byte) {
	w.buf = respAppendBulkString(w.buf, data)
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

// RIOWriter is an extension of RWriter with buffered IO, writing to a io.Writer
type RIOWriter struct {
	RWriter
	w io.Writer
}

func (w *RIOWriter) Blob(data []byte) {
	w.RWriter.Blob(data)
	if len(w.buf) >= 1024 {
		w.Flush()
	}
}

func (w *RIOWriter) Flush() {
	// debugTrace("RIOWriter flush %q", w.buf)
	if w.err == nil {
		_, w.err = w.w.Write(w.buf)
	}
	w.buf = w.buf[:0]
}
