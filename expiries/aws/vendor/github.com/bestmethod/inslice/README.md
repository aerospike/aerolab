# InSlice

[![Go Report Card](https://goreportcard.com/badge/github.com/bestmethod/inslice)](https://goreportcard.com/report/github.com/bestmethod/inslice) [![Travis Build Status](https://travis-ci.com/bestmethod/inslice.svg?branch=master)](https://travis-ci.com/bestmethod/inslice) [![CodeCov Code Coverage](https://codecov.io/github/bestmethod/inslice/coverage.svg)](https://codecov.io/github/bestmethod/inslice/) [![Godoc](https://godoc.org/github.com/bestmethod/inslice?status.svg)](https://godoc.org/github.com/bestmethod/inslice)

## Description

A small range of functions for comparing if item is present in slice and receiving the index results.

## Functions

### Int

Functions are provided for slice of int, and for slice of ptr to int, as well as "All" versions, for finding all occurrances.

We always compare values, never pointers themselves.

```go
func Int(slice []int, item int, count int) (index []int) {}
func IntVar(slice []*int, item int, count int) (index []int) {}

func IntAll(slice []int, item int) (index []int) {}
func IntVarAll(slice []*int, item int) (index []int) {}
```

### String

Functions are provided for slice of string, and for slice of ptr to string, as well as "All" versions, for finding all occurrances.

We always compare values, never pointers themselves.

```go
func String(slice []string, item string, count int) (index []int) {}
func StringVar(slice []*string, item string, count int) (index []int) {}

func StringAll(slice []string, item string) (index []int) {}
func StringVarAll(slice []*string, item string) (index []int) {}
```

### Uint

Functions are provided for slice of uint, and for slice of ptr to uint, as well as "All" versions, for finding all occurrances.

We always compare values, never pointers themselves.

```go
func Uint(slice []uint, item uint, count int) (index []int) {}
func UintVar(slice []*uint, item uint, count int) (index []int) {}

func UintAll(slice []uint, item uint) (index []int) {}
func UintVarAll(slice []*uint, item uint) (index []int) {}
```

### Reflect

Functions are provided for slice of items, as well as "All" version, for finding all occurrances

Reflect automatically checks if the slice/item contain Ptr to actual values, or actual values. If it's the Ptr, the function will compare the values themselves, not check if the Ptr are pointing to the same place. Hence, no equivalent of previous functions with "Var".

```go
func Reflect(slice interface{}, item interface{}, count int) (index []int, err error) {}
func ReflectAll(slice interface{}, item interface{}) (index []int, err error) {}
```

## Examples

### Is item in slice

```go
package main

import "github.com/bestmethod/inslice"

func main() {
    slice := []int{1,2,3}
    item := 2
    if len(inslice.Int(slice, item, 1)) != 0 {
        fmt.Println("Item found in slice")
    }
}
```

### Index for first 2 occurrances of item in slice

```go
package main

import "github.com/bestmethod/inslice"

func main() {
    slice := []string{"a","b","c","a","b","c","a","b","c"}
    item := "b"
    fmt.Println(inslice.String(slice, item, 2))
}
```

### Index of all occurances of item in slice, using interfaces and reflect

```go
package main

import "github.com/bestmethod/inslice"

func main() {
	slice := []string{
		"text1",
		"text2",
		"text3",
		"text2",
	}
	var sliceInterface interface{}
    sliceInterface = slice
    
	st := "text2"
	var itemInterface interface{}
    itemInterface = st
    
    indexes, err := ReflectAll(sliceInterface, itemInterface)
    if err != nil {
        log.Fatalf("I did something wrong: %s", err)
    }
    fmt.Println(indexes)
}
```
