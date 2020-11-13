package main

import (
	"fmt"

	"github.com/rsms/ent"
	uid "github.com/rsms/go-uuid"
)

type Thing1 int64
type Thing2 Thing1
type Thing3 Thing2

type Data []byte

// Account represents a user account
type Account struct {
	ent.EntBase `account`
	name        string

	// width in pixels
	width, height int      `ent:"w,h,index=size"`
	uuid          uid.UUID `ent:",unique"`
	flag          uint16   `ent:",index"`
	score         float32  `ent:",index"`
	picture       []byte   `ent:",index"`
	email         string   `ent:",unique"`
	emailVerified bool     `ent:"email_verified,badtag"`
	Deleted       bool
	passwordHash  string `ent:"pwhash" json:"-"` // omit from json (never leak)
	thing         Thing3
	foo           []int
	foofoo        [][]int16
	data          Data
	rgb           [3]int
	threebytes    [3]byte
	things        map[string]int
}

func (e *Account) SetSize(w, h int) {
	e.setWidth(w)
	e.SetHeight(h)
}

func (e *Account) SetWidth(w int) {
	// custom implementation of setter SetWidth causes entgen to create setWidth instead
	if w > 0 {
		e.setWidth(w)
	}
}

func (e *Account) MarshalJSON() ([]byte, error) {
	return ent.JsonEncode(e)
}

// type Location struct {
// 	ent.EntBase `loc`
// }

func main() {
	estore := ent.NewMemoryStorage()

	indexLookup := func(index, key string) []uint64 {
		ids, err := estore.FindEntIdsByIndex(Account{}.EntTypeName(), index, []byte(key))
		if err != nil {
			panic(err)
		}
		return ids
	}

	ids := indexLookup("email", "bob@bob.com")
	fmt.Printf("indexLookup email bob@bob.com => %v\n", ids)

	a1 := &Account{}
	a1.SetName("bob")
	a1.SetFlag(4)
	a1.SetScore(1.391)
	a1.SetEmail("bob@bob.com")
	a1.SetFoo([]int{1, 2, 3})
	a1.SetFoofoo([][]int16{
		{1, 2, 3},
		{},
		{10, 20},
	})
	a1.SetRgb([3]int{1, 2, 3})
	a1.threebytes[0] = 'A'
	a1.threebytes[1] = 'B'
	a1.threebytes[2] = 'C'
	a1.SetThreebytesChanged()
	a1.SetThings(map[string]int{"a": 1, "b": 2})
	if err := a1.Create(estore); err != nil {
		panic(err)
	}
	fmt.Printf("created account #%d: %+v\n", a1.Id(), a1)

	a1b, err := LoadAccountById(estore, a1.Id())
	if err != nil {
		panic(err)
	}
	fmt.Printf("loaded account #%d: %+v\n", a1b.Id(), a1b)

	a1c, err := LoadAccountByEmail(estore, "bob@bob.com")
	if err != nil {
		panic(err)
	}
	fmt.Printf("loaded account by email %q: %+v\n", "bob@bob.com", a1c)

	ids1 := indexLookup("email", "bob@bob.com")
	ids2 := indexLookup("size", "h\xff0\xffw\xff0")
	fmt.Printf("indexLookup email bob@bob.com => %v\n", ids1)
	fmt.Printf("indexLookup size 0x0 => %v\n", ids2)

	a1.SetEmail("bobby@bob.com") // causes the "email" index to be updated
	a1.SetWidth(100)             // causes the "size" index to be updated
	fmt.Printf("ent.IsFieldChanged(ent_Account_width) => %v\n",
		a1.EntIsFieldChanged(ent_Account_width))
	if err := a1.Save(); err != nil {
		panic(err)
	}

	ids1 = indexLookup("email", "bob@bob.com")
	ids2 = indexLookup("email", "bobby@bob.com")
	fmt.Printf("indexLookup email bob@bob.com   => %v\n", ids1)
	fmt.Printf("indexLookup email bobby@bob.com => %v\n", ids2)

	ids1 = indexLookup("size", "h\xff0\xffw\xff0")
	ids2 = indexLookup("size", "h\xff0\xffw\xff2s") // base36
	fmt.Printf("indexLookup size 0x0   => %v\n", ids1)
	fmt.Printf("indexLookup size 100x0 => %v\n", ids2)

	ids, err = FindAccountBySize(estore, 100, 0)
	if err != nil {
		panic(err)
	}
	fmt.Printf("FindAccountBySize(100,0) => %v\n", ids)

	// a2 := &Account{}
	// a2.SetName("jane")
	// a2.SetEmail("jane@gmail.com")
	// a2.SetFoo([]int{})
	// a2.SetFoofoo([][]int16{
	// 	{1, 2, 3},
	// 	{},
	// 	{10, 20},
	// })
	// if err := a2.Create(estore); err != nil {
	// 	panic(err)
	// }
	// fmt.Printf("created account #%d\n", a2.Id())

}
