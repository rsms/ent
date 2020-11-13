package ent

import (
	"bytes"
	"sort"
	"strconv"
)

type idSet []uint64

func parseIdSet(data []byte) idSet {
	var ids idSet
	if len(data) > 0 {
		for _, chunk := range bytes.Split(data, []byte{' '}) {
			u, err := strconv.ParseUint(string(chunk), 10, 64)
			if err != nil {
				panic("failed to parse idSet " + err.Error())
			}
			ids = append(ids, u)
		}
	}
	return ids
}

func encodeIds(ids []uint64) []byte {
	buf := make([]byte, 0, 10*len(ids))
	for i, id := range ids {
		if i > 0 {
			buf = append(buf, ' ')
		}
		buf = strconv.AppendUint(buf, id, 10)
	}
	return buf
}

func (s idSet) Encode() []byte {
	return encodeIds(s)
}

func (s idSet) Has(id uint64) bool {
	for _, v := range s {
		if v == id {
			return true
		}
	}
	return false
}

func (s *idSet) Add(id uint64) {
	for _, v := range *s {
		if v == id {
			return
		}
	}
	*s = append(*s, id)
}

func (s *idSet) Del(id uint64) {
	for i, v := range *s {
		if v == id {
			// splice
			copy((*s)[i:], (*s)[i+1:])
			*s = (*s)[:len((*s))-1]
			break
		}
	}
}

func (s idSet) Sort() {
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
}
