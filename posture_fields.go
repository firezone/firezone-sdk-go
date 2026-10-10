package firezone

import (
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// The field types below are what the generated constants in
// posture_fields_gen.go are typed as. Each carries exactly the operators
// the API accepts for that kind of attribute, as methods returning a
// [PostureNode], so a comparison the API would reject - an ordering
// operator on a string, a list for a boolean - does not compile:
//
//	firezone.PostureIntuneEnrolled.Is(true)
//	firezone.PostureIntuneComplianceState.IsIn("compliant", "inGracePeriod")
//	firezone.PostureFirezoneLastSeenVersion.Gte(firezone.PostureVersionLatest)
//	firezone.PostureIntuneLastSyncAt.WithinLast(24 * time.Hour)
//
// Every field also has Exists and DoesNotExist, which take no value. A
// missing attribute fails every operator except DoesNotExist - including
// negative ones such as IsNot - so test for the attribute first when
// that matters.
//
// The operator sets are checked against the API's OpenAPI spec by the
// spec tests, so they cannot drift from what the server accepts. What
// the types cannot express is checked by the server and reported as a
// 422: regular expression syntax, version and CIDR validity, and the
// length limits (list values hold 1 to 100 items, strings at most 1024
// bytes, regular expressions at most 256).

func fieldCheck(field string, op PostureOperator, value any) PostureNode {
	return PostureCheck(field, op, value)
}

// PostureVersionLatest is the value that stands for the newest Client
// release for the device's platform. The API accepts it only for
// [PostureFirezoneLastSeenVersion].
const PostureVersionLatest = "@latest"

// PostureBoolField is a boolean attribute.
type PostureBoolField string

// Is holds when the attribute equals v.
func (f PostureBoolField) Is(v bool) PostureNode { return fieldCheck(string(f), PostureOperatorIs, v) }

// Exists holds when the device has the attribute.
func (f PostureBoolField) Exists() PostureNode {
	return fieldCheck(string(f), PostureOperatorExists, nil)
}

// DoesNotExist holds when the device lacks the attribute.
func (f PostureBoolField) DoesNotExist() PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotExist, nil)
}

// PostureStringField is a string attribute, including those the API
// types as enum_string, which take the same operators. String
// comparisons ignore case.
type PostureStringField string

// Is holds when the attribute equals v.
func (f PostureStringField) Is(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorIs, v)
}

// IsNot holds when the attribute does not equal v.
func (f PostureStringField) IsNot(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorIsNot, v)
}

// Contains holds when the attribute contains v.
func (f PostureStringField) Contains(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorContains, v)
}

// DoesNotContain holds when the attribute does not contain v.
func (f PostureStringField) DoesNotContain(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotContain, v)
}

// StartsWith holds when the attribute starts with v.
func (f PostureStringField) StartsWith(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorStartsWith, v)
}

// EndsWith holds when the attribute ends with v.
func (f PostureStringField) EndsWith(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorEndsWith, v)
}

// IsIn holds when the attribute equals any of values.
func (f PostureStringField) IsIn(values ...string) PostureNode {
	return fieldCheck(string(f), PostureOperatorIsIn, nonNil(values))
}

// IsNotIn holds when the attribute equals none of values.
func (f PostureStringField) IsNotIn(values ...string) PostureNode {
	return fieldCheck(string(f), PostureOperatorIsNotIn, nonNil(values))
}

// Matches holds when the attribute matches the regular expression re.
func (f PostureStringField) Matches(re string) PostureNode {
	return fieldCheck(string(f), PostureOperatorMatches, re)
}

// DoesNotMatch holds when the attribute does not match the regular
// expression re.
func (f PostureStringField) DoesNotMatch(re string) PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotMatch, re)
}

// Exists holds when the device has the attribute.
func (f PostureStringField) Exists() PostureNode {
	return fieldCheck(string(f), PostureOperatorExists, nil)
}

// DoesNotExist holds when the device lacks the attribute.
func (f PostureStringField) DoesNotExist() PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotExist, nil)
}

// PostureIntegerField is an integer attribute.
type PostureIntegerField string

// Eq holds when the attribute equals v.
func (f PostureIntegerField) Eq(v int64) PostureNode {
	return fieldCheck(string(f), PostureOperatorEq, v)
}

// Ne holds when the attribute does not equal v.
func (f PostureIntegerField) Ne(v int64) PostureNode {
	return fieldCheck(string(f), PostureOperatorNe, v)
}

// Gt holds when the attribute is greater than v.
func (f PostureIntegerField) Gt(v int64) PostureNode {
	return fieldCheck(string(f), PostureOperatorGt, v)
}

// Gte holds when the attribute is greater than or equal to v.
func (f PostureIntegerField) Gte(v int64) PostureNode {
	return fieldCheck(string(f), PostureOperatorGte, v)
}

// Lt holds when the attribute is less than v.
func (f PostureIntegerField) Lt(v int64) PostureNode {
	return fieldCheck(string(f), PostureOperatorLt, v)
}

// Lte holds when the attribute is less than or equal to v.
func (f PostureIntegerField) Lte(v int64) PostureNode {
	return fieldCheck(string(f), PostureOperatorLte, v)
}

// Exists holds when the device has the attribute.
func (f PostureIntegerField) Exists() PostureNode {
	return fieldCheck(string(f), PostureOperatorExists, nil)
}

// DoesNotExist holds when the device lacks the attribute.
func (f PostureIntegerField) DoesNotExist() PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotExist, nil)
}

// PostureFloatField is a floating-point attribute.
type PostureFloatField string

// Eq holds when the attribute equals v.
func (f PostureFloatField) Eq(v float64) PostureNode {
	return fieldCheck(string(f), PostureOperatorEq, v)
}

// Ne holds when the attribute does not equal v.
func (f PostureFloatField) Ne(v float64) PostureNode {
	return fieldCheck(string(f), PostureOperatorNe, v)
}

// Gt holds when the attribute is greater than v.
func (f PostureFloatField) Gt(v float64) PostureNode {
	return fieldCheck(string(f), PostureOperatorGt, v)
}

// Gte holds when the attribute is greater than or equal to v.
func (f PostureFloatField) Gte(v float64) PostureNode {
	return fieldCheck(string(f), PostureOperatorGte, v)
}

// Lt holds when the attribute is less than v.
func (f PostureFloatField) Lt(v float64) PostureNode {
	return fieldCheck(string(f), PostureOperatorLt, v)
}

// Lte holds when the attribute is less than or equal to v.
func (f PostureFloatField) Lte(v float64) PostureNode {
	return fieldCheck(string(f), PostureOperatorLte, v)
}

// Exists holds when the device has the attribute.
func (f PostureFloatField) Exists() PostureNode {
	return fieldCheck(string(f), PostureOperatorExists, nil)
}

// DoesNotExist holds when the device lacks the attribute.
func (f PostureFloatField) DoesNotExist() PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotExist, nil)
}

// PostureVersionField is a version attribute. Versions compare segment
// by segment, so "14.4" equals "14.4.0".
type PostureVersionField string

// Is holds when the attribute equals v.
func (f PostureVersionField) Is(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorIs, v)
}

// IsNot holds when the attribute does not equal v.
func (f PostureVersionField) IsNot(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorIsNot, v)
}

// Gt holds when the attribute is newer than v.
func (f PostureVersionField) Gt(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorGt, v)
}

// Gte holds when the attribute is v or newer.
func (f PostureVersionField) Gte(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorGte, v)
}

// Lt holds when the attribute is older than v.
func (f PostureVersionField) Lt(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorLt, v)
}

// Lte holds when the attribute is v or older.
func (f PostureVersionField) Lte(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorLte, v)
}

// Exists holds when the device has the attribute.
func (f PostureVersionField) Exists() PostureNode {
	return fieldCheck(string(f), PostureOperatorExists, nil)
}

// DoesNotExist holds when the device lacks the attribute.
func (f PostureVersionField) DoesNotExist() PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotExist, nil)
}

// PostureTimestampField is a timestamp attribute. A date-only attribute
// counts as the start of that day in UTC.
type PostureTimestampField string

// Before holds when the attribute is earlier than t.
func (f PostureTimestampField) Before(t time.Time) PostureNode {
	return fieldCheck(string(f), PostureOperatorBefore, t.UTC().Format(time.RFC3339))
}

// After holds when the attribute is later than t.
func (f PostureTimestampField) After(t time.Time) PostureNode {
	return fieldCheck(string(f), PostureOperatorAfter, t.UTC().Format(time.RFC3339))
}

// WithinLast holds when the attribute is no older than d. d must be
// positive; the API rejects anything else with a 422.
func (f PostureTimestampField) WithinLast(d time.Duration) PostureNode {
	return fieldCheck(string(f), PostureOperatorWithinLast, isoDuration(d))
}

// NotWithinLast holds when the attribute is older than d. d must be
// positive; the API rejects anything else with a 422.
func (f PostureTimestampField) NotWithinLast(d time.Duration) PostureNode {
	return fieldCheck(string(f), PostureOperatorNotWithinLast, isoDuration(d))
}

// Exists holds when the device has the attribute.
func (f PostureTimestampField) Exists() PostureNode {
	return fieldCheck(string(f), PostureOperatorExists, nil)
}

// DoesNotExist holds when the device lacks the attribute.
func (f PostureTimestampField) DoesNotExist() PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotExist, nil)
}

// isoDuration formats d as an ISO 8601 duration, for example "PT24H".
// It uses only time units, since a Go Duration has no calendar days or
// months. A non-positive d becomes "PT0S", which the API rejects.
func isoDuration(d time.Duration) string {
	if d <= 0 {
		return "PT0S"
	}
	var b strings.Builder
	b.WriteString("PT")
	if h := d / time.Hour; h > 0 {
		b.WriteString(strconv.FormatInt(int64(h), 10) + "H")
		d -= h * time.Hour
	}
	if m := d / time.Minute; m > 0 {
		b.WriteString(strconv.FormatInt(int64(m), 10) + "M")
		d -= m * time.Minute
	}
	if d > 0 {
		b.WriteString(strconv.FormatFloat(d.Seconds(), 'f', -1, 64) + "S")
	}
	return b.String()
}

// PostureIPField is an IP address attribute. [PostureFirezoneIPv4]
// takes IPv4 prefixes only and [PostureFirezoneIPv6] IPv6 only.
type PostureIPField string

// InCIDR holds when the address is inside any of prefixes.
func (f PostureIPField) InCIDR(prefixes ...netip.Prefix) PostureNode {
	return fieldCheck(string(f), PostureOperatorIsInCIDR, prefixStrings(prefixes))
}

// NotInCIDR holds when the address is inside none of prefixes.
func (f PostureIPField) NotInCIDR(prefixes ...netip.Prefix) PostureNode {
	return fieldCheck(string(f), PostureOperatorIsNotInCIDR, prefixStrings(prefixes))
}

// Exists holds when the device has the attribute.
func (f PostureIPField) Exists() PostureNode {
	return fieldCheck(string(f), PostureOperatorExists, nil)
}

// DoesNotExist holds when the device lacks the attribute.
func (f PostureIPField) DoesNotExist() PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotExist, nil)
}

// PostureJSONField is a structured attribute. Only its presence and
// emptiness can be tested.
type PostureJSONField string

// IsEmpty holds when the attribute is empty.
func (f PostureJSONField) IsEmpty() PostureNode {
	return fieldCheck(string(f), PostureOperatorIsEmpty, nil)
}

// IsNotEmpty holds when the attribute is not empty.
func (f PostureJSONField) IsNotEmpty() PostureNode {
	return fieldCheck(string(f), PostureOperatorIsNotEmpty, nil)
}

// Exists holds when the device has the attribute.
func (f PostureJSONField) Exists() PostureNode {
	return fieldCheck(string(f), PostureOperatorExists, nil)
}

// DoesNotExist holds when the device lacks the attribute.
func (f PostureJSONField) DoesNotExist() PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotExist, nil)
}

// PostureStringArrayField is an attribute holding a list of strings.
type PostureStringArrayField string

// Contains holds when the list contains v.
func (f PostureStringArrayField) Contains(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorContains, v)
}

// DoesNotContain holds when the list does not contain v.
func (f PostureStringArrayField) DoesNotContain(v string) PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotContain, v)
}

// ContainsAnyOf holds when the list contains at least one of values.
func (f PostureStringArrayField) ContainsAnyOf(values ...string) PostureNode {
	return fieldCheck(string(f), PostureOperatorContainsAnyOf, nonNil(values))
}

// ContainsAllOf holds when the list contains every one of values.
func (f PostureStringArrayField) ContainsAllOf(values ...string) PostureNode {
	return fieldCheck(string(f), PostureOperatorContainsAllOf, nonNil(values))
}

// IsEmpty holds when the list is empty.
func (f PostureStringArrayField) IsEmpty() PostureNode {
	return fieldCheck(string(f), PostureOperatorIsEmpty, nil)
}

// IsNotEmpty holds when the list is not empty.
func (f PostureStringArrayField) IsNotEmpty() PostureNode {
	return fieldCheck(string(f), PostureOperatorIsNotEmpty, nil)
}

// Exists holds when the device has the attribute.
func (f PostureStringArrayField) Exists() PostureNode {
	return fieldCheck(string(f), PostureOperatorExists, nil)
}

// DoesNotExist holds when the device lacks the attribute.
func (f PostureStringArrayField) DoesNotExist() PostureNode {
	return fieldCheck(string(f), PostureOperatorDoesNotExist, nil)
}

// nonNil keeps an empty variadic list encoding as [] rather than null,
// so the API reports it as an empty list instead of a missing value.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func prefixStrings(prefixes []netip.Prefix) []string {
	out := make([]string, len(prefixes))
	for i, p := range prefixes {
		out[i] = p.String()
	}
	return out
}
