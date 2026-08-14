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
//	D0  THE GOVERNING RULE: syntax errors reject the field, semantic problems
//	    do not. If the field cannot be tokenised (unbalanced quotes, control
//	    bytes, a directive name that is not a token, junk between directives)
//	    parse returns an error. If a directive is understood but its argument
//	    is nonsense, out of range, or repeated, the directive is resolved or
//	    dropped and the rest of the field still parses.
//
//	    RFC 9111 §4.2.1 requires this: caches are "encouraged to consider
//	    responses that have invalid freshness information ... to be stale",
//	    not to discard the field. The safety argument is concrete — rejecting
//	    `no-store, max-age=abc` outright would throw away the no-store and
//	    cache a response that must never be stored.
//
//	D1  "No directives" is never an error, however it arises. An empty field, a
//	    whitespace-only field, a field of nothing but commas, and a field of
//	    nothing but directives we do not model all parse to a non-nil, zero
//	    CacheControlResponse. Only ParseCacheControlResponse returns nil, and
//	    only when the header is absent or blank.
//
//	    This was three inconsistent rules before: "" and "   " were errors,
//	    "," was an error, but `must-revalidate` produced a non-nil zero value —
//	    three inputs carrying zero directives, three different outcomes. Under
//	    D0 none of them is a syntax problem, so none of them errors. See D13.
//	D2  Directive names are case-insensitive (RFC 9110 tokens).
//	D3  Directives that the CacheControlResponse struct does not model
//	    (must-revalidate, no-transform, proxy-revalidate, immutable,
//	    stale-while-revalidate, arbitrary cache extensions) are ignored, not
//	    rejected. RFC 9111 §5.2: unknown directives MUST be ignored. A header
//	    consisting only of such directives still parses to a non-nil, zero
//	    CacheControlResponse.
//	D4  UNUSABLE-ARGUMENT RULE (D4, D5 and D19 are one rule, stated once):
//	    when a directive is understood but its argument is missing, empty or
//	    unusable, the argument is discarded and the directive survives if and
//	    only if it still means something unqualified.
//
//	      no-store, public        meaningful bare  → kept  (`no-store=true`
//	                                                        still sets NoStore)
//	      no-cache, private       meaningful bare  → kept, unqualified (D19)
//	      max-age, s-maxage       argument IS the
//	                              whole meaning    → dropped to zero value
//
//	    So the three cases only look like separate policies; they are one test
//	    applied to directives with different amounts of standalone meaning.
//	D5  See D4. max-age / s-maxage with a missing, empty or non-numeric
//	    argument is dropped, never an error. "Dropped" means the field is left
//	    nil, NOT zero: MaxAge and SMaxAge are pointers precisely so that an
//	    absent or unusable directive is distinguishable from `max-age=0`, which
//	    is a real value meaning "already stale". A caller that wants to apply
//	    its own default TTL can only do so if it can tell those apart.
//	D6  delta-seconds content is 1*DIGIT — no sign, no decimal point, no
//	    underscores. Leading zeros are fine. Content that is not 1*DIGIT drops
//	    the directive (D0). The *form* is irrelevant: RFC 9111 §5.2 tells
//	    recipients to accept token and quoted-string arguments alike, so
//	    `max-age="60"` and `max-age=60` are the same thing.
//	D7  delta-seconds above MaxInt32 clamps to MaxInt32 (RFC 9111 §1.2.2),
//	    including values that overflow int64. Note the asymmetry with D6: a
//	    huge value is a well-formed delta-seconds that overflows, and §1.2.2
//	    says to clamp it; a negative or signed value is not a delta-seconds at
//	    all, so D6 drops it. Different outcomes, different reasons.
//
//	    WARNING — parseDeltaSeconds as it stands disagrees with this spec in
//	    three places, so parse() cannot simply delegate to it:
//
//	      "+60"    strconv accepts it → 60, but D6 requires 1*DIGIT → drop
//	      "-1"     returns (-1, nil), a sentinel the caller MUST test for;
//	               it is not reported as an error
//	      "999…9"  returns strconv's ErrRange, but D7 requires MaxInt32
//
//	    TestParseDeltaSeconds pins the current behaviour; TestParse pins what
//	    the parser must produce. Either pre-validate and post-process at the
//	    call site, or change parseDeltaSeconds and update both.
//	D8  no-cache / private accept their field-name argument in either the
//	    quoted-string or the token form, per the same §5.2 recipient advice as
//	    D6. Senders SHOULD emit the quoted form (§5.2.2.4, §5.2.2.7) and only
//	    the quoted form can carry more than one name, but a bare token is read
//	    as a single-element list. Consequence: `no-cache="true"` is a valid
//	    one-element field list, not an error.
//	D9  Field names are stored VERBATIM. No case normalisation, and lookups
//	    via FieldNames.has compare exactly.
//
//	    RFC 9111 defines these as HTTP field names, which are case-insensitive,
//	    so lowercasing would be correct for a general-purpose cache. It is
//	    wrong here: GraphQL is case-sensitive, `productName` and `productname`
//	    are different fields, and ToLower merges them with no way to recover
//	    the difference. Same reasoning as D19 — this parser serves GraphQL
//	    federation entity caching, so it must not impose HTTP naming semantics
//	    on names that may not be HTTP names.
//
//	    Note the asymmetry with D2: directive NAMES (max-age, no-cache) are
//	    genuinely HTTP tokens defined by RFC 9111 and stay case-insensitive.
//	    Only the field-list CONTENTS, whose namespace is the subgraph's, are
//	    left alone. A caller that knows it is dealing with HTTP header names
//	    can fold case itself; one dealing with GraphQL names could not have
//	    undone it.
//	D10 Inside the quoted-string, elements are comma-separated with optional
//	    whitespace, and empty elements are skipped (RFC 9110 §5.6.1 legacy list
//	    rule). Each non-empty element must be a valid token.
//	D11 `no-cache=""` and `no-cache="   "` mean Present with no field names —
//	    identical to bare `no-cache`.
//	D12 Commas inside a quoted-string do not terminate the directive.
//	D13 Empty elements in the top-level directive list are skipped
//	    (leading/trailing/double commas are tolerated), per the RFC 9110 §5.6.1
//	    legacy list rule. A list that yields zero directives is fine — see D1.
//
//	D14 DUPLICATE max-age / s-maxage: the FIRST occurrence wins. RFC 9111
//	    §4.2.1 says so outright — "When there is more than one value present
//	    for a given directive (e.g., two Expires header field lines or multiple
//	    Cache-Control: max-age directives), either the first occurrence should
//	    be used or the response should be considered stale" — and max-age is
//	    its own worked example. Duplicates are never an error, and no value
//	    comparison happens, so equal and conflicting repeats take one path.
//
//	    Evaluation stops at the first occurrence, including when that one is
//	    unusable: `max-age=abc, max-age=60` drops the directive (D4) and does
//	    not promote the 60, while `max-age=60, max-age=abc` keeps 60. Order
//	    matters, which is inherent to a first-occurrence rule.
//
//	    Deliberately NOT the minimum. Taking the smallest value would be a
//	    caching policy decision, and D16 reserves those for the caller; it
//	    would also diverge from every cache that follows §4.2.1, on a point
//	    where the spec is explicit rather than silent.
//
//	D15 DUPLICATE no-cache / private: join on restrictiveness — an unqualified
//	    occurrence wins outright, otherwise union the field names.
//
//	    This differs from D14 for one reason: RFC 9111 says nothing whatever
//	    about repeating no-cache or private, and §4.2.1's first-occurrence
//	    sentence is scoped to freshness (it lives in "Calculating Freshness
//	    Lifetime"; its examples are Expires and max-age). Where the spec
//	    speaks we follow it (D14); where it is silent we derive from the
//	    directive semantics, and §5.2.2.4 / §5.2.2.7 make the qualified form a
//	    *relaxation* of the unqualified one. Unqualified no-cache forbids reuse
//	    outright while no-cache="X" permits reuse once X is stripped;
//	    unqualified private forbids a shared cache from storing the response at
//	    all while private="X" only withholds X. Restrictiveness runs
//
//	      unqualified  >>  many names  >  few names
//
//	    Applying first-occurrence here instead would make
//	    `private="Set-Cookie", private` keep the qualified form and discard the
//	    unqualified one — downgrading a MUST NOT into a MAY on a privacy
//	    directive. That asymmetry is why the two rules differ, and it is a
//	    justified difference rather than the inconsistency it resembles.
//
//	D16 The parser resolves REPEATS of one directive (D14) but does not resolve
//	    conflicts BETWEEN different directives. `public, private` records both;
//	    `no-store, max-age=60` records both. §4.2.1 suggests honouring the most
//	    restrictive on conflict, but that is a policy decision the struct can
//	    represent faithfully, so it belongs to the caller. The boundary: a
//	    repeat has to collapse because there is one field to store it in; a
//	    conflict does not.
//
//	D17 Control characters are always a syntax error: CTLs are neither tchar
//	    nor qdtext, so they cannot legally appear anywhere, quoted or not.
//
//	    Non-ASCII bytes are NOT the same case. %x80-FF is obs-text, which is
//	    valid qdtext (RFC 9110 §5.6.4) but never a tchar. So obs-text outside a
//	    quoted string is a syntax error — no token can hold it — while obs-text
//	    inside one is a well-formed string whose content is not a field name,
//	    which is D19. Treating both as errors was inconsistent: it split
//	    `no-cache="Sét-Cookie"` from `no-cache="Set Cookie"`, which fail in
//	    precisely the same way.
//	D18 Quoted-pair escaping (`\"`, `\\`) is not accepted inside the field-name
//	    list, because the resulting characters are not valid token characters
//	    anyway.
//
//	D19 Field-list elements are stored VERBATIM. Split on commas, trim
//	    surrounding whitespace, drop empties, keep whatever is left. No token
//	    validation, so nothing in a field list is ever rejected or discarded.
//
//	    This is a domain call, not an HTTP one. RFC 9111 defines these lists as
//	    HTTP field names, so tchar validation would be correct for a general
//	    cache — but this parser feeds GraphQL federation entity caching, where
//	    what a subgraph puts in the list is not yet settled. It may well be
//	    GraphQL field names or type-qualified paths (`Product.name`), which are
//	    not HTTP field names at all.
//
//	    Validating against a namespace we have not defined has a bad failure
//	    mode: an unrecognised name would silently degrade the directive and
//	    cost cache hits, and the more restrictive the fallback, the more it
//	    costs. Storing verbatim keeps the decision with the caller, which knows
//	    what the names mean. The parser's job here is to split a list, not to
//	    judge its contents.
//
//	    This replaces an earlier rule where one invalid element collapsed the
//	    whole directive to its unqualified form. That was defensible for a
//	    general-purpose HTTP cache and wrong for this one.
//

func TestParse(t *testing.T) {
	t.Parallel()

	// --- D1: input carrying zero directives is not an error -----------------
	// These four, the "only commas" group under D13, and the D3 group are the
	// same situation reached four different ways. They must agree.

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "", &CacheControlResponse{})
	})

	t.Run("single space", func(t *testing.T) {
		t.Parallel()
		requireParses(t, " ", &CacheControlResponse{})
	})

	t.Run("single tab", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "\t", &CacheControlResponse{})
	})

	// OWS is SP / HTAB only (RFC 9110 §5.6.3). A bare CR or LF is malformed
	// rather than blank, and is covered by the D17 subtests below.
	t.Run("mixed whitespace only", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "  \t \t ", &CacheControlResponse{})
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
		requireParses(t, "max-age=60, stale-while-revalidate=30", &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("stale-if-error is ignored", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60, stale-if-error=120", &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("unknown extension with quoted argument is ignored", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `community="UCI", max-age=60`, &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("unknown extension with comma inside quotes is ignored without breaking the list", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `community="UCI, UCLA", no-store`, &CacheControlResponse{NoStore: true})
	})

	t.Run("unknown bare extension is ignored", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60, ext", &CacheControlResponse{MaxAge: seconds(60)})
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
		requireParses(t, "MAX-AGE=60", &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("S-MaxAge mixed case", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "S-MaxAge=60", &CacheControlResponse{SMaxAge: seconds(60)})
	})

	// --- delta-seconds, valid -----------------------------------------------

	t.Run("max-age zero", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=0", &CacheControlResponse{MaxAge: seconds(0)})
	})

	t.Run("max-age", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60", &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("s-maxage", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "s-maxage=60", &CacheControlResponse{SMaxAge: seconds(60)})
	})

	t.Run("max-age at MaxInt32", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=2147483647", &CacheControlResponse{MaxAge: seconds(math.MaxInt32)})
	})

	// D7
	t.Run("max-age above MaxInt32 clamps", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=2147483648", &CacheControlResponse{MaxAge: seconds(math.MaxInt32)})
	})

	// D7 — this one overflows int64 and makes strconv return ErrRange.
	t.Run("max-age above MaxInt64 clamps", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=99999999999999999999999999", &CacheControlResponse{MaxAge: seconds(math.MaxInt32)})
	})

	t.Run("s-maxage above MaxInt32 clamps", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "s-maxage=4294967296", &CacheControlResponse{SMaxAge: seconds(math.MaxInt32)})
	})

	t.Run("leading zeros", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=007", &CacheControlResponse{MaxAge: seconds(7)})
	})

	t.Run("all zeros", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=00000", &CacheControlResponse{MaxAge: seconds(0)})
	})

	// MaxAge / SMaxAge are pointers so that "absent" and "explicitly zero" are
	// distinguishable. These three pin that distinction directly, since it is
	// the reason for the pointer and the easiest thing to regress.

	t.Run("absent max-age is nil rather than zero", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-store", &CacheControlResponse{NoStore: true})
	})

	t.Run("explicit zero max-age is set rather than absent", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-store, max-age=0", &CacheControlResponse{
			NoStore: true,
			MaxAge:  seconds(0),
		})
	})

	t.Run("max-age set does not imply s-maxage set", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=0", &CacheControlResponse{MaxAge: seconds(0)})
	})

	// --- D6: the quoted form of delta-seconds is equivalent -----------------

	t.Run("max-age with quotes", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `max-age="60"`, &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("s-maxage with quotes", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `s-maxage="60"`, &CacheControlResponse{SMaxAge: seconds(60)})
	})

	// --- D5/D6: delta-seconds, invalid content drops the directive ----------

	t.Run("max-age with no argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age", &CacheControlResponse{})
	})

	t.Run("s-maxage with no argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "s-maxage", &CacheControlResponse{})
	})

	t.Run("max-age with empty argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=", &CacheControlResponse{})
	})

	t.Run("s-maxage with empty argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "s-maxage=", &CacheControlResponse{})
	})

	t.Run("max-age non-numeric", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=abc", &CacheControlResponse{})
	})

	t.Run("max-age float", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60.5", &CacheControlResponse{})
	})

	t.Run("max-age negative", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=-1", &CacheControlResponse{})
	})

	t.Run("s-maxage negative", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "s-maxage=-100", &CacheControlResponse{})
	})

	t.Run("max-age explicit plus sign", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=+60", &CacheControlResponse{})
	})

	t.Run("max-age with underscore separator", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=1_000", &CacheControlResponse{})
	})

	t.Run("max-age hex", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=0x3c", &CacheControlResponse{})
	})

	t.Run("max-age with trailing unit", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60s", &CacheControlResponse{})
	})

	// D0 — the safety case. A broken max-age must not cost us the no-store.
	t.Run("invalid max-age does not discard the rest of the field", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-store, max-age=abc", &CacheControlResponse{NoStore: true})
	})

	t.Run("invalid max-age does not discard a later valid directive", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `max-age=abc, private="Set-Cookie"`, &CacheControlResponse{
			Private: fieldNames("Set-Cookie"),
		})
	})

	// This one stays an error: the space breaks the list structure, so it is a
	// syntax problem rather than a bad argument (D0).
	t.Run("max-age with internal space", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "max-age=6 0")
	})

	// --- D4: boolean directives ignore a stray argument ---------------------

	t.Run("no-store with token argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-store=true", &CacheControlResponse{NoStore: true})
	})

	t.Run("no-store with quoted argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-store="true"`, &CacheControlResponse{NoStore: true})
	})

	t.Run("public with token argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "public=true", &CacheControlResponse{Public: true})
	})

	t.Run("public with quoted argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `public="true"`, &CacheControlResponse{Public: true})
	})

	t.Run("no-store with empty argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-store=", &CacheControlResponse{NoStore: true})
	})

	// --- D8: both argument forms are accepted for field lists ---------------

	t.Run("no-cache with bare token argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-cache=Set-Cookie", &CacheControlResponse{NoCache: fieldNames("Set-Cookie")})
	})

	t.Run("private with bare token argument", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "private=Set-Cookie", &CacheControlResponse{Private: fieldNames("Set-Cookie")})
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
		requireParses(t, `no-cache="Set-Cookie"`, &CacheControlResponse{NoCache: fieldNames("Set-Cookie")})
	})

	t.Run("private with one field name", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="Set-Cookie"`, &CacheControlResponse{Private: fieldNames("Set-Cookie")})
	})

	t.Run("no-cache with two field names", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie, Authorization"`, &CacheControlResponse{
			NoCache: fieldNames("Set-Cookie", "Authorization"),
		})
	})

	t.Run("private with three field names and no whitespace", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="Set-Cookie,Authorization,X-Custom"`, &CacheControlResponse{
			Private: fieldNames("Set-Cookie", "Authorization", "X-Custom"),
		})
	})

	t.Run("field names with surrounding whitespace are trimmed", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="  Set-Cookie ,  Authorization  "`, &CacheControlResponse{
			Private: fieldNames("Set-Cookie", "Authorization"),
		})
	})

	t.Run("field names with tabs are trimmed", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "private=\"\tSet-Cookie\t,\tAuthorization\t\"", &CacheControlResponse{
			Private: fieldNames("Set-Cookie", "Authorization"),
		})
	})

	// D9 — case is preserved exactly as sent.
	t.Run("field names keep their case", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="SET-COOKIE, AuThOrIzAtIoN"`, &CacheControlResponse{
			NoCache: fieldNames("SET-COOKIE", "AuThOrIzAtIoN"),
		})
	})

	// D9 — the case that decides it. Under GraphQL semantics these are three
	// different fields, so they must not collapse.
	t.Run("field names differing only in case stay distinct", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie, set-cookie, SET-COOKIE"`, &CacheControlResponse{
			NoCache: fieldNames("Set-Cookie", "set-cookie", "SET-COOKIE"),
		})
	})

	// D9 — the GraphQL pair that motivated dropping normalisation.
	t.Run("graphql field names differing only in case stay distinct", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="productName, productname"`, &CacheControlResponse{
			NoCache: fieldNames("productName", "productname"),
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
			NoCache: fieldNames("Set-Cookie", "Authorization"),
		})
	})

	// D10
	t.Run("leading and trailing commas inside the field list are skipped", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache=",Set-Cookie,"`, &CacheControlResponse{NoCache: fieldNames("Set-Cookie")})
	})

	t.Run("field names containing all token special characters", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="X-Foo!#$%&'*+.^_`+"`"+`|~0"`, &CacheControlResponse{
			NoCache: fieldNames(`X-Foo!#$%&'*+.^_` + "`" + `|~0`),
		})
	})

	// --- D19: field-list elements are stored verbatim ------------------------
	// None of these is a legal HTTP token, and none of them is rejected. See
	// D19 for why this domain cannot afford token validation.

	t.Run("field name with internal space", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set Cookie"`, &CacheControlResponse{NoCache: fieldNames("Set Cookie")})
	})

	t.Run("field name with semicolon", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie;"`, &CacheControlResponse{NoCache: fieldNames("Set-Cookie;")})
	})

	t.Run("field name with colon", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie:"`, &CacheControlResponse{NoCache: fieldNames("Set-Cookie:")})
	})

	t.Run("field name with slash", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set/Cookie"`, &CacheControlResponse{NoCache: fieldNames("Set/Cookie")})
	})

	t.Run("field name with parentheses", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="(Set-Cookie)"`, &CacheControlResponse{NoCache: fieldNames("(Set-Cookie)")})
	})

	t.Run("field name with equals", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set=Cookie"`, &CacheControlResponse{NoCache: fieldNames("Set=Cookie")})
	})

	// D19 — an odd element never costs us the elements around it.
	t.Run("an unrecognised element does not discard its neighbours", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie, Bad Name"`, &CacheControlResponse{
			NoCache: fieldNames("Set-Cookie", "Bad Name"),
		})
	})

	t.Run("an unrecognised element does not discard the rest of the field", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set Cookie", max-age=60`, &CacheControlResponse{
			NoCache: fieldNames("Set Cookie"),
			MaxAge:  seconds(60),
		})
	})

	// D19 — a GraphQL field name or a type-qualified path is not an HTTP
	// token, and both must survive. We do not yet know what subgraphs will put
	// here, so the parser stores it rather than judging it.
	t.Run("graphql style field names are kept", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Product.name, Review.body"`, &CacheControlResponse{
			NoCache: fieldNames("Product.name", "Review.body"),
		})
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

	// Apostrophe IS a tchar (RFC 9110 §5.6.2), so this is a well-formed token
	// argument that happens to contain quote characters — not a quoted string.
	// The apostrophes are simply part of the stored name.
	t.Run("single quotes are not quoting", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache='Set-Cookie'`, &CacheControlResponse{NoCache: fieldNames("'Set-Cookie'")})
	})

	t.Run("doubled quotes", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache=""Set-Cookie""`)
	})

	// D18 — with quoted-pair unsupported, the string closes at the second
	// quote and `b"` is left over as junk between directives. Syntax, so the
	// whole field is rejected (D0).
	t.Run("escaped quote inside the field list", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="a\"b"`)
	})

	// D18 — the quoted string is well formed, so its content is kept as-is.
	// Backslash has no special meaning: there is no unescaping step.
	t.Run("escaped backslash inside the field list", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="a\\b"`, &CacheControlResponse{NoCache: fieldNames(`a\\b`)})
	})

	t.Run("trailing content after closing quote", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, `no-cache="Set-Cookie"x`)
	})

	// --- multiple directives -------------------------------------------------

	t.Run("max-age and no-store", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60, no-store", &CacheControlResponse{MaxAge: seconds(60), NoStore: true})
	})

	t.Run("no space after comma", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60,no-store", &CacheControlResponse{MaxAge: seconds(60), NoStore: true})
	})

	t.Run("whitespace on both sides of the comma", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60 ,  no-store", &CacheControlResponse{MaxAge: seconds(60), NoStore: true})
	})

	t.Run("tabs around the comma", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "\tmax-age=60\t,\tno-store\t", &CacheControlResponse{MaxAge: seconds(60), NoStore: true})
	})

	t.Run("leading and trailing whitespace around the whole value", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "   max-age=60, no-store   ", &CacheControlResponse{MaxAge: seconds(60), NoStore: true})
	})

	t.Run("public with max-age and s-maxage", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "public, max-age=3600, s-maxage=7200", &CacheControlResponse{
			Public: true, MaxAge: seconds(3600), SMaxAge: seconds(7200),
		})
	})

	t.Run("typical no-cache combination", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-cache, no-store, must-revalidate, max-age=0", &CacheControlResponse{
			NoCache: fieldNames(), NoStore: true, MaxAge: seconds(0),
		})
	})

	t.Run("private field list alongside max-age", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="Set-Cookie", max-age=60`, &CacheControlResponse{
			Private: fieldNames("Set-Cookie"), MaxAge: seconds(60),
		})
	})

	// D12
	t.Run("comma inside quotes does not terminate the directive", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie, Authorization", max-age=60`, &CacheControlResponse{
			NoCache: fieldNames("Set-Cookie", "Authorization"),
			MaxAge:  seconds(60),
		})
	})

	// D12
	t.Run("two quoted field lists in one header", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="A, B", private="C, D"`, &CacheControlResponse{
			NoCache: fieldNames("A", "B"),
			Private: fieldNames("C", "D"),
		})
	})

	t.Run("all modelled directives at once", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `max-age=1, s-maxage=2, no-store, no-cache="A", public, private="B"`, &CacheControlResponse{
			MaxAge:  seconds(1),
			SMaxAge: seconds(2),
			NoStore: true,
			NoCache: fieldNames("A"),
			Public:  true,
			Private: fieldNames("B"),
		})
	})

	// --- D13: list structure -------------------------------------------------

	t.Run("trailing comma is tolerated", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60,", &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("leading comma is tolerated", func(t *testing.T) {
		t.Parallel()
		requireParses(t, ",max-age=60", &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("double comma is tolerated", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60,,no-store", &CacheControlResponse{MaxAge: seconds(60), NoStore: true})
	})

	// D13 + D1 — if a trailing empty element is skipped in "max-age=60,", then
	// a field of nothing but empty elements is a field of zero directives, not
	// an error. Erroring here while accepting "max-age=60," was the
	// contradiction.

	t.Run("only a comma", func(t *testing.T) {
		t.Parallel()
		requireParses(t, ",", &CacheControlResponse{})
	})

	t.Run("only commas", func(t *testing.T) {
		t.Parallel()
		requireParses(t, ",,,", &CacheControlResponse{})
	})

	t.Run("only commas and whitespace", func(t *testing.T) {
		t.Parallel()
		requireParses(t, " , , ", &CacheControlResponse{})
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

	// Whitespace around "=" is not permitted by the grammar, but it is
	// tolerated rather than rejected: discarding the whole field over a
	// cosmetic space is exactly what D0 exists to prevent. `max-age=6 0`
	// below still fails, because two values with a gap and no comma is a
	// structural problem rather than a stray space.
	t.Run("space before equals is tolerated", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age =60", &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("space after equals is tolerated", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age= 60", &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("space on both sides of equals is tolerated", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache = "Set-Cookie"`, &CacheControlResponse{
			NoCache: fieldNames("Set-Cookie"),
		})
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
		requireParses(t, "max-age=60, max-age=60", &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("duplicate boolean directive", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-store, no-store", &CacheControlResponse{NoStore: true})
	})

	// D14 — first occurrence wins. Equal and conflicting repeats take the same
	// path, so no error case is needed.
	t.Run("duplicate max-age takes the first", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60, max-age=120", &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("duplicate s-maxage takes the first", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "s-maxage=1, s-maxage=2", &CacheControlResponse{SMaxAge: seconds(1)})
	})

	// D14 — first even when it is the larger value. This is the case that
	// separates first-occurrence from "most restrictive": the minimum would
	// give 60 here, but choosing it is a caching policy decision and D16
	// leaves those to the caller.
	t.Run("duplicate max-age takes the first even when it is larger", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=120, max-age=60", &CacheControlResponse{MaxAge: seconds(120)})
	})

	t.Run("duplicate s-maxage takes the first even when it is larger", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "s-maxage=2, s-maxage=1", &CacheControlResponse{SMaxAge: seconds(2)})
	})

	// D14 — zero is a real value, and later occurrences never override it.
	// This expected 3600 before the fields became pointers, which was simply
	// wrong for a first-occurrence rule; a zero max-age used to be
	// indistinguishable from an absent one, so nothing caught it.
	t.Run("duplicate max-age takes a leading zero value", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=0, max-age=3600", &CacheControlResponse{MaxAge: seconds(0)})
	})

	t.Run("duplicate max-age ignores a trailing zero value", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=3600, max-age=0", &CacheControlResponse{MaxAge: seconds(3600)})
	})

	t.Run("duplicate max-age that both clamp to MaxInt32", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=2147483648, max-age=4294967296", &CacheControlResponse{MaxAge: seconds(math.MaxInt32)})
	})

	// D14 + D4 — evaluation stops at the first occurrence even when it is the
	// unusable one, so the valid later value is not resurrected.
	t.Run("invalid first occurrence does not promote the second", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=abc, max-age=60", &CacheControlResponse{})
	})

	// D14 — and the mirror image: a usable first occurrence stands, whatever
	// follows it.
	t.Run("invalid second occurrence does not disturb the first", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age=60, max-age=abc", &CacheControlResponse{MaxAge: seconds(60)})
	})

	// D15 tier 2 — all occurrences qualified, so union. Stripping both A and B
	// is stricter than stripping either alone.
	t.Run("duplicate no-cache field lists are unioned", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="A", no-cache="B"`, &CacheControlResponse{NoCache: fieldNames("A", "B")})
	})

	t.Run("duplicate private field lists are unioned", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="A", private="B, C"`, &CacheControlResponse{Private: fieldNames("A", "B", "C")})
	})

	t.Run("overlapping field lists union without duplicating", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="A, B", no-cache="B, C"`, &CacheControlResponse{
			NoCache: fieldNames("A", "B", "C"),
		})
	})

	// D15 tier 1 — an unqualified occurrence dominates, in either order.
	// Producing fieldNames("Set-Cookie") here would downgrade "a shared cache
	// MUST NOT store this response" into "it may store all but Set-Cookie".
	t.Run("bare private before a private field list stays unqualified", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private, private="Set-Cookie"`, &CacheControlResponse{Private: fieldNames()})
	})

	t.Run("bare private after a private field list stays unqualified", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `private="Set-Cookie", private`, &CacheControlResponse{Private: fieldNames()})
	})

	t.Run("bare no-cache dominates a no-cache field list", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie", no-cache`, &CacheControlResponse{NoCache: fieldNames()})
	})

	// D15 tier 1 + D11 — an empty argument is the unqualified form, so it
	// dominates too.
	t.Run("empty field list dominates a populated one", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie", no-cache=""`, &CacheControlResponse{NoCache: fieldNames()})
	})

	// D15 tier 2 + D19 — an odd name in the second occurrence is just another
	// name. Nothing is discarded, so both survive the union.
	t.Run("an unrecognised name in a repeat still unions", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Set-Cookie", no-cache="Bad Name"`, &CacheControlResponse{
			NoCache: fieldNames("Set-Cookie", "Bad Name"),
		})
	})

	// --- D16: the parser records, it does not resolve -------------------------

	t.Run("public and private together", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "public, private", &CacheControlResponse{Public: true, Private: fieldNames()})
	})

	t.Run("no-store with a positive max-age", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-store, max-age=60", &CacheControlResponse{NoStore: true, MaxAge: seconds(60)})
	})

	t.Run("no-cache with a positive max-age", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "no-cache, max-age=3600", &CacheControlResponse{NoCache: fieldNames(), MaxAge: seconds(3600)})
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

	// D17 + D19 — obs-text (%x80-FF) IS valid qdtext per RFC 9110 §5.6.4, so
	// this quoted string is well formed and only its *content* fails to be a
	// token. That is exactly the `no-cache="Set Cookie"` situation, so it must
	// reach the same answer: unqualified, not an error. Contrast the directive
	// name below, where there is no quoted string to make obs-text legal.
	t.Run("non-ASCII in field name", func(t *testing.T) {
		t.Parallel()
		requireParses(t, `no-cache="Sét-Cookie"`, &CacheControlResponse{NoCache: fieldNames("Sét-Cookie")})
	})

	t.Run("non-ASCII alone", func(t *testing.T) {
		t.Parallel()
		requireParseError(t, "🚀")
	})

	// --- robustness -----------------------------------------------------------

	t.Run("many repeated ignorable directives", func(t *testing.T) {
		t.Parallel()

		input := strings.TrimSuffix(strings.Repeat("must-revalidate, ", 1000), ", ") + ", max-age=60"
		requireParses(t, input, &CacheControlResponse{MaxAge: seconds(60)})
	})

	t.Run("very long field list", func(t *testing.T) {
		t.Parallel()

		input := `no-cache="` + strings.TrimSuffix(strings.Repeat("x-a,", 500), ",") + `"`
		requireParses(t, input, &CacheControlResponse{NoCache: fieldNames("x-a")})
	})

	t.Run("very long run of leading zeros", func(t *testing.T) {
		t.Parallel()
		requireParses(t, "max-age="+strings.Repeat("0", 100)+"60", &CacheControlResponse{MaxAge: seconds(60)})
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

	// D1 — same content as the empty case above, so it must reach the same
	// answer. Erroring here while returning (nil, nil) for "" was a difference
	// of pure whitespace.
	t.Run("Cache-Control whitespace only", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t, http.Header{"Cache-Control": []string{"   "}}, nil)
	})

	t.Run("Cache-Control commas only", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t, http.Header{"Cache-Control": []string{" , "}}, nil)
	})

	// D1 + D3 — a field we understood but modelled none of still parsed, so it
	// is not the "absent" case and must not be nil.
	t.Run("Cache-Control with only unmodelled directives is non-nil", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t,
			http.Header{"Cache-Control": []string{"must-revalidate"}},
			&CacheControlResponse{},
		)
	})

	t.Run("valid Cache-Control", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t,
			http.Header{"Cache-Control": []string{"max-age=60, no-store"}},
			&CacheControlResponse{MaxAge: seconds(60), NoStore: true},
		)
	})

	// D0 — a malformed *structure* still propagates as an error.
	t.Run("malformed Cache-Control", func(t *testing.T) {
		t.Parallel()
		requireHeaderParseError(t, http.Header{"Cache-Control": []string{`no-cache="unterminated`}})
	})

	// D5 — a bad value is not malformed. The directive is dropped and the
	// response still parses.
	t.Run("Cache-Control with an unusable value", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t,
			http.Header{"Cache-Control": []string{"no-store, max-age=abc"}},
			&CacheControlResponse{NoStore: true},
		)
	})

	t.Run("header name lookup is case-insensitive", func(t *testing.T) {
		t.Parallel()

		// http.Header.Get canonicalises, but a hand-built map may not.
		h := http.Header{}
		h.Set("cache-control", "max-age=60")
		requireHeaderParses(t, h, &CacheControlResponse{MaxAge: seconds(60)})
	})

	// Repeated field lines are semantically one comma-joined value
	// (RFC 9110 §5.3). Get() only returns the first, so the implementation
	// must join all values.
	t.Run("multiple Cache-Control header values are joined", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t,
			http.Header{"Cache-Control": []string{"max-age=60", "no-store"}},
			&CacheControlResponse{MaxAge: seconds(60), NoStore: true},
		)
	})

	// D14 — joined into one field, so the first occurrence wins and the
	// unusable repeat is skipped rather than failing the header.
	t.Run("multiple values where the second one is unusable", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t,
			http.Header{"Cache-Control": []string{"max-age=60", "max-age=abc"}},
			&CacheControlResponse{MaxAge: seconds(60)},
		)
	})

	t.Run("multiple values where the second one is malformed", func(t *testing.T) {
		t.Parallel()
		requireHeaderParseError(t, http.Header{"Cache-Control": []string{"max-age=60", `no-cache="x`}})
	})

	t.Run("multiple values with a quoted field list split across lines", func(t *testing.T) {
		t.Parallel()
		requireHeaderParses(t,
			http.Header{"Cache-Control": []string{`no-cache="A, B"`, "max-age=60"}},
			&CacheControlResponse{NoCache: fieldNames("A", "B"), MaxAge: seconds(60)},
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
		assert.False(t, f.has("set-cookie"))
	})

	t.Run("add and contains", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("set-cookie")
		assert.True(t, f.has("set-cookie"))
		assert.False(t, f.has("authorization"))
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
		assert.False(t, f.has("set-cookie"))
	})

	t.Run("contains is false when not present", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("set-cookie")
		f.Present = false
		assert.False(t, f.has("set-cookie"))
	})

	// D9 — no normalisation, so lookups must match exactly.
	t.Run("add and has are case-sensitive", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("Set-Cookie")
		assert.True(t, f.has("Set-Cookie"))
		assert.False(t, f.has("set-cookie"))
		assert.False(t, f.has("SET-COOKIE"))
	})

	t.Run("add stores the name verbatim", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("Set-Cookie")
		assert.Equal(t, []string{"Set-Cookie"}, keys(f.Fields))
	})

	t.Run("add keeps names differing only in case apart", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("Set-Cookie")
		f.Add("SET-COOKIE")
		f.Add("set-cookie")
		assert.Len(t, f.Fields, 3)
	})

	// The GraphQL case: two distinct fields that lowercasing would have merged.
	t.Run("add keeps graphql field names differing only in case apart", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("productName")
		f.Add("productname")
		assert.Len(t, f.Fields, 2)
		assert.True(t, f.has("productName"))
		assert.True(t, f.has("productname"))
	})

	t.Run("contains on a nil receiver is false", func(t *testing.T) {
		t.Parallel()

		var f *FieldNames
		assert.False(t, f.has("set-cookie"))
	})

	// ClearAndLock is the D15 tier-1 mechanism: an unqualified occurrence must
	// win over a qualified one in either order, so it both drops the names
	// already collected and prevents later ones from re-qualifying the
	// directive.

	t.Run("clear and lock drops names collected so far", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("Set-Cookie")
		f.ClearAndLock()

		assert.Empty(t, f.Fields)
		assert.False(t, f.has("set-cookie"))
		// Still present — unqualified is the strongest form, not an absence.
		assert.True(t, f.Present)
	})

	t.Run("clear and lock rejects later names", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.ClearAndLock()
		f.Add("Set-Cookie")

		assert.Empty(t, f.Fields)
		assert.False(t, f.has("set-cookie"))
	})

	t.Run("clear and lock is idempotent", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.Add("Set-Cookie")
		f.ClearAndLock()
		f.Add("Authorization")
		f.ClearAndLock()

		assert.Empty(t, f.Fields)
		assert.True(t, f.Present)
	})

	t.Run("clear and lock on a fresh value leaves it unqualified", func(t *testing.T) {
		t.Parallel()

		f := NewFieldNames()
		f.ClearAndLock()

		assert.Empty(t, f.Fields)
		assert.True(t, f.Present)
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

// assertDeltaSeconds compares two optional delta-seconds fields. It reports
// absent-vs-present separately from a value mismatch, because "no max-age" and
// "max-age=0" are different responses that a bare assert.Equal would describe
// only as two pointers.
func assertDeltaSeconds(t *testing.T, name string, expected, actual *DeltaSeconds) {
	t.Helper()

	switch {
	case expected == nil && actual == nil:
		return
	case expected == nil:
		t.Errorf("%s: expected absent, got %d", name, *actual)
	case actual == nil:
		t.Errorf("%s: expected %d, got absent", name, *expected)
	case *expected != *actual:
		t.Errorf("%s: expected %d, got %d", name, *expected, *actual)
	}
}

// seconds builds a *DeltaSeconds for an expected max-age / s-maxage. A nil
// field means the directive was absent or dropped; seconds(0) means it was
// present and explicitly zero. Those are different responses, which is the
// whole reason the fields are pointers.
func seconds(d DeltaSeconds) *DeltaSeconds {
	return &d
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

	assertDeltaSeconds(t, "MaxAge", expected.MaxAge, actual.MaxAge)
	assertDeltaSeconds(t, "SMaxAge", expected.SMaxAge, actual.SMaxAge)
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
