package cache

import (
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests define the intended behaviour of parse(). The parser is not
// implemented yet, so this file is the specification.
//
// SPEC DECISIONS — each is exercised by at least one subtest tagged with its
// number. Flip a decision here and the tagged subtests tell you what to change.
//
//	D1  An empty or whitespace-only header value is an error. The "header not
//	    present" case is handled one level up in ParseCacheControlResponse,
//	    which returns (nil, nil).
//	D2  Directive names are case-insensitive (RFC 9110 tokens).
//	D3  Directives that the CacheControlResponse struct does not model
//	    (must-revalidate, no-transform, proxy-revalidate, immutable,
//	    stale-while-revalidate, arbitrary cache extensions) are ignored, not
//	    rejected. RFC 9111 §5.2: unknown directives MUST be ignored. A header
//	    consisting only of such directives still parses to a non-nil, zero
//	    CacheControlResponse.
//	D4  A boolean directive (no-store, public) carrying any argument is an
//	    error. `no-store=true` and `no-store="true"` both fail.
//	D5  A directive requiring an argument (max-age, s-maxage) with no argument,
//	    or an empty argument, is an error.
//	D6  delta-seconds is 1*DIGIT. No sign, no decimal point, no quotes, no
//	    underscores. `max-age="60"` is an error. Leading zeros are fine.
//	D7  delta-seconds above MaxInt32 clamps to MaxInt32 (RFC 9111 §1.2.2),
//	    including values that overflow int64. Note that parseDeltaSeconds
//	    currently returns an error from strconv for the int64-overflow case —
//	    the caller must treat strconv.ErrRange as "clamp", not "reject".
//	D8  no-cache / private field-name arguments MUST be a quoted-string.
//	    `no-cache=Set-Cookie` (bare token) is an error. Consequence:
//	    `no-cache="true"` is a *valid* one-element field list, not an error.
//	    NOTE: RFC 9111 §5.2 advises recipients to accept the token form too.
//	    See the "D8 is stricter than the RFC" subtests below.
//	D9  Field names are HTTP field names and therefore case-insensitive; they
//	    are normalised to lowercase when stored.
//	D10 Inside the quoted-string, elements are comma-separated with optional
//	    whitespace, and empty elements are skipped (RFC 9110 §5.6.1 legacy list
//	    rule). Each non-empty element must be a valid token.
//	D11 `no-cache=""` and `no-cache="   "` mean Present with no field names —
//	    identical to bare `no-cache`.
//	D12 Commas inside a quoted-string do not terminate the directive.
//	D13 Empty elements in the top-level directive list are skipped
//	    (leading/trailing/double commas are tolerated), but a list that yields
//	    zero directives is an error.
//	D14 Duplicate scalar directives with equal values are fine; with conflicting
//	    values they are an error.
//	D15 Duplicate field-list directives are merged (union), not rejected.
//	D16 The parser records what it read and does not resolve policy conflicts.
//	    `public, private` and `no-store, max-age=60` both parse without error.
//	D17 ASCII control characters and non-ASCII bytes anywhere in the input are
//	    an error.
//	D18 Quoted-pair escaping (`\"`, `\\`) is not accepted inside the field-name
//	    list, because the resulting characters are not valid token characters
//	    anyway.

func TestParse(t *testing.T) {
	t.Parallel()

	// --- D1: empty and whitespace-only input --------------------------------

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "")
	})

	t.Run("single space", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, " ")
	})

	t.Run("single tab", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "\t")
	})

	t.Run("mixed whitespace only", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "  \t \r\n ")
	})

	// --- single boolean directives ------------------------------------------

	t.Run("no-store", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-store", &CacheControlResponse{NoStore: true})
	})

	t.Run("public", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "public", &CacheControlResponse{Public: true})
	})

	t.Run("no-cache without field names", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-cache", &CacheControlResponse{NoCache: fieldNames()})
	})

	t.Run("private without field names", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "private", &CacheControlResponse{Private: fieldNames()})
	})

	// --- D3: unmodelled directives are ignored ------------------------------

	t.Run("must-revalidate alone is ignored but still parses", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "must-revalidate", &CacheControlResponse{})
	})

	t.Run("no-transform is ignored", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-transform", &CacheControlResponse{})
	})

	t.Run("proxy-revalidate is ignored", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "proxy-revalidate", &CacheControlResponse{})
	})

	t.Run("immutable is ignored", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "immutable", &CacheControlResponse{})
	})

	t.Run("stale-while-revalidate is ignored", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60, stale-while-revalidate=30", &CacheControlResponse{MaxAge: 60})
	})

	t.Run("stale-if-error is ignored", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60, stale-if-error=120", &CacheControlResponse{MaxAge: 60})
	})

	t.Run("unknown extension with quoted argument is ignored", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `community="UCI", max-age=60`, &CacheControlResponse{MaxAge: 60})
	})

	t.Run("unknown extension with comma inside quotes is ignored without breaking the list", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `community="UCI, UCLA", no-store`, &CacheControlResponse{NoStore: true})
	})

	t.Run("unknown bare extension is ignored", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60, ext", &CacheControlResponse{MaxAge: 60})
	})

	// --- D2: case insensitivity of directive names --------------------------

	t.Run("NO-STORE uppercase", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "NO-STORE", &CacheControlResponse{NoStore: true})
	})

	t.Run("PuBlIc mixed case", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "PuBlIc", &CacheControlResponse{Public: true})
	})

	t.Run("PRIVATE uppercase", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "PRIVATE", &CacheControlResponse{Private: fieldNames()})
	})

	t.Run("No-Cache mixed case", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "No-Cache", &CacheControlResponse{NoCache: fieldNames()})
	})

	t.Run("MAX-AGE uppercase", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "MAX-AGE=60", &CacheControlResponse{MaxAge: 60})
	})

	t.Run("S-MaxAge mixed case", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "S-MaxAge=60", &CacheControlResponse{SMaxAge: 60})
	})

	// --- delta-seconds, valid -----------------------------------------------

	t.Run("max-age zero", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=0", &CacheControlResponse{MaxAge: 0})
	})

	t.Run("max-age", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60", &CacheControlResponse{MaxAge: 60})
	})

	t.Run("s-maxage", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "s-maxage=60", &CacheControlResponse{SMaxAge: 60})
	})

	t.Run("max-age at MaxInt32", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=2147483647", &CacheControlResponse{MaxAge: math.MaxInt32})
	})

	// D7
	t.Run("max-age above MaxInt32 clamps", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=2147483648", &CacheControlResponse{MaxAge: math.MaxInt32})
	})

	// D7 — this one overflows int64 and makes strconv return ErrRange.
	t.Run("max-age above MaxInt64 clamps", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=99999999999999999999999999", &CacheControlResponse{MaxAge: math.MaxInt32})
	})

	t.Run("s-maxage above MaxInt32 clamps", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "s-maxage=4294967296", &CacheControlResponse{SMaxAge: math.MaxInt32})
	})

	t.Run("leading zeros", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=007", &CacheControlResponse{MaxAge: 7})
	})

	t.Run("all zeros", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=00000", &CacheControlResponse{MaxAge: 0})
	})

	// --- D5/D6: delta-seconds, invalid --------------------------------------

	t.Run("max-age with no argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age")
	})

	t.Run("s-maxage with no argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "s-maxage")
	})

	t.Run("max-age with empty argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=")
	})

	t.Run("s-maxage with empty argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "s-maxage=")
	})

	t.Run("max-age with quotes", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `max-age="60"`)
	})

	t.Run("s-maxage with quotes", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `s-maxage="60"`)
	})

	t.Run("max-age non-numeric", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=abc")
	})

	t.Run("max-age float", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=60.5")
	})

	t.Run("max-age negative", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=-1")
	})

	t.Run("s-maxage negative", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "s-maxage=-100")
	})

	t.Run("max-age explicit plus sign", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=+60")
	})

	t.Run("max-age with underscore separator", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=1_000")
	})

	t.Run("max-age with internal space", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=6 0")
	})

	t.Run("max-age hex", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=0x3c")
	})

	t.Run("max-age with trailing unit", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=60s")
	})

	// --- D4: boolean directives must not carry arguments --------------------

	t.Run("no-store with token argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "no-store=true")
	})

	t.Run("no-store with quoted argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-store="true"`)
	})

	t.Run("public with token argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "public=true")
	})

	t.Run("public with quoted argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `public="true"`)
	})

	t.Run("no-store with empty argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "no-store=")
	})

	// --- D8: field-list directives require a quoted-string ------------------
	// These two are stricter than RFC 9111 §5.2, which advises recipients to
	// accept the token form as a single-element list. Flip them if you decide
	// to be lenient.

	t.Run("no-cache with bare token argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "no-cache=Set-Cookie")
	})

	t.Run("private with bare token argument", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "private=Set-Cookie")
	})

	// D8 consequence: this is a valid one-element field list, NOT an error.
	t.Run("no-cache with quoted true is a field name", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="true"`, &CacheControlResponse{NoCache: fieldNames("true")})
	})

	t.Run("private with quoted true is a field name", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="true"`, &CacheControlResponse{Private: fieldNames("true")})
	})

	// --- field-name lists ----------------------------------------------------

	t.Run("no-cache with one field name", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie"`, &CacheControlResponse{NoCache: fieldNames("set-cookie")})
	})

	t.Run("private with one field name", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="Set-Cookie"`, &CacheControlResponse{Private: fieldNames("set-cookie")})
	})

	t.Run("no-cache with two field names", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie, Authorization"`, &CacheControlResponse{
			NoCache: fieldNames("set-cookie", "authorization"),
		})
	})

	t.Run("private with three field names and no whitespace", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="Set-Cookie,Authorization,X-Custom"`, &CacheControlResponse{
			Private: fieldNames("set-cookie", "authorization", "x-custom"),
		})
	})

	t.Run("field names with surrounding whitespace are trimmed", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="  Set-Cookie ,  Authorization  "`, &CacheControlResponse{
			Private: fieldNames("set-cookie", "authorization"),
		})
	})

	t.Run("field names with tabs are trimmed", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "private=\"\tSet-Cookie\t,\tAuthorization\t\"", &CacheControlResponse{
			Private: fieldNames("set-cookie", "authorization"),
		})
	})

	// D9
	t.Run("field names are lowercased", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="SET-COOKIE, AuThOrIzAtIoN"`, &CacheControlResponse{
			NoCache: fieldNames("set-cookie", "authorization"),
		})
	})

	// D9
	t.Run("field names differing only in case collapse to one entry", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie, set-cookie, SET-COOKIE"`, &CacheControlResponse{
			NoCache: fieldNames("set-cookie"),
		})
	})

	// D11
	t.Run("no-cache with empty quoted string", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache=""`, &CacheControlResponse{NoCache: fieldNames()})
	})

	// D11
	t.Run("private with whitespace-only quoted string", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="   "`, &CacheControlResponse{Private: fieldNames()})
	})

	// D10
	t.Run("empty elements inside the field list are skipped", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie,,Authorization"`, &CacheControlResponse{
			NoCache: fieldNames("set-cookie", "authorization"),
		})
	})

	// D10
	t.Run("leading and trailing commas inside the field list are skipped", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache=",Set-Cookie,"`, &CacheControlResponse{NoCache: fieldNames("set-cookie")})
	})

	t.Run("field names containing all token special characters", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="X-Foo!#$%&'*+.^_`+"`"+`|~0"`, &CacheControlResponse{
			NoCache: fieldNames(`x-foo!#$%&'*+.^_` + "`" + `|~0`),
		})
	})

	t.Run("field name with internal space", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="Set Cookie"`)
	})

	t.Run("field name with semicolon", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="Set-Cookie;"`)
	})

	t.Run("field name with colon", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="Set-Cookie:"`)
	})

	t.Run("field name with slash", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="Set/Cookie"`)
	})

	t.Run("field name with parentheses", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="(Set-Cookie)"`)
	})

	t.Run("field name with equals", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="Set=Cookie"`)
	})

	// --- quoting edge cases --------------------------------------------------

	t.Run("unterminated quote", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="Set-Cookie`)
	})

	t.Run("unterminated empty quote", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="`)
	})

	t.Run("stray closing quote", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache=Set-Cookie"`)
	})

	t.Run("single quotes are not quoting", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache='Set-Cookie'`)
	})

	t.Run("doubled quotes", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache=""Set-Cookie""`)
	})

	// D18
	t.Run("escaped quote inside the field list", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="a\"b"`)
	})

	// D18
	t.Run("escaped backslash inside the field list", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="a\\b"`)
	})

	t.Run("trailing content after closing quote", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="Set-Cookie"x`)
	})

	// --- multiple directives -------------------------------------------------

	t.Run("max-age and no-store", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60, no-store", &CacheControlResponse{MaxAge: 60, NoStore: true})
	})

	t.Run("no space after comma", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60,no-store", &CacheControlResponse{MaxAge: 60, NoStore: true})
	})

	t.Run("whitespace on both sides of the comma", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60 ,  no-store", &CacheControlResponse{MaxAge: 60, NoStore: true})
	})

	t.Run("tabs around the comma", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "\tmax-age=60\t,\tno-store\t", &CacheControlResponse{MaxAge: 60, NoStore: true})
	})

	t.Run("leading and trailing whitespace around the whole value", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "   max-age=60, no-store   ", &CacheControlResponse{MaxAge: 60, NoStore: true})
	})

	t.Run("public with max-age and s-maxage", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "public, max-age=3600, s-maxage=7200", &CacheControlResponse{
			Public: true, MaxAge: 3600, SMaxAge: 7200,
		})
	})

	t.Run("typical no-cache combination", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-cache, no-store, must-revalidate, max-age=0", &CacheControlResponse{
			NoCache: fieldNames(), NoStore: true, MaxAge: 0,
		})
	})

	t.Run("private field list alongside max-age", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="Set-Cookie", max-age=60`, &CacheControlResponse{
			Private: fieldNames("set-cookie"), MaxAge: 60,
		})
	})

	// D12
	t.Run("comma inside quotes does not terminate the directive", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie, Authorization", max-age=60`, &CacheControlResponse{
			NoCache: fieldNames("set-cookie", "authorization"),
			MaxAge:  60,
		})
	})

	// D12
	t.Run("two quoted field lists in one header", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="A, B", private="C, D"`, &CacheControlResponse{
			NoCache: fieldNames("a", "b"),
			Private: fieldNames("c", "d"),
		})
	})

	t.Run("all modelled directives at once", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `max-age=1, s-maxage=2, no-store, no-cache="A", public, private="B"`, &CacheControlResponse{
			MaxAge:  1,
			SMaxAge: 2,
			NoStore: true,
			NoCache: fieldNames("a"),
			Public:  true,
			Private: fieldNames("b"),
		})
	})

	// --- D13: list structure -------------------------------------------------

	t.Run("trailing comma is tolerated", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60,", &CacheControlResponse{MaxAge: 60})
	})

	t.Run("leading comma is tolerated", func(t *testing.T) {
		t.Parallel()
		requireParses(t, ",max-age=60", &CacheControlResponse{MaxAge: 60})
	})

	t.Run("double comma is tolerated", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60,,no-store", &CacheControlResponse{MaxAge: 60, NoStore: true})
	})

	t.Run("only a comma", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, ",")
	})

	t.Run("only commas", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, ",,,")
	})

	t.Run("only commas and whitespace", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, " , , ")
	})

	t.Run("only an equals sign", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "=")
	})

	t.Run("argument with no directive name", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "=60")
	})

	t.Run("double equals", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age==60")
	})

	t.Run("space inside directive name", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max age=60")
	})

	t.Run("space before equals", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age =60")
	})

	t.Run("space after equals", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age= 60")
	})

	t.Run("directive name with slash", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max/age=60")
	})

	t.Run("quoted directive name", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `"max-age"=60`)
	})

	// --- D14/D15: duplicates --------------------------------------------------

	t.Run("duplicate max-age with equal values", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60, max-age=60", &CacheControlResponse{MaxAge: 60})
	})

	t.Run("duplicate boolean directive", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-store, no-store", &CacheControlResponse{NoStore: true})
	})

	t.Run("duplicate max-age with conflicting values", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=60, max-age=120")
	})

	t.Run("duplicate s-maxage with conflicting values", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "s-maxage=1, s-maxage=2")
	})

	// D14 — clamping must happen before the comparison.
	t.Run("duplicate max-age that both clamp to MaxInt32", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=2147483648, max-age=4294967296", &CacheControlResponse{MaxAge: math.MaxInt32})
	})

	// D15
	t.Run("duplicate no-cache field lists are merged", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="A", no-cache="B"`, &CacheControlResponse{NoCache: fieldNames("a", "b")})
	})

	// D15
	t.Run("bare private followed by a private field list", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private, private="Set-Cookie"`, &CacheControlResponse{Private: fieldNames("set-cookie")})
	})

	// D15
	t.Run("private field list followed by bare private stays present", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="Set-Cookie", private`, &CacheControlResponse{Private: fieldNames("set-cookie")})
	})

	// --- D16: the parser records, it does not resolve -------------------------

	t.Run("public and private together", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "public, private", &CacheControlResponse{Public: true, Private: fieldNames()})
	})

	t.Run("no-store with a positive max-age", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-store, max-age=60", &CacheControlResponse{NoStore: true, MaxAge: 60})
	})

	t.Run("no-cache with a positive max-age", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-cache, max-age=3600", &CacheControlResponse{NoCache: fieldNames(), MaxAge: 3600})
	})

	// --- D17: control characters and non-ASCII --------------------------------

	t.Run("NUL byte", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=60\x00")
	})

	t.Run("control character inside quotes", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "no-cache=\"a\x01b\"")
	})

	t.Run("DEL byte", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=60\x7f")
	})

	t.Run("bare line feed", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=60\nno-store")
	})

	t.Run("CRLF header folding", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=60\r\n no-store")
	})

	t.Run("vertical tab", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=60\vno-store")
	})

	t.Run("non-ASCII in directive name", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-agé=60")
	})

	t.Run("non-ASCII in field name", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="Sét-Cookie"`)
	})

	t.Run("non-ASCII alone", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "🚀")
	})

	// --- robustness -----------------------------------------------------------

	t.Run("many repeated ignorable directives", func(t *testing.T) {
		t.Parallel()

		input := strings.TrimSuffix(strings.Repeat("must-revalidate, ", 1000), ", ") + ", max-age=60"
		requireParses(t, input, &CacheControlResponse{MaxAge: 60})
	})

	t.Run("very long field list", func(t *testing.T) {
		t.Parallel()

		input := `no-cache="` + strings.TrimSuffix(strings.Repeat("x-a,", 500), ",") + `"`
		requireParses(t, input, &CacheControlResponse{NoCache: fieldNames("x-a")})
	})

	t.Run("very long run of leading zeros", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age="+strings.Repeat("0", 100)+"60", &CacheControlResponse{MaxAge: 60})
	})
}

func TestParseCacheControlResponse(t *testing.T) {
	t.Parallel()

	t.Run("nil headers", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t, nil, nil)
	})

	t.Run("empty headers", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t, http.Header{}, nil)
	})

	t.Run("no Cache-Control header", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t, http.Header{"Content-Type": []string{"application/json"}}, nil)
	})

	t.Run("Cache-Control present but empty", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t, http.Header{"Cache-Control": []string{""}}, nil)
	})

	t.Run("Cache-Control whitespace only", func(t *testing.T) {
		t.Parallel()
		requireHeaderParseError(t, http.Header{"Cache-Control": []string{"   "}})
	})

	t.Run("valid Cache-Control", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t,
			http.Header{"Cache-Control": []string{"max-age=60, no-store"}},
			&CacheControlResponse{MaxAge: 60, NoStore: true},
		)
	})

	t.Run("invalid Cache-Control", func(t *testing.T) {
		t.Parallel()
		requireHeaderParseError(t, http.Header{"Cache-Control": []string{"max-age=abc"}})
	})

	t.Run("header name lookup is case-insensitive", func(t *testing.T) {
		t.Parallel()

		// http.Header.Get canonicalises, but a hand-built map may not.
		h := http.Header{}
		h.Set("cache-control", "max-age=60")
		requireHeaderParses(t, h, &CacheControlResponse{MaxAge: 60})
	})

	// Repeated field lines are semantically one comma-joined value
	// (RFC 9110 §5.3). Get() only returns the first, so the implementation
	// must join all values.
	t.Run("multiple Cache-Control header values are joined", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t,
			http.Header{"Cache-Control": []string{"max-age=60", "no-store"}},
			&CacheControlResponse{MaxAge: 60, NoStore: true},
		)
	})

	t.Run("multiple values where the second one is invalid", func(t *testing.T) {
		t.Parallel()
		requireHeaderParseError(t, http.Header{"Cache-Control": []string{"max-age=60", "max-age=abc"}})
	})

	t.Run("multiple values with a quoted field list split across lines", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t,
			http.Header{"Cache-Control": []string{`no-cache="A, B"`, "max-age=60"}},
			&CacheControlResponse{NoCache: fieldNames("a", "b"), MaxAge: 60},
		)
	})

	t.Run("other headers alongside a valid Cache-Control", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t,
			http.Header{"Cache-Control": []string{"private"}, "Vary": []string{"Accept"}},
			&CacheControlResponse{Private: fieldNames()},
		)
	})
}

func TestParseDeltaSeconds(t *testing.T) {
	t.Parallel()

	t.Run("zero", func(t *testing.T) {
		t.Parallel()
		requireDeltaSeconds(t, "0", 0)
	})

	t.Run("small", func(t *testing.T) {
		t.Parallel()
		requireDeltaSeconds(t, "60", 60)
	})

	t.Run("leading zeros", func(t *testing.T) {
		t.Parallel()
		requireDeltaSeconds(t, "0060", 60)
	})

	t.Run("MaxInt32", func(t *testing.T) {
		t.Parallel()
		requireDeltaSeconds(t, "2147483647", math.MaxInt32)
	})

	t.Run("above MaxInt32 clamps", func(t *testing.T) {
		t.Parallel()
		requireDeltaSeconds(t, "2147483648", math.MaxInt32)
	})

	t.Run("MaxInt64", func(t *testing.T) {
		t.Parallel()
		requireDeltaSeconds(t, "9223372036854775807", math.MaxInt32)
	})

	// Documents current behaviour: a negative value is reported as -1 with no
	// error, so the caller must treat -1 as invalid (D6).
	t.Run("negative returns the -1 sentinel", func(t *testing.T) {
		t.Parallel()
		requireDeltaSeconds(t, "-1", -1)
	})

	t.Run("large negative returns the -1 sentinel", func(t *testing.T) {
		t.Parallel()
		requireDeltaSeconds(t, "-2147483649", -1)
	})

	// Documents current behaviour: this returns an error, but D7 says the
	// parser must clamp. parse() has to map strconv.ErrRange to MaxInt32
	// rather than propagating it.
	t.Run("above MaxInt64 errors", func(t *testing.T) {
		t.Parallel()
		requireDeltaSecondsError(t, "99999999999999999999999999")
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		requireDeltaSecondsError(t, "")
	})

	t.Run("non-numeric", func(t *testing.T) {
		t.Parallel()
		requireDeltaSecondsError(t, "abc")
	})

	t.Run("float", func(t *testing.T) {
		t.Parallel()
		requireDeltaSecondsError(t, "60.5")
	})

	t.Run("explicit plus sign is accepted by strconv", func(t *testing.T) {
		t.Parallel()
		requireDeltaSeconds(t, "+60", 60)
	})

	t.Run("whitespace padded", func(t *testing.T) {
		t.Parallel()
		requireDeltaSecondsError(t, " 60 ")
	})

	t.Run("quoted", func(t *testing.T) {
		t.Parallel()
		requireDeltaSecondsError(t, `"60"`)
	})

	t.Run("hex", func(t *testing.T) {
		t.Parallel()
		requireDeltaSecondsError(t, "0x3c")
	})

	t.Run("underscore separator", func(t *testing.T) {
		t.Parallel()
		requireDeltaSecondsError(t, "1_000")
	})
}

func TestDeltaSecondsToGoSeconds(t *testing.T) {
	t.Parallel()

	t.Run("zero", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, time.Duration(0), DeltaSeconds(0).ToGoSeconds())
	})

	t.Run("sixty seconds", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 60*time.Second, DeltaSeconds(60).ToGoSeconds())
	})

	t.Run("negative sentinel", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, -1*time.Second, DeltaSeconds(-1).ToGoSeconds())
	})

	t.Run("MaxInt32 does not overflow", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, time.Duration(math.MaxInt32)*time.Second, DeltaSeconds(math.MaxInt32).ToGoSeconds())
	})
}

func TestFieldNames(t *testing.T) {
	t.Parallel()

	t.Run("new field names is present and empty", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		assert.True(t, f.Present)
		// The map is allocated lazily by Add, so it is nil until first use.
		// Anything reading Fields must tolerate a nil map.
		assert.Empty(t, f.Fields)
		assert.False(t, f.Contains("set-cookie"))
	})

	t.Run("add and contains", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("set-cookie")
		assert.True(t, f.Contains("set-cookie"))
		assert.False(t, f.Contains("authorization"))
	})

	t.Run("add is idempotent", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("set-cookie")
		f.Add("set-cookie")
		assert.Len(t, f.Fields, 1)
	})

	t.Run("add lazily initialises the map on a zero value", func(t *testing.T) {
		t.Parallel()

		f := &FieldNames{}
		f.Add("set-cookie")
		assert.NotNil(t, f.Fields)
		// Present is false on a zero value, so Contains reports false even
		// though the name was added.
		assert.False(t, f.Contains("set-cookie"))
	})

	t.Run("contains is false when not present", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("set-cookie")
		f.Present = false
		assert.False(t, f.Contains("set-cookie"))
	})

	t.Run("contains is case-sensitive so callers must pass lowercase", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("set-cookie")
		assert.False(t, f.Contains("Set-Cookie"))
	})
}

// FuzzParse asserts the parser never panics and never returns a response
// together with an error.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		" ",
		"max-age=60",
		"no-store",
		`no-cache="Set-Cookie, Authorization"`,
		`private="A", max-age=60, public`,
		"max-age=99999999999999999999999999",
		`no-cache="`,
		",,,",
		"max-age=60\x00",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		got, err := parse(input)
		if err != nil {
			require.Nil(t, got, "parse returned both a response and an error for %q", input)
		}
	})
}

// requireParses asserts that input parses without error into want.
func requireParses(t *testing.T, input string, want *CacheControlResponse) {
	t.Helper()

	got, err := parse(input)
	require.NoError(t, err, "unexpected error for input %q", input)
	assertCacheControlResponse(t, want, got)
}

// requireParseError asserts that input is rejected and yields no response.
func requireParseError(t *testing.T, input string) {
	t.Helper()

	got, err := parse(input)
	require.Error(t, err, "expected an error for input %q", input)
	require.Nil(t, got, "a failed parse must not return a response")
}

func requireHeaderParses(t *testing.T, headers http.Header, want *CacheControlResponse) {
	t.Helper()

	got, err := ParseCacheControlResponse(headers)
	require.NoError(t, err)
	assertCacheControlResponse(t, want, got)
}

func requireHeaderParseError(t *testing.T, headers http.Header) {
	t.Helper()

	got, err := ParseCacheControlResponse(headers)
	require.Error(t, err)
	require.Nil(t, got)
}

func requireDeltaSeconds(t *testing.T, input string, want DeltaSeconds) {
	t.Helper()

	got, err := parseDeltaSeconds(input)
	require.NoError(t, err, "unexpected error for input %q", input)
	assert.Equal(t, want, got)
}

// requireDeltaSecondsError asserts an error and the -1 sentinel return value.
func requireDeltaSecondsError(t *testing.T, input string) {
	t.Helper()

	got, err := parseDeltaSeconds(input)
	require.Error(t, err, "expected an error for input %q", input)
	assert.Equal(t, DeltaSeconds(-1), got)
}

// fieldNames builds a *FieldNames marked as present containing the given names.
func fieldNames(names ...string) *FieldNames {
	f := NewFieldNames()
	for _, name := range names {
		f.Add(name)
	}

	return f
}

func assertCacheControlResponse(t *testing.T, expected, actual *CacheControlResponse) {
	t.Helper()

	if expected == nil {
		assert.Nil(t, actual, "expected no response")
		return
	}

	if !assert.NotNil(t, actual, "expected a response, got nil") {
		return
	}

	assert.Equal(t, expected.MaxAge, actual.MaxAge, "MaxAge")
	assert.Equal(t, expected.SMaxAge, actual.SMaxAge, "SMaxAge")
	assert.Equal(t, expected.NoStore, actual.NoStore, "NoStore")
	assert.Equal(t, expected.Public, actual.Public, "Public")

	assertFieldNames(t, "NoCache", expected.NoCache, actual.NoCache)
	assertFieldNames(t, "Private", expected.Private, actual.Private)
}

// assertFieldNames compares two *FieldNames by value. A nil Fields map and an
// empty Fields map are treated as equal.
func assertFieldNames(t *testing.T, name string, expected, actual *FieldNames) {
	t.Helper()

	if expected == nil {
		assert.Nil(t, actual, "%s: expected nil", name)
		return
	}

	if !assert.NotNil(t, actual, "%s: expected non-nil", name) {
		return
	}

	assert.Equal(t, expected.Present, actual.Present, "%s.Present", name)
	assert.ElementsMatch(t, keys(expected.Fields), keys(actual.Fields), "%s.Fields", name)
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}
