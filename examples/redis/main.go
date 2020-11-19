//go:generate entgen
package main

import (
	"fmt"

	"github.com/rsms/ent"
	"github.com/rsms/ent/redis"
	"github.com/rsms/go-uuid"
)

type Account struct {
	ent.EntBase   `account`
	name          string `ent:",index"`
	email         string `ent:",unique"`
	emailVerified bool   `ent:"email_verified"`
	deleted       bool
	passwordHash  string `ent:"pwhash" json:"-"` // omit from json (never leak)
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

func main() {
	r := &redis.Redis{}
	panicOnError(r.Open("127.0.0.1:6379", "", 1))
	estore := redis.NewEntStorage(r)

	// create
	fmt.Printf("\n-- create --\n")
	a1 := &Account{}
	a1.SetName("bob")
	a1.SetEmail(uuid.MustGen().String() + "@bob.com")
	panicOnError(a1.Create(estore))
	fmt.Printf("created account #%d: %+v\n", a1.Id(), a1)

	// create a second account with the same email
	fmt.Printf("\n-- create with duplicate email --\n")
	a2 := &Account{}
	a2.SetEmail(a1.Email())
	fmt.Printf("create w same email [should fail] => error: %v\n", a2.Create(estore))

	// load
	fmt.Printf("\n-- load --\n")
	a3, err := LoadAccountById(estore, a1.Id())
	panicOnError(err)
	fmt.Printf("loaded account #%d: %+v\n", a1.Id(), a3)

	// save
	fmt.Printf("\n-- save --\n")
	a1.SetName("Bobby")
	a1.SetEmail(uuid.MustGen().String() + "@bob.com")
	panicOnError(a1.Save())
	fmt.Printf("saved account #%d\n", a1.Id())

	// save outdated should fail
	fmt.Printf("\n-- save outdated --\n")
	a3.SetName("Bo")
	fmt.Printf("outdated.Save() [should fail] => error: %v\n", a3.Save())

	// load by unique index
	fmt.Printf("\n-- load by unique index 'email' --\n")
	a4, err := LoadAccountByEmail(estore, a1.Email())
	panicOnError(err)
	fmt.Printf("loaded account by email %q: %+v\n", a1.Email(), a4)

	// create another account with the same name
	fmt.Printf("\n-- create --\n")
	a5 := &Account{}
	a5.SetName(a1.Name())
	panicOnError(a5.Create(estore))

	// find by non-unique index
	fmt.Printf("\n-- find by non-unique index 'name' --\n")
	ids, err := FindAccountByName(estore, a1.Name(), 10)
	panicOnError(err)
	fmt.Printf("loaded account IDs with name %q: %+v\n", a1.Name(), ids)

	// delete an account
	fmt.Printf("\n-- delete and then load (now non-existing) ent --\n")
	panicOnError(a1.PermanentlyDelete())
	_, err = LoadAccountById(estore, a1.Id())
	fmt.Printf("LoadAccountById(%d) [should fail] => error: %v\n", a1.Id(), err)

	// -----
	// iteration and loading by limit & ent.Reverse

	fmt.Printf("\n-- delete all department ents & recreate them --\n")
	// remove any and all preexisting departments from a previous run
	panicOnError(ent.DeleteAllEntsOfType(estore, &Department{}))

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
		fmt.Printf("#%d %s\n", d.Id(), d.Name())
	}

	departments, err := LoadDepartmentByBuilding(estore, BuildingF2, 0)
	panicOnError(err)
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

	// --- end
	estore.Close()
	r.Close()
}

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
