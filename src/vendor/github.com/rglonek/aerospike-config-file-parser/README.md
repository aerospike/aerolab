# Aerospike Configuration File Parser

## Types

```go
// stanza will be of type map[string]nil, map[string]string, map[string]stanza
type stanza map[string]interface{}

// types returned by the stanza.Type() function
type ValueType string

const ValueString = ValueType("string")
const ValueNil = ValueType("nil")
const ValueStanza = ValueType("stanza")
const ValueUnknown = ValueType("unknown")
```

## Functions

```go
// open a file and pass it to Parse
func ParseFile(f string) (s stanza, err error)

// parse given reader and produce a stanza variable with the parse results
func Parse(r io.Reader) (s stanza, err error)

// create a file and pass handler to Write
func (s stanza) WriteFile(f string, prefix string, indent string, sortItems bool) (err error)

// write back the stanza, as defined, to the writer, optionally sorting the items in preferred aerospike sorting order
// prefix will be appended to every line
// indent will be used to indent each line accordingly
func (s stanza) Write(w io.Writer, prefix string, indent string, sortItems bool) (err error)

// get values associated with the key
// if key is stanza or doesn't exit, nil will be returned
func (s stanza) GetValues(key string) ([]*string, error)

// set string value for a given key
func (s stanza) SetValue(key string, value string) error

// set one or multiple values for the given key
// if multiple values are given, Write will repeat the key with each value in the final output
func (s stanza) SetValues(key string, values []*string) error

// delete a given key
func (s stanza) Delete(key string) error

// create a new sub-stanza
func (s stanza) NewStanza(key string) error

// helper function to change []string to []*string
func SliceToValues(val []string) []*string
```

## Example

```go
func main() {
	// parse file into 's'
	s, err := aeroconf.ParseFile("/etc/aerospike.conf")
	if err != nil {
		log.Fatal(err)
	}

	// print types of variables
	fmt.Println(s.Type("service"))
	fmt.Println(s.Stanza("service").Type("proto-fd-max"))

	// get value of proto-fd-max
	out, _ := s.Stanza("service").GetValues("proto-fd-max")
	for _, i := range out {
		fmt.Println(*i)
	}

	// adjust value of proto-fd-max
	if s.Stanza("service") == nil {
		s.NewStanza("service")
	}
	s.Stanza("service").SetValue("proto-fd-max", "30000")
	
	// change heartbeat mode to mesh
	if s.Stanza("network") == nil {
		s.NewStanza("network")
	}
	if s.Stanza("network").Stanza("heartbeat") == nil {
		s.Stanza("network").NewStanza("heartbeat")
	}
	s.Stanza("network").Stanza("heartbeat").Delete("multicast-group")
	s.Stanza("network").Stanza("heartbeat").SetValue("mode", "mesh")
	s.Stanza("network").Stanza("heartbeat").SetValues("mesh-seed-address-port", SliceToValues([]string{"172.17.0.2 3000", "172.17.0.3 3000"}))

	// remove and rewrite network.info stanza completely
	s.Stanza("network").Delete("info")
	s.Stanza("network").NewStanza("info")
	s.Stanza("network").Stanza("info").SetValue("port", "3003")

	// write back contents of 's'
	err = s.WriteFile("/etc/aerospike", "", "    ", true)
	if err != nil {
		log.Fatal(err)
	}
}
```
