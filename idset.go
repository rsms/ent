package ent

import (
	"bytes"
	"sort"
	"strconv"
)

// IdSet is a list of integers which are treated as a set
type IdSet []uint64

func ParseIdSet(data []byte) IdSet {
	var ids IdSet
	if len(data) > 0 {
		for _, chunk := range bytes.Split(data, []byte{' '}) {
			u, err := strconv.ParseUint(string(chunk), 10, 64)
			if err != nil {
				panic("failed to parse IdSet " + err.Error())
			}
			ids = append(ids, u)
		}
	}
	return ids
}

func (s IdSet) Encode() []byte {
	buf := make([]byte, 0, 10*len(s))
	for i, id := range s {
		if i > 0 {
			buf = append(buf, ' ')
		}
		buf = strconv.AppendUint(buf, id, 10)
	}
	return buf
}

func (s IdSet) Has(id uint64) bool {
	for _, v := range s {
		if v == id {
			return true
		}
	}
	return false
}

func (s *IdSet) Add(id uint64) {
	for _, v := range *s {
		if v == id {
			return
		}
	}
	*s = append(*s, id)
}

func (s *IdSet) Del(id uint64) {
	for i, v := range *s {
		if v == id {
			// splice
			copy((*s)[i:], (*s)[i+1:])
			*s = (*s)[:len((*s))-1]
			break
		}
	}
}

func (s IdSet) Sort() {
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
}
