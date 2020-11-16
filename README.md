# ent -- Simple data entities for Go

[![GitHub tag (latest SemVer)](https://img.shields.io/github/tag/rsms/ent.svg)][godoc]
[![PkgGoDev](https://pkg.go.dev/badge/github.com/rsms/ent)][godoc]
[![Go Report Card](https://goreportcard.com/badge/github.com/rsms/ent)](https://goreportcard.com/report/github.com/rsms/ent)

[godoc]: https://pkg.go.dev/github.com/rsms/ent

Ent, short for "entities", is data persistence for Go on plain Go structs.

Features:

- No extra "description" files needed: just add tags to your Go structs.

- Automatically versioned ents.

- Automatic unique and non-unique secondary indexes (e.g. look up an account by email)
  including compound indexes.

- Transactional edits — a change either fully succeeds or not at all.

- Multiple storage backends with a small API for using custom storage.

- CouchDB-like get-modify-put — when "putting" it back, if the ent has changed by someone
  else since you loaded it, there will be a version conflict error and no changes will be made.

- Uses code generation instead of reflection to minimize magic — read the `go fmt`-formatted
  generated code to understand exactly what is happening.

- Generated code is included in your documentation (e.g. via `go doc`)


## Tutorial

Ent uses Go structs. By adding `ent.EntBase` as the first embedded field in a struct, you have
made it into an ent!

Here's a simple example:

```go
//go:generate entgen
package main

import "github.com/rsms/ent"

type AccountKind int

const (
  AccountRestricted = AccountKind(iota)
  AccountMember
  AccountAdmin
)

type Account struct {
  ent.EntBase `account` // type name, used to storage these kinds of ents
  name        string
  displayName string      `ent:"alias"`   // use a different field name for storage
  email       string      `ent:",unique"` // maintain a unique index for this field
  kind        AccountKind `ent:",index"`  // maintain a (non-unique) index for this field
}
```

When this file changes we should run `entgen` (or `go generate` if we have a `//go:generate`
comment in the file.) Doing so causes a few methods, functions and data to be generated for the
Account type, which makes it a fully-functional data entity.

We can now create, load, query, update and delete accounts.
But let's start by making one and just printing it:

```go
func main() {
  a := &Account{
    name:  "Jane",
    email: "jane@example.com",
    kind:  AccountMember,
  }
  println(a.String())
}
```

If we build & run this we should see the following output:

```
{
  _ver:  "0",
  _id:   "0",
  name:  "Jane",
  email: "jane@example.com",
  kind:  "1"
}
```

Notice a couple of things:

- The `String` method returns a JSON-like representation

- There are two implicit fields: `_ver` and `_id`. These represent the current version and id
  of an ent, respectively. These values can be accessed via the `Version()` and `Id()` methods.
  A value of `0` (zero) means "not yet assigned".

- The `displayName` field is called `alias`; renamed by the `ent` field tag.

Now let's store this account in a database. This is really what _ent_ is about — data persistence.
We start this example by creating a place to store ents, a storage. Here we use an in-memory
storage implementation `mem.EntStorage` but there are other kinds, like [Redis](redis/).

```go
import "github.com/rsms/ent/mem"

  // add to our main function:
  estore := mem.NewEntStorage()
  if err := a.Create(estore); err != nil {
    panic(err)
  }
  println(a.String())
```

Our account is now persisted.
The output from the last print statement now contains non-zero id and version:

```
{
  _ver:  "1",
  _id:   "1",
  name:  "Jane",
  ...
```

Another part of the program can load the ent:

```go
  a, _ = LoadAccountById(estore, 1)
  fmt.Printf("account #1: %v\n", a) // { _ver: "1", _id: "1", name: "Jane", ...
```

> Notice that we started using Go's "fmt" package. If you are following along, import "fmt".

Since we specified `unique` on the `email` field, we can look up ents by email in addition to id:

```go
  b, _ := LoadAccountByEmail(estore, "jane@example.com")
  fmt.Printf(`account with email "jane@example.com": %v\n`, b)
  // { _ver: "1", _id: "1", name: "Jane", ...

  _, err := FindAccountByEmail(estore, "does@not.exist")
  fmt.Printf("error from lookup of non-existing email: %v\n", err)
```

If we just need to check if an ent exists or we use a cache of some sort, we can use the `Find...`
functions instead of the `Load...` functions:

```go
  id, _ := FindAccountByEmail(estore, "jane@example.com")
  fmt.Printf(`id of account with email "jane@example.com": %v\n`, id) // 1
```

These functions were generated for us by `entgen`.
The `Find...ByFIELD` and `Load...ByFIELD` functions performs a lookup on a secondary index
("email" in the example above.)

In our struct definition we declared that we wanted the `kind` field to be indexed, which means
there are also functions for looking up accounts by kind. Indexes which are not unique, i.e.
indexes declared with the "index" field tag rather than the "unique" tag, returns a list of ents.
To make this example more interesting, let's create a few more ents to play with:

```go
  (&Account{email: "robin@foo.com", kind: AccountMember}).Create(estore)
  (&Account{email: "thor@xy.z", kind: AccountAdmin}).Create(estore)
  (&Account{email: "alice@es.gr", kind: AccountRestricted}).Create(estore)
```

And let's try querying for different kinds of users:

```go
  accounts, _ := LoadAccountByKind(estore, AccountMember, 0)
  fmt.Printf("'member' accounts: %+v\n", accounts)

  accounts, _ = LoadAccountByKind(estore, AccountAdmin, 0)
  fmt.Printf("'admin' accounts: %+v\n", accounts)
```

We should see "Jane" and "robin" listed for `AccountMember` and "thor" for `AccountAdmin`.

Non-unique indexes as we just explored does not imply any constraints on ents.
But unique indexes do — it's kind of the whole point with a _unique_ index :-)
When we create or update an ent with a change to a unique index we may get an error in case
there is a conflict. For example, let's try creating a new account that uses the same email
address as Jane's account:

```go
  err = (&Account{email: "jane@example.com"}).Create(estore)
  fmt.Printf("error (duplicate email): %v\n", err)
  // unique index conflict account.email with ent #1
```

The same would happen if we tried to update an account to use an already-used email value:

```go
  a, _ = LoadAccountByEmail(estore, "robin@foo.com")
  a.SetEmail("jane@example.com")
  fmt.Printf("error (duplicate email): %v\n", a.Save())
  // unique index conflict account.email with ent #1
```

However if we change the email of Jane's account, we then use the email address Jane used to use
for other accounts:

```go
  a, _ = LoadAccountById(estore, 1)
  a.SetEmail("jane.smith@foo.z")
  a.Save()

  a, _ = LoadAccountByEmail(estore, "robin@foo.com")
  a.SetEmail("jane@example.com")
  fmt.Printf("no error: %v\n", a.Save())
```

The ent system maintains these indexes automatically and updating them in a transactional manner:
a `Create` or `Save` call either fully succeeds, including index changes, or doesn't have an
effect at all on any sort of failure. This a promise declared by the ent system but actually
fulfilled by the particular storage used. Both of the storage implementations that comes with
ent are fully transactional (mem and redis.)
