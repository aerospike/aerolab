# StringByteSlice

Convert between String and Byte Slice without copying memory.

## WARNING

Read the usage notes on `pkg.dev`. Use with caution.

## Standard behaviour

Type | Behaviour
--- | ---
`[]byte` | Has value, length (current value length) and cap (the size of the underlying array). Can modify byte values in place. Can grow length up to cap size. If new length exceeds cap, a copy of the underlying array will be performed.
`string` | Has value and length (string length). Cannot be modified in place. Any modification results in a new string being allocated.

Standard conversion of `str := string([]byte{'t'})` and `slice := []byte("s")` results in an allocation and complete copy of the memory. This is to ensure safety with the above variable behaviours and detach the underlying data.

## This package

This package provide functions for casting a `[]byte` to `string` and back. This is done using the `unsafe` package. It is much faster than the standard behaviours, as no underlying data copy is performed.

The casting means the developer must ensure that the variables will NOT be modified, as it will result in breaking the expectation of the standard beaviour of strings. It may also result in unexpected results if attempting to grow the byte slice.

There are two methods of casting between byte slices and string provided.

## Speed comparison

```
goos: darwin
goarch: amd64
pkg: sbs
cpu: Intel(R) Core(TM) i9-9880H CPU @ 2.30GHz
Benchmark                                Runs             Speed             Alloc        Allocs         Speed gain compared to standard
---------                                ----             -----             -----        ------         -------------------------------
BenchmarkByteSliceToString-16            1000000000       0.2426 ns/op      0 B/op       0 allocs/op    15.8x
BenchmarkByteSliceToStringAlt-16         1000000000       0.4315 ns/op      0 B/op       0 allocs/op     8.9x
BenchmarkByteSliceToStringStandard-16    310850160        3.847 ns/op       0 B/op       0 allocs/op       -
BenchmarkStringToByteSlice-16            1000000000       0.3538 ns/op      0 B/op       0 allocs/op    11.4x
BenchmarkStringToByteSliceStandard-16    301289979        4.027 ns/op       0 B/op       0 allocs/op       -
```

## Examples

```go
r := ByteSliceToString([]byte{'t', 'e', 's', 't', 'i', 'n', 'g'})
r := ByteSliceToStringAlt([]byte{'t', 'e', 's', 't', 'i', 'n', 'g'})
r := StringToByteSlice("testing")
```
