package mem

// ScopedMap is like a map[string][]byte but prototypal in behaviour; local read misses causes
// a parent ScopedMap to be tried, while writes are always local. Sort of like a hacky HAMT map.
type ScopedMap struct {
	outer *ScopedMap
	m     map[string][]byte
}

func (s ScopedMap) Get(key string) []byte {
	if s.m != nil {
		v, ok := s.m[key]
		if ok {
			return v
		}
	}
	if s.outer != nil {
		return s.outer.Get(key)
	}
	return nil
}

func (s *ScopedMap) Put(key string, value []byte) {
	if value == nil {
		s.Del(key)
	} else {
		if s.m == nil {
			s.m = make(map[string][]byte)
		}
		s.m[key] = value
	}
}

func (s *ScopedMap) Del(key string) {
	if s.outer == nil {
		delete(s.m, key)
	} else {
		if s.m == nil {
			s.m = make(map[string][]byte)
		}
		s.m[key] = nil
	}
}

func (s *ScopedMap) NewScope() *ScopedMap {
	return &ScopedMap{outer: s}
}

// ApplyToOuter applies all entries (including deletes) of this scope to its outer scope.
// This effectively moves changes from this scope to the outer scope, clearing this scope.
func (s *ScopedMap) ApplyToOuter() {
	for k, v := range s.m {
		s.outer.Put(k, v)
	}
	s.m = nil
}
