package firezone

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// The typed field constants and their operators live in
// posture_fields.go and posture_fields_gen.go. The constants are
// generated from the OpenAPI spec; regenerate them with `mise run
// generate` after refreshing testdata/openapi.json.
//go:generate go run ./internal/genposture

// PostureOperator is the comparison a posture leaf applies between its
// Field and Value. Which operators are valid depends on the type of the
// field - strings, booleans, numbers, versions, timestamps, IP addresses
// and lists each take their own set - and the API rejects a mismatch
// with a 422 naming the path in validation_errors.postures.
type PostureOperator string

// PostureOperator values.
const (
	// Any field accepts these two. They take no value.
	PostureOperatorExists       PostureOperator = "exists"
	PostureOperatorDoesNotExist PostureOperator = "does_not_exist"

	// Strings, booleans and versions.
	PostureOperatorIs    PostureOperator = "is"
	PostureOperatorIsNot PostureOperator = "is_not"

	// Strings. is_in and is_not_in take a list of strings, matches and
	// does_not_match a regular expression. Comparison ignores case.
	PostureOperatorIsIn           PostureOperator = "is_in"
	PostureOperatorIsNotIn        PostureOperator = "is_not_in"
	PostureOperatorContains       PostureOperator = "contains"
	PostureOperatorDoesNotContain PostureOperator = "does_not_contain"
	PostureOperatorStartsWith     PostureOperator = "starts_with"
	PostureOperatorEndsWith       PostureOperator = "ends_with"
	PostureOperatorMatches        PostureOperator = "matches"
	PostureOperatorDoesNotMatch   PostureOperator = "does_not_match"

	// Numbers.
	PostureOperatorEq PostureOperator = "eq"
	PostureOperatorNe PostureOperator = "ne"

	// Numbers and versions.
	PostureOperatorGt  PostureOperator = "gt"
	PostureOperatorGte PostureOperator = "gte"
	PostureOperatorLt  PostureOperator = "lt"
	PostureOperatorLte PostureOperator = "lte"

	// Timestamps. before and after take an ISO 8601 datetime;
	// within_last and not_within_last an ISO 8601 duration such as
	// "PT24H" or "P30D".
	PostureOperatorBefore        PostureOperator = "before"
	PostureOperatorAfter         PostureOperator = "after"
	PostureOperatorWithinLast    PostureOperator = "within_last"
	PostureOperatorNotWithinLast PostureOperator = "not_within_last"

	// IP addresses. Values are a list of CIDRs.
	PostureOperatorIsInCIDR    PostureOperator = "is_in_cidr"
	PostureOperatorIsNotInCIDR PostureOperator = "is_not_in_cidr"

	// Lists. is_empty and is_not_empty take no value.
	PostureOperatorContainsAnyOf PostureOperator = "contains_any_of"
	PostureOperatorContainsAllOf PostureOperator = "contains_all_of"
	PostureOperatorIsEmpty       PostureOperator = "is_empty"
	PostureOperatorIsNotEmpty    PostureOperator = "is_not_empty"
)

// PostureRows says how a leaf treats a device that matches more than
// one record of a provider.
type PostureRows string

// PostureRows values.
const (
	// PostureRowsAny holds when any matching record satisfies the leaf.
	// This is the API's default.
	PostureRowsAny PostureRows = "any"
	// PostureRowsAll holds only when every matching record does.
	PostureRowsAll PostureRows = "all"
)

// PostureNode is one node of a Policy's device posture expression: the
// checks a connecting device must pass, in addition to every Condition,
// before the Policy grants access.
//
// A node has exactly one of four shapes, and the constructors build
// each:
//
//   - [PostureAnd]: every child must hold.
//   - [PostureOr]: at least one child must hold.
//   - [PostureNot]: the child must not hold.
//   - [PostureCheck]: a leaf comparing a field against a value.
//
// Build leaves from the typed field constants, which allow only the
// operators the API accepts for each attribute:
//
//	firezone.PostureIntuneEnrolled.Is(true)
//	firezone.PostureIntuneComplianceState.Is("compliant").WithRows(firezone.PostureRowsAll)
//
// A leaf's Field is "<provider>.<attribute>", for example
// "intune.compliance_state" or "firezone.last_seen_version". The
// provider is one of firezone (the connecting device's own record),
// intune, iru, defender, santa, sentinelone or sophos. [PostureCheck]
// builds a leaf from a bare field name and operator, for an attribute
// newer than this SDK's constants; nothing then checks the pairing
// except the API, which answers 422 with the offending path.
//
// Value is whatever JSON the operator needs: a string, bool, number, or
// a []string for the list operators. It is nil for operators that take
// none. Marshalling rejects, before anything is sent, a node that is
// not exactly one of the four shapes. A non-null expression needs the
// account's device_posture entitlement, otherwise the API answers 403.
//
// On a node read back from the API, Value holds what encoding/json
// decodes into an any: numbers are float64 and lists are []any.
type PostureNode struct {
	// And, Or and Not make this a boolean node. And and Or must be
	// non-empty.
	And []PostureNode
	Or  []PostureNode
	Not *PostureNode

	// Field, Operator and Value make this a leaf.
	Field    string
	Operator PostureOperator
	Value    any
	// Rows applies to this leaf alone. The zero value leaves it to the
	// API, which treats it as [PostureRowsAny]. It is rejected for
	// "firezone" fields, which always describe exactly one device.
	Rows PostureRows
}

// PostureAnd returns a node that holds when every child holds.
func PostureAnd(children ...PostureNode) PostureNode { return PostureNode{And: children} }

// PostureOr returns a node that holds when at least one child holds.
func PostureOr(children ...PostureNode) PostureNode { return PostureNode{Or: children} }

// PostureNot returns a node that holds when child does not.
func PostureNot(child PostureNode) PostureNode { return PostureNode{Not: &child} }

// PostureCheck returns a leaf comparing field against value. Prefer the
// typed field constants, such as [PostureIntuneEnrolled], whose methods
// build the same leaf with the operator and value checked at compile
// time. Pass a nil value for operators that take none, such as
// [PostureOperatorExists].
func PostureCheck(field string, op PostureOperator, value any) PostureNode {
	return PostureNode{Field: field, Operator: op, Value: value}
}

// WithRows returns a copy of the leaf with Rows set. See [PostureRows].
func (n PostureNode) WithRows(rows PostureRows) PostureNode {
	n.Rows = rows
	return n
}

// errPostureShape is wrapped by the error MarshalJSON returns for a
// node that is not exactly one of the four shapes.
var errPostureShape = errors.New("posture node must be exactly one of and, or, not, or a field check")

// postureWire is the JSON shape of a node. The shapes are told apart by
// which key is present, so every field is omitted when empty.
type postureWire struct {
	And   []PostureNode   `json:"and,omitempty"`
	Or    []PostureNode   `json:"or,omitempty"`
	Not   *PostureNode    `json:"not,omitempty"`
	Field string          `json:"field,omitempty"`
	Op    PostureOperator `json:"op,omitempty"`
	Rows  PostureRows     `json:"rows,omitempty"`
	Value any             `json:"value,omitempty"`
}

// MarshalJSON implements [json.Marshaler]. It fails on a node that is
// not exactly one shape - empty, or mixing a leaf with and/or/not -
// rather than sending a body the API would reject for a less specific
// reason.
func (n PostureNode) MarshalJSON() ([]byte, error) {
	shapes := 0
	if len(n.And) > 0 {
		shapes++
	}
	if len(n.Or) > 0 {
		shapes++
	}
	if n.Not != nil {
		shapes++
	}
	if n.Field != "" || n.Operator != "" {
		shapes++
		if n.Field == "" || n.Operator == "" {
			return nil, fmt.Errorf("firezone: %w: a field check needs both a field and an operator", errPostureShape)
		}
	}
	if shapes != 1 {
		return nil, fmt.Errorf("firezone: %w (got %d)", errPostureShape, shapes)
	}
	if n.Rows != "" && n.Field == "" {
		return nil, fmt.Errorf("firezone: %w: rows applies only to a field check", errPostureShape)
	}
	if n.Rows != "" && strings.HasPrefix(n.Field, "firezone.") {
		return nil, fmt.Errorf("firezone: %w: rows does not apply to %q, which describes exactly one device", errPostureShape, n.Field)
	}
	return json.Marshal(postureWire{
		And: n.And, Or: n.Or, Not: n.Not,
		Field: n.Field, Op: n.Operator, Rows: n.Rows, Value: n.Value,
	})
}

// UnmarshalJSON implements [json.Unmarshaler].
func (n *PostureNode) UnmarshalJSON(data []byte) error {
	var w postureWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*n = PostureNode{
		And: w.And, Or: w.Or, Not: w.Not,
		Field: w.Field, Operator: w.Op, Rows: w.Rows, Value: w.Value,
	}
	return nil
}
