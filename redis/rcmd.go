package redis

import (
	"bufio"
	"fmt"
	"io"

	"github.com/mediocregopher/radix/v3"
)

type RCmd struct { // conforms to radix.Marshaler
	Encode func(w *RWriter) error
	Decode func(r *RReader) error
}

func (a *RCmd) Keys() []string { return []string{} }

func (a *RCmd) String() string {
	return fmt.Sprintf("RCmd{Encode:%v, Decode:%v}", a.Encode, a.Decode)
}

func (a *RCmd) Run(c radix.Conn) error {
	if err := c.Encode(a); err != nil {
		return err
	}
	return c.Decode(a)
}

func (a *RCmd) MarshalRESP(w io.Writer) error {
	writer := RWriter{w: w, buf: make([]byte, 0, 128)}
	if err := a.Encode(&writer); err != nil {
		return err
	}
	if len(writer.buf) > 0 {
		writer.Flush()
	}
	return writer.err
}

func (a *RCmd) UnmarshalRESP(r *bufio.Reader) error {
	var buf [32]byte // must be at least intBase10MaxLen
	reader := RReader{r: r, buf: buf[:]}
	err := a.Decode(&reader)
	if err == nil {
		err = reader.err
	}
	return err
}
