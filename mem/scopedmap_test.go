package mem

import (
	"testing"

	"github.com/rsms/go-testutil"
)

func TestScopedMap(t *testing.T) {
	assert := testutil.NewAssert(t)
	assert.Ok("ok", true)

	var empty []byte

	var m1 ScopedMap
	m1.Put("a", []byte{'a'})
	m1.Put("b", []byte{'b'})

	m2 := m1.NewScope()
	m2.Put("c", []byte{'c'})
	m2.Del("b")

	// value of m2
	assert.Eq("get", m2.Get("a"), []byte("a"))
	assert.Eq("get", m2.Get("b"), empty)
	assert.Eq("get", m2.Get("c"), []byte("c"))

	// values of m1
	assert.Eq("get", m1.Get("a"), []byte("a"))
	assert.Eq("get", m1.Get("b"), []byte("b"))
	assert.Eq("get", m1.Get("c"), empty)

	// apply changes in m2 to m1
	m2.ApplyToOuter()
	assert.Eq("get", m1.Get("a"), []byte("a"))
	assert.Eq("get", m1.Get("b"), empty)
	assert.Eq("get", m1.Get("c"), []byte("c"))
}
