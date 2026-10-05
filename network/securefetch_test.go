package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// httptest.Server uses 127.0.0.1, so we never enable DenyPrivateRanges
// in tests that use it. The DenyPrivateRanges path is exercised
// separately with a stub DNSResolve.

func mustReq(t *testing.T, method, urlStr string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestSecureFetch_DeniesByDefault(t *testing.T) {
	t.Parallel()
	d := NewSecureFetch(nil)
	req := mustReq(t, "GET", "https://api.example.com/")
	_, err := d.Do(context.Background(), req)
	var nad *NetworkAccessDeniedError
	if !errors.As(err, &nad) {
		t.Fatalf("err = %v, want *NetworkAccessDeniedError", err)
	}
}

func TestSecureFetch_AllowsExactPrefix(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "hello %s", r.URL.Path)
	}))
	t.Cleanup(srv.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: srv.URL + "/api/"}},
	})
	req := mustReq(t, "GET", srv.URL+"/api/users")
	resp, err := d.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if got, want := string(resp.Body), "hello /api/users"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestSecureFetch_RejectsOutsidePrefix(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: srv.URL + "/api/"}},
	})
	req := mustReq(t, "GET", srv.URL+"/other")
	_, err := d.Do(context.Background(), req)
	var nad *NetworkAccessDeniedError
	if !errors.As(err, &nad) {
		t.Fatalf("err = %v, want *NetworkAccessDeniedError", err)
	}
}

func TestSecureFetch_DangerouslyAllowFullAccess(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)

	d := NewSecureFetch(&Config{
		DangerouslyAllowFullAccess: true,
		AllowedMethods:             []string{"GET"},
	})
	req := mustReq(t, "POST", srv.URL+"/anything")
	resp, err := d.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Status)
	}
}

func TestSecureFetch_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: "https://x"}},
		AllowedMethods:     []string{"GET"},
	})
	req := mustReq(t, "POST", "https://x/y")
	_, err := d.Do(context.Background(), req)
	var mna *MethodNotAllowedError
	if !errors.As(err, &mna) {
		t.Fatalf("err = %v, want *MethodNotAllowedError", err)
	}
	if mna.Method != "POST" {
		t.Errorf("Method = %q, want POST", mna.Method)
	}
}

func TestSecureFetch_ResponseTooLarge(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	t.Cleanup(srv.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: srv.URL}},
		MaxResponseSize:    50,
	})
	req := mustReq(t, "GET", srv.URL+"/")
	_, err := d.Do(context.Background(), req)
	var rtl *ResponseTooLargeError
	if !errors.As(err, &rtl) {
		t.Fatalf("err = %v, want *ResponseTooLargeError", err)
	}
	if rtl.Max != 50 {
		t.Errorf("Max = %d, want 50", rtl.Max)
	}
}

func TestSecureFetch_ResponseExactlyAtCap(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 50)))
	}))
	t.Cleanup(srv.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: srv.URL}},
		MaxResponseSize:    50,
	})
	req := mustReq(t, "GET", srv.URL+"/")
	resp, err := d.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body) != 50 {
		t.Errorf("body len = %d, want 50", len(resp.Body))
	}
}

func TestSecureFetch_TooManyRedirects(t *testing.T) {
	t.Parallel()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: srv.URL}},
		MaxRedirects:       3,
	})
	req := mustReq(t, "GET", srv.URL+"/")
	_, err := d.Do(context.Background(), req)
	var tmr *TooManyRedirectsError
	if !errors.As(err, &tmr) {
		t.Fatalf("err = %v, want *TooManyRedirectsError", err)
	}
	if tmr.Max != 3 {
		t.Errorf("Max = %d, want 3", tmr.Max)
	}
}

func TestSecureFetch_RedirectNotAllowed(t *testing.T) {
	t.Parallel()
	// Redirect to a host NOT in the allow-list.
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(denied.Close)

	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, denied.URL+"/elsewhere", http.StatusFound)
	}))
	t.Cleanup(src.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: src.URL}}, // only src
	})
	req := mustReq(t, "GET", src.URL+"/")
	_, err := d.Do(context.Background(), req)
	var rna *RedirectNotAllowedError
	if !errors.As(err, &rna) {
		t.Fatalf("err = %v, want *RedirectNotAllowedError", err)
	}
	if !strings.Contains(rna.To, "/elsewhere") {
		t.Errorf("To = %q, expected to contain /elsewhere", rna.To)
	}
}

func TestSecureFetch_HeaderTransformApplied(t *testing.T) {
	t.Parallel()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{
			URL: srv.URL,
			Transform: []RequestTransform{
				{Headers: map[string]string{"authorization": "Bearer t0ken"}},
			},
		}},
	})
	req := mustReq(t, "GET", srv.URL+"/")
	_, err := d.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer t0ken" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer t0ken")
	}
}

func TestSecureFetch_TimeoutCancels(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: srv.URL}},
		Timeout:            20 * time.Millisecond,
	})
	req := mustReq(t, "GET", srv.URL+"/")
	_, err := d.Do(context.Background(), req)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	// http.Client wraps timeouts in *url.Error → context.DeadlineExceeded.
	if !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "Timeout") && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("err = %v, expected timeout-shaped", err)
	}
}

func TestSecureFetch_DenyPrivateRanges_Loopback(t *testing.T) {
	t.Parallel()
	d := NewSecureFetch(&Config{
		AllowedURLPrefixes:         []AllowedURLEntry{{URL: "https://internal.example.com"}},
		DangerouslyAllowFullAccess: false,
		DenyPrivateRanges:          true,
		DNSResolve: func(ctx context.Context, host string) ([]DNSLookupResult, error) {
			return []DNSLookupResult{{Address: "127.0.0.1", Family: 4}}, nil
		},
	})
	req := mustReq(t, "GET", "https://internal.example.com/")
	_, err := d.Do(context.Background(), req)
	var nad *NetworkAccessDeniedError
	if !errors.As(err, &nad) {
		t.Fatalf("err = %v, want *NetworkAccessDeniedError (private range)", err)
	}
	if !strings.Contains(nad.Reason, "private") {
		t.Errorf("Reason = %q, want 'private' substring", nad.Reason)
	}
}

func TestSecureFetch_DenyPrivateRanges_RFC1918(t *testing.T) {
	t.Parallel()
	cases := []string{"10.0.0.1", "192.168.1.1", "172.16.5.5", "::1", "fe80::1"}
	for _, ip := range cases {
		t.Run(ip, func(t *testing.T) {
			t.Parallel()
			d := NewSecureFetch(&Config{
				AllowedURLPrefixes: []AllowedURLEntry{{URL: "https://x.example.com"}},
				DenyPrivateRanges:  true,
				DNSResolve: func(ctx context.Context, host string) ([]DNSLookupResult, error) {
					return []DNSLookupResult{{Address: ip, Family: 4}}, nil
				},
			})
			req := mustReq(t, "GET", "https://x.example.com/")
			_, err := d.Do(context.Background(), req)
			var nad *NetworkAccessDeniedError
			if !errors.As(err, &nad) {
				t.Fatalf("ip=%s: err = %v, want NetworkAccessDeniedError", ip, err)
			}
		})
	}
}

func TestSecureFetch_NilRequest(t *testing.T) {
	t.Parallel()
	d := NewSecureFetch(&Config{DangerouslyAllowFullAccess: true})
	_, err := d.Do(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}

func TestSecureFetch_NegativeMaxResponseSize_DisablesCap(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 1000)))
	}))
	t.Cleanup(srv.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: srv.URL}},
		MaxResponseSize:    -1,
	})
	req := mustReq(t, "GET", srv.URL+"/")
	resp, err := d.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body) != 1000 {
		t.Errorf("body len = %d, want 1000", len(resp.Body))
	}
}

func TestSecureFetch_FinalURLAfterRedirect(t *testing.T) {
	t.Parallel()
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "final")
	}))
	t.Cleanup(dest.Close)

	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL+"/final", http.StatusFound)
	}))
	t.Cleanup(src.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{
			{URL: src.URL},
			{URL: dest.URL},
		},
	})
	req := mustReq(t, "GET", src.URL+"/")
	resp, err := d.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(resp.URL, "/final") {
		t.Errorf("final URL = %q, want suffix /final", resp.URL)
	}
	if string(resp.Body) != "final" {
		t.Errorf("body = %q, want final", string(resp.Body))
	}
}

func TestSecureFetch_DisablesRedirects(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)

	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: srv.URL}},
		MaxRedirects:       -1,
	})
	req := mustReq(t, "GET", srv.URL+"/")
	resp, err := d.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != 302 {
		t.Errorf("status = %d, want 302", resp.Status)
	}
}

// Expected precedence follows just-bash src/network/fetch.ts:143–164,
// pinned by PARITY_AUDIT.md to commit 7537a260e38648e8998db7a95504102ad80b0194.
func TestSecureFetch_CumulativeTransforms(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s|%s|%s|%s|%s", r.Header.Get("Authorization"),
			r.Header.Get("X-Broad"), r.Header.Get("X-Narrow"),
			r.Header.Get("X-Unmatched"), r.Header.Get("X-Caller"))
	}))
	t.Cleanup(srv.Close)

	for _, fullAccess := range []bool{false, true} {
		t.Run(fmt.Sprintf("fullAccess=%v", fullAccess), func(t *testing.T) {
			d := NewSecureFetch(&Config{
				DangerouslyAllowFullAccess: fullAccess,
				AllowedURLPrefixes: []AllowedURLEntry{
					{URL: srv.URL + "/api/", Transform: []RequestTransform{
						{Headers: map[string]string{"Authorization": "narrow", "X-Narrow": "yes"}},
					}},
					// Later wins, even when the later prefix is less specific.
					{URL: srv.URL, Transform: []RequestTransform{
						{Headers: map[string]string{"authorization": "broad", "X-Broad": "yes"}},
						{Headers: map[string]string{"AUTHORIZATION": "last"}},
					}},
					{URL: srv.URL + "/other/", Transform: []RequestTransform{
						{Headers: map[string]string{"Authorization": "wrong path", "X-Unmatched": "bad"}},
					}},
					{URL: "https://other.example.com/api/", Transform: []RequestTransform{
						{Headers: map[string]string{"Authorization": "wrong origin", "X-Unmatched": "bad"}},
					}},
				},
			})
			req := mustReq(t, "GET", srv.URL+"/api/users")
			req.Header.Set("Authorization", "caller")
			req.Header.Set("X-Caller", "preserved")
			resp, err := d.Do(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(resp.Body), "last|yes|yes||preserved"; got != want {
				t.Errorf("headers = %q, want %q", got, want)
			}
			if req.Header.Get("Authorization") != "caller" || req.Header.Get("X-Broad") != "" {
				t.Errorf("caller headers mutated: %v", req.Header)
			}
		})
	}
}

func TestSecureFetch_TransformsRecomputedOnRedirect(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			if r.Header.Get("Authorization") != "start-secret" || r.Header.Get("X-Shared") != "start" {
				http.Error(w, "missing initial transforms", http.StatusBadRequest)
				return
			}
			http.Redirect(w, r, "/end", http.StatusFound)
			return
		}
		_, _ = fmt.Fprintf(w, "%s|%s|%s", r.Header.Get("Authorization"), r.Header.Get("X-Shared"), r.Header.Get("X-End"))
	}))
	t.Cleanup(srv.Close)
	d := NewSecureFetch(&Config{AllowedURLPrefixes: []AllowedURLEntry{
		{URL: srv.URL, Transform: []RequestTransform{
			{Headers: map[string]string{"X-Shared": "base"}},
		}},
		{URL: srv.URL + "/start", Transform: []RequestTransform{
			{Headers: map[string]string{"Authorization": "start-secret", "X-Shared": "start"}},
		}},
		{URL: srv.URL + "/end", Transform: []RequestTransform{
			{Headers: map[string]string{"X-Shared": "end", "X-End": "yes"}},
		}},
	}})
	resp, err := d.Do(context.Background(), mustReq(t, "GET", srv.URL+"/start"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(resp.Body), "|end|yes"; got != want {
		t.Errorf("redirect headers = %q, want %q (no start credential)", got, want)
	}
}

func TestSecureFetch_ExplicitEmptyAllowedMethods(t *testing.T) {
	t.Parallel()
	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: "https://example.com"}},
		AllowedMethods:     []string{},
	})
	for _, method := range []string{"GET", "HEAD", "POST"} {
		_, err := d.Do(context.Background(), mustReq(t, method, "https://example.com/"))
		var denied *MethodNotAllowedError
		if !errors.As(err, &denied) || denied.Method != method {
			t.Errorf("method %s: err = %v, want MethodNotAllowedError", method, err)
		}
	}
}
