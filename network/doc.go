// Package network is the gobash secure HTTP layer. It defines the
// Doer interface every network-touching built-in talks to, the
// allow-list machinery, the typed errors host code can match with
// errors.As, and the default SecureFetch implementation backed by
// net/http.
//
// # Phase 9 surface
//
// Five things land in Phase 9:
//
//  1. The Doer interface and Response value type (§9.3).
//  2. Config + AllowedURLEntry + RequestTransform + DNSLookupResult
//     (§9.1).
//  3. The allow-list compiler and per-redirect re-validation logic
//     (§9.2).
//  4. NewSecureFetch (§9.3) — an http.Client-equivalent that enforces
//     Config and defaults to DENY-ALL when no allowed prefixes are
//     supplied.
//  5. The five typed errors (§9.4): NetworkAccessDeniedError,
//     TooManyRedirectsError, RedirectNotAllowedError,
//     MethodNotAllowedError, ResponseTooLargeError.
//
// Phase 9 does NOT add any built-ins; curl / html-to-markdown ship in
// Phase 10. The package is consumed today by gobash.(*Bash) (which
// resolves a Doer for every Exec) and by gobash/command.Context.Fetch
// (which Phase 10 commands will read).
//
// # Security defaults
//
// Network is OFF unless BashOptions.Network or BashOptions.Fetch is
// explicitly configured. When Network is set with no AllowedURLPrefixes
// and DangerouslyAllowFullAccess=false, every request fails with
// NetworkAccessDeniedError — there is no implicit allow-all path.
// DenyPrivateRanges layers on top of the allow-list: even an allowed
// URL is denied if its resolved IPs are private/loopback/link-local.
//
// # Go configuration compatibility
//
// Header transforms accumulate across all matching allow-list entries in
// configuration order; later transforms override earlier values for the same
// header name. They are recomputed for each redirect destination.
// AllowedMethods distinguishes nil (GET/HEAD defaults) from an empty slice
// (deny every method), except in DangerouslyAllowFullAccess mode.
//
// Unlike optional fields in just-bash, the existing scalar Config fields
// cannot distinguish omission from an explicit zero. For compatibility,
// MaxRedirects=0 still means 20 (use -1 to return the first redirect response),
// and Timeout=0 and MaxResponseSize=0 still select their package defaults.
// DenyPrivateRanges defaults to false, not just-bash's production default of
// true; production callers should explicitly enable it. Changing these field
// types or silently reinterpreting existing configurations would break the
// Go API or its behavior.
//
// # Reference (read-only)
//
// vercel-labs/just-bash, src/network/. The Go port preserves the TS
// matcher semantics: origin normalization, %2f / %5c rejection, and
// per-redirect re-validation.
package network
