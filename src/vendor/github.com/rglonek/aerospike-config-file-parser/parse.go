package aeroconf

import (
	"bufio"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
)

type Stanza map[string]interface{}

type ValueType string

const ValueString = ValueType("string")
const ValueNil = ValueType("nil")
const ValueStanza = ValueType("stanza")
const ValueUnknown = ValueType("unknown")

func ParseFile(f string) (s Stanza, err error) {
	var r *os.File
	r, err = os.Open(f)
	if err != nil {
		return nil, errors.New("could not open file: " + err.Error())
	}
	defer r.Close()
	return Parse(r)
}

func Parse(r io.Reader) (s Stanza, err error) {
	s = make(Stanza)
	scanner := bufio.NewScanner(r)
	err = s.parseLines(scanner)
	return s, err
}

func (s Stanza) parseLines(scanner *bufio.Scanner) (err error) {
	for scanner.Scan() {
		// read line, ignore empty lines
		linex := scanner.Text()
		if len(linex) == 0 {
			continue
		}
		// remove comments
		line := linex
		ind := strings.IndexRune(line, '#')
		if ind >= 0 {
			line = line[0:ind]
		}
		// trim any mess, check line isn't empty and check if it's a substanza
		line = strings.Trim(line, "\r\n\t ")
		if len(line) == 0 {
			continue
		}
		if strings.HasSuffix(line, "{") {
			// substanza found
			k := strings.Trim(line, "{ ")
			if len(k) == 0 {
				return errors.New("line has empty stanza name: " + linex)
			}
			sub := make(Stanza)
			err = sub.parseLines(scanner)
			if err != nil {
				return
			}
			s[k] = sub
		} else if line == "}" {
			// return from stanza
			return
		} else {
			// key:value
			kv := strings.Split(line, " ")
			if len(kv) == 0 {
				return errors.New("line is not a `key value': " + linex)
			}
			if len(kv) == 1 {
				if _, ok := s[kv[0]]; !ok {
					s[kv[0]] = []*string{nil}
				} else {
					s[kv[0]] = append(s[kv[0]].([]*string), nil)
				}
			} else {
				vv := strings.Join(kv[1:], " ")
				if _, ok := s[kv[0]]; !ok {
					s[kv[0]] = []*string{&vv}
				} else {
					s[kv[0]] = append(s[kv[0]].([]*string), &vv)
				}
			}
		}
	}
	return
}

func (s Stanza) WriteFile(f string, prefix string, indent string, sortItems bool) (err error) {
	w, err := os.Create(f)
	if err != nil {
		return errors.New("creating file: " + err.Error())
	}
	defer w.Close()
	return s.Write(w, prefix, indent, sortItems)
}

func (s Stanza) Write(w io.Writer, prefix string, indent string, sortItems bool) (err error) {
	return s.write(w, prefix, indent, "", sortItems, "")
}

func getSortOrder() []string {
	return []string{
		".service",
		".logging",
		".security",
		".security.ldap",
		".security.log",
		".network",
		".network.tls",
		".network.service",
		".network.heartbeat",
		".network.fabric",
		".network.info",
		".namespace",
		".xdr",
	}
}

func (s Stanza) write(w io.Writer, prefix string, indent string, currentIndent string, sortItems bool, top string) (err error) {
	var keys []string
	if sortItems {
		for i := range s {
			keys = append(keys, i)
		}
		so := getSortOrder()
		sort.Slice(keys, func(i, j int) bool {
			k1 := top + "." + strings.Split(keys[i], " ")[0]
			k2 := top + "." + strings.Split(keys[j], " ")[0]
			k1ind := -1
			k2ind := -1
			for ni, item := range so {
				if k1 == item {
					k1ind = ni
				}
				if k2 == item {
					k2ind = ni
				}
				if k1ind != -1 && k2ind != -1 {
					break
				}
			}
			if k1ind == -1 && k2ind == -1 {
				return keys[i] < keys[j]
			}
			return k1ind < k2ind
		})
		for _, k := range keys {
			err = s.writeLine(w, prefix, indent, currentIndent, sortItems, k, top)
			if err != nil {
				return err
			}
		}
		return
	}
	for k := range s {
		err = s.writeLine(w, prefix, indent, currentIndent, sortItems, k, top)
		if err != nil {
			return
		}
	}
	return
}

func (s Stanza) writeLine(w io.Writer, prefix string, indent string, currentIndent string, sortItems bool, k string, top string) (err error) {
	item := s[k]
	switch v := item.(type) {
	case string:
		_, err = w.Write([]byte(prefix + currentIndent + k + " " + v + "\n"))
		if err != nil {
			return errors.New("cannot write: " + err.Error())
		}
	case []string:
		for _, vv := range v {
			_, err = w.Write([]byte(prefix + currentIndent + k + " " + vv + "\n"))
			if err != nil {
				return errors.New("cannot write: " + err.Error())
			}
		}
	case []*string:
		for _, vv := range v {
			_, err = w.Write([]byte(prefix + currentIndent + k + " " + *vv + "\n"))
			if err != nil {
				return errors.New("cannot write: " + err.Error())
			}
		}
	case Stanza:
		_, err = w.Write([]byte(prefix + currentIndent + k + " {\n"))
		if err != nil {
			return errors.New("cannot write: " + err.Error())
		}
		err = v.write(w, prefix, indent, currentIndent+indent, sortItems, top+"."+k)
		if err != nil {
			return
		}
		_, err = w.Write([]byte(prefix + currentIndent + "}\n"))
		if err != nil {
			return errors.New("cannot write: " + err.Error())
		}
	case nil:
		_, err = w.Write([]byte(prefix + currentIndent + k + "\n"))
		if err != nil {
			return errors.New("cannot write: " + err.Error())
		}
	default:
		return errors.New("map item interface is not of type string|stanza|nil")
	}
	return
}

func (s Stanza) Type(key string) ValueType {
	switch s[key].(type) {
	case string, []string, []*string:
		return ValueString
	case nil:
		return ValueNil
	case Stanza:
		return ValueStanza
	default:
		return ValueUnknown
	}
}

func (s Stanza) ListKeys() []string {
	keys := []string{}
	for key := range s {
		keys = append(keys, key)
	}
	return keys
}

func (s Stanza) Stanza(key string) Stanza {
	switch k := s[key].(type) {
	case Stanza:
		return k
	default:
		return nil
	}
}

func (s Stanza) GetValues(key string) ([]*string, error) {
	ret := []*string{}
	switch k := s[key].(type) {
	case string:
		ret = append(ret, &k)
	case []string:
		for _, kk := range k {
			ret = append(ret, &kk)
		}
	case []*string:
		ret = k
	case nil:
		return nil, nil
	case Stanza:
		return nil, errors.New("type is Stanza")
	default:
		return nil, errors.New("unknown type")
	}
	return ret, nil
}

func (s Stanza) SetValue(key string, value string) error {
	if s == nil {
		return errors.New("stanza does not exist")
	}
	s[key] = value
	return nil
}

func (s Stanza) SetValues(key string, values []*string) error {
	if s == nil {
		return errors.New("stanza does not exist")
	}
	s[key] = values
	return nil
}

func SliceToValues(val []string) []*string {
	r := []*string{}
	for _, v := range val {
		v := v
		r = append(r, &v)
	}
	return r
}

func (s Stanza) Delete(key string) error {
	if s == nil {
		return errors.New("stanza does not exist")
	}
	delete(s, key)
	return nil
}

func (s Stanza) NewStanza(key string) error {
	if s == nil {
		return errors.New("parent stanza does not exist")
	}
	if _, ok := s[key]; ok {
		return errors.New("stanza already exists")
	}
	s[key] = make(Stanza)
	return nil
}
