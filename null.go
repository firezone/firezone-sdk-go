package firezone

import "encoding/json"

// Null holds an optional, nullable field in an update request.
//
// The API's update endpoints are merge-patch: a field absent from the
// request body keeps its current value, while an explicit JSON null
// clears it. A plain Go string can't express both - its zero value is
// indistinguishable from "not set" - so nullable update fields are
// typed *Null[T], which has three states:
//
//	nil            field omitted; the server keeps its current value
//	Clear[T]()     field sent as JSON null; the server clears it
//	Set(v)         field sent as v
//
// Set("") also clears a nullable string field, rather than storing an
// empty one: the API's changeset treats "" as an empty value and
// replaces it with the field's default, which for a nullable field is
// null. Prefer [Clear] anyway - it says what it means, works for
// non-string types, and doesn't depend on that coincidence holding.
type Null[T any] struct {
	// Value is the value to send. Ignored unless Valid is true.
	Value T
	// Valid reports whether Value should be sent. When false, the field
	// is sent as JSON null.
	Valid bool
}

// Set returns a *Null that sends v.
func Set[T any](v T) *Null[T] { return &Null[T]{Value: v, Valid: true} }

// Clear returns a *Null that sends JSON null, clearing the field on the
// server. The type parameter is usually explicit, since there's no
// argument to infer it from: firezone.Clear[string]().
func Clear[T any]() *Null[T] { return &Null[T]{} }

// MarshalJSON implements [json.Marshaler], encoding an invalid Null as
// JSON null and a valid one as its value.
func (n Null[T]) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

// UnmarshalJSON implements [json.Unmarshaler]. A null decodes to the
// zero value with Valid false.
//
// Note that encoding/json sets a *Null field to nil on JSON null without
// calling this method, so decoding cannot tell a cleared field from an
// omitted one. Null is an encode-side type; the SDK never decodes a
// request body, and read models use plain fields.
func (n *Null[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		var zero T
		n.Value, n.Valid = zero, false
		return nil
	}
	if err := json.Unmarshal(data, &n.Value); err != nil {
		return err
	}
	n.Valid = true
	return nil
}

// Null's JSON methods split their receivers deliberately: MarshalJSON
// takes a value so a Null works whether it is addressable or not, while
// UnmarshalJSON needs a pointer to assign through. These assertions pin
// that choice down at build time - changing either receiver would stop
// encoding/json finding the method, and it would silently fall back to
// encoding the struct's fields rather than failing to compile.
var (
	_ json.Marshaler   = Null[string]{}
	_ json.Unmarshaler = (*Null[string])(nil)
)
