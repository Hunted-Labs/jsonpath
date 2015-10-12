package jsonpath

import (
	"encoding/json"
	"io"
)

// KeyString is returned from Decoder.Token() to represent each object key string.
type KeyString string

// Decoder extends the Go runtime's encoding/json.Decoder to support navigating in a stream of JSON tokens.
type Decoder struct {
	json.Decoder

	path    JsonPath
	context jsonContext
}

// NewDecoder creates a new instance of the extended JSON Decoder.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{Decoder: *json.NewDecoder(r)}
}

// SeekTo causes the Decoder to move forward to a given path in the JSON structure.
//
// The path argument must consist of strings or integers. Each string specifies an JSON object key, and
// each integer specifies an index into a JSON array.
//
// Consider the JSON structure
//
//  { "a": [0,"s",12e4,{"b":0,"v":35} ] }
//
// SeekTo("a",3,"v") will move to the value referenced by the "a" key in the current object,
// followed by a move to the 4th value (index 3) in the array, followed by a move to the value at key "v".
// In this example, a subsequent call to the decoder's Decode() would unmarshal the value 35.
//
// SeekTo returns a boolean value indicating whether a match was found.
//
// Decoder is intended to be used with a stream of tokens. As a result it navigates forward only.
func (w *Decoder) SeekTo(path ...interface{}) (bool, error) {

	if len(path) == 0 {
		return len(w.path) == 0, nil
	}
	last := len(path) - 1
	if i, ok := path[last].(int); ok {
		path[last] = i - 1
	}

	for {
		if w.path.Equal(path) {
			return true, nil
		}
		_, err := w.Token()
		if err == io.EOF {
			return false, nil
		} else if err != nil {
			return false, err
		}
	}
}

// Decode reads the next JSON-encoded value from its input and stores it in the value pointed to by v. This is
// equivalent to encoding/json.Decode().
func (d *Decoder) Decode(v interface{}) error {
	switch d.context {
	case objValue:
		d.context = objKey
		break
	case arrValue:
		d.path.incTop()
		break
	}
	return d.Decoder.Decode(v)
}

// Path returns a slice of string and/or int values representing the path from the root of the JSON object to the
// position of the most-recently parsed token.
func (d *Decoder) Path() JsonPath {
	p := make(JsonPath, len(d.path))
	copy(p, d.path)
	return p
}

// Token is equivalent to the Token() method on json.Decoder. The primary difference is that it distinguishes
// between strings that are keys and and strings that are values. String tokens that are object keys are returned as a
// KeyString rather than as a native string.
func (d *Decoder) Token() (json.Token, error) {
	t, err := d.Decoder.Token()
	if err != nil {
		return t, err
	}

	if t == nil {
		switch d.context {
		case objValue:
			d.context = objKey
			break
		case arrValue:
			d.path.incTop()
			break
		}
		return t, err
	}

	switch t := t.(type) {
	case json.Delim:
		switch t {
		case json.Delim('{'):
			if d.context == arrValue {
				d.path.incTop()
			}
			d.path.push("")
			d.context = objKey
			break
		case json.Delim('}'):
			d.path.pop()
			d.context = d.path.inferContext()
			break
		case json.Delim('['):
			if d.context == arrValue {
				d.path.incTop()
			}
			d.path.push(-1)
			d.context = arrValue
			break
		case json.Delim(']'):
			d.path.pop()
			d.context = d.path.inferContext()
			break
		}
	case float64, json.Number, bool:
		switch d.context {
		case objValue:
			d.context = objKey
			break
		case arrValue:
			d.path.incTop()
			break
		}
		break
	case string:
		switch d.context {
		case objKey:
			d.path.nameTop(t)
			d.context = objValue
			return KeyString(t), err
		case objValue:
			d.context = objKey
		case arrValue:
			d.path.incTop()
		}
		break
	}

	return t, err
}

func (d *Decoder) Scan(ext *PathActions) (bool, error) {

	matched := false
	rootPath := d.Path()
	if rootPath.inferContext() == arrValue {
		rootPath.incTop()
	}

	for {
		_, err := d.Token()
		if err != nil {
			return matched, err
		}

	match:
		path := d.Path()
		relPath := JsonPath{}

		// fmt.Printf("rootPath: %v path: %v rel: %v\n", rootPath, path, relPath)

		if len(path) > len(rootPath) {
			relPath = path[len(rootPath):]
		} else {
			return matched, nil
		}

		if node, ok := ext.node.match(relPath); ok {
			if node.action != nil {
				matched = true
				node.action(d)
				if d.context == arrValue && d.Decoder.More() {
					goto match
				}
			}
		}
	}
}
