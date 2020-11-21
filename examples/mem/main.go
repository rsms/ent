//go:generate entgen
package main

import (
	"fmt"

	"github.com/rsms/ent"
	"github.com/rsms/ent/mem"
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

type Building int32

const (
	BuildingF1 = iota
	BuildingF2
	BuildingF3
	BuildingGymnasium = 100
	BuildingObservatory
)

type Department struct {
	ent.EntBase `dept`
	name        string
	building    Building `ent:",index"`
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

// MarshalJSON overrides the ent version to produce pretty-printed JSON
func (e *Account) MarshalJSON() ([]byte, error) {
	return ent.JsonEncode(e, "  ")
}

func main() {
	estore := mem.NewEntStorage()

	ids, _ := FindAccountByEmail(estore, "bob@bob.com")
	fmt.Printf("FindAccountByEmail bob@bob.com => %v\n", ids)

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

	ids1, _ := FindAccountByEmail(estore, "bob@bob.com")
	ids2, _ := FindAccountBySize(estore, 0, 0, -1)
	fmt.Printf("FindAccountByEmail bob@bob.com => %v\n", ids1)
	fmt.Printf("FindAccountBySize 0x0 => %v\n", ids2)

	a1.SetEmail("bobby@bob.com") // causes the "email" index to be updated
	a1.SetWidth(100)             // causes the "size" index to be updated
	fmt.Printf("IsEntFieldChanged(ent_Account_width) => %v\n",
		a1.IsEntFieldChanged(ent_Account_f_width))
	if err := a1.Save(); err != nil {
		panic(err)
	}

	// reflection
	fmt.Printf("GetFieldValue(ent_Account_f_width) => %v\n",
		ent.GetFieldValue(a1, ent_Account_f_width))

	// secondary index access
	ids3, _ := FindAccountByEmail(estore, "bob@bob.com")
	ids4, _ := FindAccountByEmail(estore, "bobby@bob.com")
	fmt.Printf("FindAccountByEmail bob@bob.com   => %v\n", ids3)
	fmt.Printf("FindAccountByEmail bobby@bob.com => %v\n", ids4)

	ids5, _ := FindAccountBySize(estore, 0, 0, -1)
	ids6, _ := FindAccountBySize(estore, 100, 0, -1)
	fmt.Printf("FindAccountBySize 0x0   => %v\n", ids5)
	fmt.Printf("FindAccountBySize 100x0 => %v\n", ids6)

	// -----
	// iteration and loading by limit & ent.Reverse

	// create some departments
	panicOnError((&Department{name: "Astronomy", building: BuildingF2}).Create(estore))
	panicOnError((&Department{name: "Computer Science", building: BuildingF2}).Create(estore))
	panicOnError((&Department{name: "Mathematical Sciences", building: BuildingF2}).Create(estore))
	panicOnError((&Department{name: "Art History", building: BuildingF2}).Create(estore))
	panicOnError((&Department{name: "Physics", building: BuildingF2}).Create(estore))
	panicOnError((&Department{name: "Psychology", building: BuildingF2}).Create(estore))
	panicOnError((&Department{name: "Religious Mythology", building: BuildingF3}).Create(estore))
	panicOnError((&Department{name: "Bookstore", building: BuildingF3}).Create(estore))

	// list all departments using an iterator. Iteration order is undefined; varies by storage.
	fmt.Printf("\n-- list all departments --\n")
	var d Department
	for it := d.Iterator(estore); it.Next(&d); {
		fmt.Printf("  %v\n", d.Name())
	}

	departments, _ := LoadDepartmentByBuilding(estore, BuildingF2, 0)
	fmt.Printf("\nLoadDepartmentByBuilding(BuildingF2, limit=0, flags=0) =>\n")
	for _, d := range departments {
		fmt.Printf("  %v\n", d.Name())
	}

	departments, _ = LoadDepartmentByBuilding(estore, BuildingF2, 3)
	fmt.Printf("\nLoadDepartmentByBuilding(BuildingF2, limit=3, flags=0) =>\n")
	for _, d := range departments {
		fmt.Printf("  %v\n", d.Name())
	}

	departments, _ = LoadDepartmentByBuilding(estore, BuildingF2, 3, ent.Reverse)
	fmt.Printf("\nLoadDepartmentByBuilding(BuildingF2, limit=3, flags=ent.Reverse) =>\n")
	for _, d := range departments {
		fmt.Printf("  %v\n", d.Name())
	}

	// delete all departments
	fmt.Printf("\n-- delete all departments & then list: (should be empty) --\n")
	panicOnError(ent.DeleteAllEntsOfType(estore, &Department{}))
	for it := d.Iterator(estore); it.Next(&d); {
		fmt.Printf("  #%d %s\n", d.Id(), d.Name())
	}
	fmt.Printf("(end list)\n")
}

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
