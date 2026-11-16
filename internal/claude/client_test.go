package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/ferry/internal/engine"
	"github.com/dcadolph/ferry/internal/logutil"
)

// testKey is the API key every test client sends. No test reads a real key.
const testKey = "test-key"

// fallbackBeta is the beta header value that turns on server-side refusal fallbacks.
const fallbackBeta = "server-side-fallback-2026-07-01"

// testRequest is one request the test server received.
type testRequest struct {
	// Path is the request path.
	Path string
	// Header holds the request headers.
	Header http.Header
	// Body holds the raw request body.
	Body []byte
}

// testServer records the requests it receives.
type testServer struct {
	// URL is the base URL of the server.
	URL string
	// mu guards requests.
	mu sync.Mutex
	// requests lists the requests received, in order.
	requests []testRequest
}

// newTestServer starts a local server that records each request and answers it with reply.
func newTestServer(t *testing.T, reply http.HandlerFunc) *testServer {
	t.Helper()
	ts := &testServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		ts.mu.Lock()
		ts.requests = append(ts.requests, testRequest{
			Path: r.URL.Path, Header: r.Header.Clone(), Body: body,
		})
		ts.mu.Unlock()
		reply(w, r)
	}))
	t.Cleanup(srv.Close)
	ts.URL = srv.URL
	return ts
}

// Requests returns a copy of the requests received so far.
func (ts *testServer) Requests() []testRequest {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return slices.Clone(ts.requests)
}

// testReply returns a handler that answers with status and a JSON body.
func testReply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// testHang returns a handler that answers only after the client gives up.
func testHang() http.HandlerFunc {
	return func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}
}

// testMessage returns a Messages API reply with stopReason and the JSON-encoded content and
// stop details.
func testMessage(stopReason, content, stopDetails string) string {
	return fmt.Sprintf(`{"id":"msg_test","type":"message","role":"assistant",`+
		`"model":"claude-fable-5-2","content":%s,"stop_reason":%q,"stop_sequence":null,`+
		`"stop_details":%s,"usage":{"input_tokens":12,"output_tokens":34}}`,
		content, stopReason, stopDetails)
}

// testAPIError returns a Messages API error body with errType and message.
func testAPIError(errType, message string) string {
	return fmt.Sprintf(`{"type":"error","error":{"type":%q,"message":%q},"request_id":"req_test"}`,
		errType, message)
}

// newTestClient returns a Client that sends to baseURL with the test key. It makes no retries
// unless extra says otherwise.
func newTestClient(t *testing.T, opts Options, baseURL string, extra ...option.RequestOption) *Client {
	t.Helper()
	seam := []option.RequestOption{
		option.WithBaseURL(baseURL), option.WithAPIKey(testKey), option.WithMaxRetries(0),
	}
	c, err := newClient(opts, append(seam, extra...)...)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	return c
}

// TestNewClient checks that the client fills empty options with the defaults, keeps set ones,
// and rejects options it cannot run with.
func TestNewClient(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantOptions Options
		Want        error
		Options     Options
	}{{ // Test 0: Empty fields take the defaults.
		Options: Options{},
		WantOptions: Options{
			Model: DefaultModel, Effort: DefaultEffort, MaxTokens: DefaultMaxTokens,
		},
	}, { // Test 1: Set fields are kept.
		Options: Options{
			Model: "claude-sonnet-5-5", Effort: "xhigh", MaxTokens: 4096,
			Timeout: 2 * time.Minute,
		},
		WantOptions: Options{
			Model: "claude-sonnet-5-5", Effort: "xhigh", MaxTokens: 4096,
			Timeout: 2 * time.Minute,
		},
	}, { // Test 2: An unknown effort gives ErrEffort.
		Options: Options{Effort: "extreme"}, Want: ErrEffort,
	}, { // Test 3: Effort names are case sensitive.
		Options: Options{Effort: "High"}, Want: ErrEffort,
	}, { // Test 4: A negative token cap gives ErrMaxTokens.
		Options: Options{MaxTokens: -1}, Want: ErrMaxTokens,
	}, { // Test 5: A negative timeout gives ErrTimeout.
		Options: Options{Timeout: -time.Second}, Want: ErrTimeout,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			c, err := newClient(test.Options, option.WithAPIKey(testKey))
			if !errors.Is(err, test.Want) {
				t.Fatalf("newClient() error = %v, want %v", err, test.Want)
			}
			if err != nil {
				if strings.Contains(err.Error(), testKey) {
					t.Errorf("newClient() error %q holds the API key", err)
				}
				return
			}
			if diff := cmp.Diff(test.WantOptions, c.opts, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("options mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantOptions.Effort, string(c.effort)); diff != "" {
				t.Errorf("effort mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNew checks that New rejects bad options the way the client does. The cases stop before
// the SDK client is built, so no test resolves real credentials.
func TestNew(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want    error
		Options Options
	}{{ // Test 0: An unknown effort.
		Options: Options{Effort: "extreme"}, Want: ErrEffort,
	}, { // Test 1: A negative token cap.
		Options: Options{MaxTokens: -1}, Want: ErrMaxTokens,
	}, { // Test 2: A negative timeout.
		Options: Options{Timeout: -time.Second}, Want: ErrTimeout,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			c, err := New(test.Options)
			if !errors.Is(err, test.Want) {
				t.Fatalf("New() error = %v, want %v", err, test.Want)
			}
			if c != nil {
				t.Errorf("New() returned a client with an error")
			}
		})
	}
}

// TestCheckEffort checks that every level Claude accepts passes and anything else fails.
func TestCheckEffort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want error
		In   string
	}{{ // Test 0: Low.
		In: "low", Want: nil,
	}, { // Test 1: Medium.
		In: "medium", Want: nil,
	}, { // Test 2: High, the default.
		In: "high", Want: nil,
	}, { // Test 3: Extra high.
		In: "xhigh", Want: nil,
	}, { // Test 4: Max.
		In: "max", Want: nil,
	}, { // Test 5: An unknown level is rejected.
		In: "extreme", Want: ErrEffort,
	}, { // Test 6: Empty is rejected.
		In: "", Want: ErrEffort,
	}, { // Test 7: Case matters here, the command lowers it first.
		In: "HIGH", Want: ErrEffort,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			err := CheckEffort(test.In)
			if !errors.Is(err, test.Want) {
				t.Fatalf("CheckEffort(%q) = %v, want %v", test.In, err, test.Want)
			}
			if err != nil && !strings.Contains(err.Error(), "low, medium, high, xhigh, max") {
				t.Errorf("CheckEffort(%q) error %q does not list the levels", test.In, err)
			}
		})
	}
}

// TestEfforts checks the list of effort levels and that a caller cannot change it.
func TestEfforts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult []string
	}{{ // Test 0: The five levels, lowest first.
		WantResult: []string{"low", "medium", "high", "xhigh", "max"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := Efforts()
			if diff := cmp.Diff(test.WantResult, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			got[0] = "changed"
			if diff := cmp.Diff(test.WantResult, Efforts()); diff != "" {
				t.Errorf("mismatch after a caller changed its copy (-want +got):\n%s", diff)
			}
		})
	}
}

// TestMaxTokensFor checks that the two highest efforts get the larger reply cap.
func TestMaxTokensFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult int64
	}{{ // Test 0: Low.
		In: "low", WantResult: 16000,
	}, { // Test 1: Medium.
		In: "medium", WantResult: 16000,
	}, { // Test 2: High, the default.
		In: "high", WantResult: 16000,
	}, { // Test 3: Extra high gets room for thinking.
		In: "xhigh", WantResult: 32000,
	}, { // Test 4: Max gets room for thinking.
		In: "max", WantResult: 32000,
	}, { // Test 5: An unknown effort gets the default.
		In: "extreme", WantResult: 16000,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, MaxTokensFor(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestFallbackModel checks which models take the server-side refusal fallback.
func TestFallbackModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Model        string
		WantFallback bool
	}{{ // Test 0: Claude Fable 5.2 takes fallbacks.
		Model: "claude-fable-5-2", WantFallback: true,
	}, { // Test 1: Claude Fable 5.1 takes fallbacks.
		Model: "claude-fable-5-1", WantFallback: true,
	}, { // Test 2: Claude Opus 5.5 takes fallbacks.
		Model: "claude-opus-5-5", WantFallback: true,
	}, { // Test 3: Claude Opus 5 takes fallbacks.
		Model: "claude-opus-5", WantFallback: true,
	}, { // Test 4: Claude Sonnet 5.5 takes fallbacks.
		Model: "claude-sonnet-5-5", WantFallback: true,
	}, { // Test 5: Claude Haiku 5.5 does not.
		Model: "claude-haiku-5-5",
	}, { // Test 6: Claude Sonnet 5 does not.
		Model: "claude-sonnet-5",
	}, { // Test 7: An empty model does not.
		Model: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := fallbackModel(test.Model); got != test.WantFallback {
				t.Errorf("fallbackModel(%q) = %t, want %t", test.Model, got, test.WantFallback)
			}
		})
	}
}

// TestCompleteRequest checks the request each kind of client sends: the body, the fallback
// header, the key, and that the schema goes out byte for byte, so its properties keep the
// order the engine gave them.
func TestCompleteRequest(t *testing.T) {
	t.Parallel()
	rawSchema := json.RawMessage(`{"type":"object","properties":{"zeta":{"type":"boolean"},` +
		`"alpha":{"type":"string"}},"required":["zeta","alpha"],"additionalProperties":false}`)
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"zeta": map[string]any{"type": "boolean"}, "alpha": map[string]any{"type": "string"},
		},
		"required":             []any{"zeta", "alpha"},
		"additionalProperties": false,
	}
	system := []any{map[string]any{
		"type": "text", "text": "Stay on task.",
		"cache_control": map[string]any{"type": "ephemeral"},
	}}
	messages := []any{map[string]any{
		"role": "user", "content": []any{map[string]any{"type": "text", "text": "Draft one."}},
	}}
	full := engine.Request{
		Step: engine.StepDraft, System: "Stay on task.", Prompt: "Draft one.", Schema: rawSchema,
	}
	format := map[string]any{"type": "json_schema", "schema": schema}
	tests := []struct {
		WantBody     map[string]any
		Request      engine.Request
		Options      Options
		WantFallback bool
	}{{ // Test 0: The defaults send Claude Opus 5.5 at high effort with fallbacks.
		Request: full,
		WantBody: map[string]any{
			"model": "claude-opus-5-5", "max_tokens": float64(16000),
			"system": system, "messages": messages,
			"output_config": map[string]any{"effort": "high", "format": format},
			"fallbacks":     "default",
		},
		WantFallback: true,
	}, { // Test 1: Claude Haiku 5.5 sends no fallbacks and no beta header.
		Options: Options{Model: "claude-haiku-5-5", Effort: "low", MaxTokens: 2048},
		Request: full,
		WantBody: map[string]any{
			"model": "claude-haiku-5-5", "max_tokens": float64(2048),
			"system": system, "messages": messages,
			"output_config": map[string]any{"effort": "low", "format": format},
		},
	}, { // Test 2: Claude Fable 5.2 at max effort sends fallbacks.
		Options: Options{Model: "claude-fable-5-2", Effort: "max"},
		Request: full,
		WantBody: map[string]any{
			"model": "claude-fable-5-2", "max_tokens": float64(16000),
			"system": system, "messages": messages,
			"output_config": map[string]any{"effort": "max", "format": format},
			"fallbacks":     "default",
		},
		WantFallback: true,
	}, { // Test 3: Claude Opus 5 at xhigh effort sends fallbacks.
		Options: Options{Model: "claude-opus-5", Effort: "xhigh"},
		Request: full,
		WantBody: map[string]any{
			"model": "claude-opus-5", "max_tokens": float64(16000),
			"system": system, "messages": messages,
			"output_config": map[string]any{"effort": "xhigh", "format": format},
			"fallbacks":     "default",
		},
		WantFallback: true,
	}, { // Test 4: An empty system prompt and schema send neither.
		Options: Options{Model: "claude-haiku-5-5", Effort: "medium"},
		Request: engine.Request{Prompt: "Draft one."},
		WantBody: map[string]any{
			"model": "claude-haiku-5-5", "max_tokens": float64(16000), "messages": messages,
			"output_config": map[string]any{"effort": "medium"},
		},
	}, { // Test 5: The larger reply cap goes out when a timeout is set.
		Options: Options{
			Model: "claude-haiku-5-5", Effort: "max",
			MaxTokens: DeepMaxTokens, Timeout: time.Minute,
		},
		Request: full,
		WantBody: map[string]any{
			"model": "claude-haiku-5-5", "max_tokens": float64(32000),
			"system": system, "messages": messages,
			"output_config": map[string]any{"effort": "max", "format": format},
		},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			reply := testMessage("end_turn", `[{"type":"text","text":"{\"ok\":true}"}]`, "null")
			srv := newTestServer(t, testReply(http.StatusOK, reply))
			c := newTestClient(t, test.Options, srv.URL)
			if _, err := c.Complete(context.Background(), test.Request); err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			reqs := srv.Requests()
			if len(reqs) != 1 {
				t.Fatalf("server got %d requests, want 1", len(reqs))
			}
			if diff := cmp.Diff("/v1/messages", reqs[0].Path); diff != "" {
				t.Errorf("path mismatch (-want +got):\n%s", diff)
			}
			var body map[string]any
			if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if diff := cmp.Diff(test.WantBody, body, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("request body mismatch (-want +got):\n%s", diff)
			}
			if len(test.Request.Schema) > 0 && !bytes.Contains(reqs[0].Body, rawSchema) {
				t.Errorf("request body does not hold the schema byte for byte:\n%s",
					reqs[0].Body)
			}
			beta := strings.Join(reqs[0].Header.Values("Anthropic-Beta"), ",")
			if got := strings.Contains(beta, fallbackBeta); got != test.WantFallback {
				t.Errorf("anthropic-beta %q has %s: %t, want %t", beta, fallbackBeta, got,
					test.WantFallback)
			}
			auth := reqs[0].Header.Get("Authorization") + reqs[0].Header.Get("X-Api-Key")
			if !strings.Contains(auth, testKey) {
				t.Errorf("request did not carry the API key")
			}
		})
	}
}

// TestCompleteReply checks how each kind of reply and each HTTP status comes back: the text,
// or the error that names what went wrong, with one request sent and no key in the message.
func TestCompleteReply(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want         error
		Body         string
		WantText     string
		WantContains string
		Status       int
	}{{ // Test 0: Text blocks are joined and other blocks are skipped.
		Status: http.StatusOK,
		Body: testMessage("end_turn", `[{"type":"thinking","thinking":"","signature":"sig"},`+
			`{"type":"text","text":"{\"ok\":"},{"type":"text","text":"true}"}]`, "null"),
		WantText: `{"ok":true}`,
	}, { // Test 1: A refusal gives ErrRefused with its category and explanation.
		Status: http.StatusOK,
		Body: testMessage("refusal", `[]`, `{"type":"refusal","category":"cyber",`+
			`"explanation":"This request was declined.","fallback_credit_token":null,`+
			`"fallback_has_prefill_claim":false,"recommended_model":null}`),
		Want:         ErrRefused,
		WantContains: "category cyber: This request was declined.",
	}, { // Test 2: A refusal without details still gives ErrRefused.
		Status:       http.StatusOK,
		Body:         testMessage("refusal", `[]`, "null"),
		Want:         ErrRefused,
		WantContains: "category none: no explanation given",
	}, { // Test 3: Hitting max tokens gives ErrTruncated.
		Status:       http.StatusOK,
		Body:         testMessage("max_tokens", `[{"type":"text","text":"{\"ok\":"}]`, "null"),
		Want:         ErrTruncated,
		WantContains: "claude reply truncated: reached max tokens 16000: lower --effort",
	}, { // Test 4: Hitting the context window gives ErrTruncated.
		Status: http.StatusOK,
		Body: testMessage("model_context_window_exceeded",
			`[{"type":"text","text":"{\"ok\":"}]`, "null"),
		Want: ErrTruncated,
	}, { // Test 5: A reply with no content gives ErrEmpty.
		Status: http.StatusOK,
		Body:   testMessage("end_turn", `[]`, "null"),
		Want:   ErrEmpty,
	}, { // Test 6: A reply with only white space gives ErrEmpty.
		Status: http.StatusOK,
		Body:   testMessage("end_turn", `[{"type":"text","text":" \n"}]`, "null"),
		Want:   ErrEmpty,
	}, { // Test 7: A 401 gives ErrAuth with the credential hint.
		Status:       http.StatusUnauthorized,
		Body:         testAPIError("authentication_error", "invalid x-api-key"),
		Want:         ErrAuth,
		WantContains: "set ANTHROPIC_API_KEY or run ant auth login",
	}, { // Test 8: A 403 gives ErrAuth.
		Status: http.StatusForbidden,
		Body:   testAPIError("permission_error", "no access to this model"),
		Want:   ErrAuth,
	}, { // Test 9: A 429 gives ErrRateLimit without a retry.
		Status: http.StatusTooManyRequests,
		Body:   testAPIError("rate_limit_error", "rate limit exceeded"),
		Want:   ErrRateLimit,
	}, { // Test 10: A 500 gives ErrUnavailable.
		Status: http.StatusInternalServerError,
		Body:   testAPIError("api_error", "internal server error"),
		Want:   ErrUnavailable,
	}, { // Test 11: A 529 gives ErrUnavailable.
		Status: 529,
		Body:   testAPIError("overloaded_error", "overloaded"),
		Want:   ErrUnavailable,
	}, { // Test 12: A 400 gives ErrAPI.
		Status:       http.StatusBadRequest,
		Body:         testAPIError("invalid_request_error", "messages: field required"),
		Want:         ErrAPI,
		WantContains: "messages: field required",
	}, { // Test 13: A 404 gives ErrAPI.
		Status: http.StatusNotFound,
		Body:   testAPIError("not_found_error", "model not found"),
		Want:   ErrAPI,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv := newTestServer(t, testReply(test.Status, test.Body))
			c := newTestClient(t, Options{}, srv.URL)
			got, err := c.Complete(context.Background(), engine.Request{
				System: "Stay on task.", Prompt: "Draft one.",
			})
			if !errors.Is(err, test.Want) {
				t.Fatalf("Complete() error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantText, got); diff != "" {
				t.Errorf("reply mismatch (-want +got):\n%s", diff)
			}
			if n := len(srv.Requests()); n != 1 {
				t.Errorf("server got %d requests, want 1", n)
			}
			if err == nil {
				return
			}
			if strings.Contains(err.Error(), testKey) {
				t.Errorf("Complete() error %q holds the API key", err)
			}
			if !strings.Contains(err.Error(), test.WantContains) {
				t.Errorf("Complete() error %q does not mention %q", err, test.WantContains)
			}
		})
	}
}

// TestCompleteTransport checks the failures that never get an HTTP answer: a canceled
// context, a call that runs out of time, and a refused connection.
func TestCompleteTransport(t *testing.T) {
	t.Parallel()
	ok := testReply(http.StatusOK,
		testMessage("end_turn", `[{"type":"text","text":"{\"ok\":true}"}]`, "null"))
	tests := []struct {
		Want     error
		Handler  http.HandlerFunc
		WantText string
		Options  Options
		Cancel   bool
	}{{ // Test 0: A canceled context passes through.
		Handler: ok, Cancel: true, Want: context.Canceled,
		WantText: "claude request stopped: context canceled",
	}, { // Test 1: The time limit stops a slow call, and the error says what to change.
		Handler: testHang(), Options: Options{Timeout: 50 * time.Millisecond},
		Want:     context.DeadlineExceeded,
		WantText: "timed out after 50ms: raise --timeout or lower --effort",
	}, { // Test 2: A refused connection gives ErrRequest.
		Want: ErrRequest,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var baseURL string
			if test.Handler == nil {
				srv := httptest.NewServer(http.NotFoundHandler())
				baseURL = srv.URL
				srv.Close()
			} else {
				baseURL = newTestServer(t, test.Handler).URL
			}
			c := newTestClient(t, test.Options, baseURL)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.Cancel {
				cancel()
			}
			_, err := c.Complete(ctx, engine.Request{Prompt: "Draft one."})
			if !errors.Is(err, test.Want) {
				t.Fatalf("Complete() error = %v, want %v", err, test.Want)
			}
			if !errors.Is(test.Want, ErrRequest) && errors.Is(err, ErrRequest) {
				t.Errorf("Complete() error %v is ErrRequest, want the context error alone", err)
			}
			if strings.Contains(err.Error(), testKey) {
				t.Errorf("Complete() error %q holds the API key", err)
			}
			if test.WantText != "" {
				if diff := cmp.Diff(test.WantText, err.Error()); diff != "" {
					t.Errorf("message mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

// testFlaky returns a handler that answers the first fails requests with status and a short
// retry delay, and every later one with a good reply.
func testFlaky(fails, status int) http.HandlerFunc {
	var mu sync.Mutex
	seen := 0
	ok := testReply(http.StatusOK,
		testMessage("end_turn", `[{"type":"text","text":"{\"ok\":true}"}]`, "null"))
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen++
		n := seen
		mu.Unlock()
		if n > fails {
			ok(w, r)
			return
		}
		w.Header().Set("Retry-After-Ms", "5")
		testReply(status, testAPIError("api_error", "try again"))(w, r)
	}
}

// TestCompleteTimeLimit checks that the timeout is one limit on the whole call with the SDK's
// retries left on: a call that hangs is sent once and ends at the limit, and a call that is
// rate limited or hits a server error is still retried inside it.
func TestCompleteTimeLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want         error
		Handler      http.HandlerFunc
		WantText     string
		Timeout      time.Duration
		WantRequests int
	}{{ // Test 0: A hung call is sent once, not three times, and ends at the limit.
		Handler: testHang(), Timeout: 300 * time.Millisecond,
		WantRequests: 1, Want: context.DeadlineExceeded,
		WantText: "timed out after 300ms: raise --timeout or lower --effort",
	}, { // Test 1: A rate limit is retried inside the limit and the call succeeds.
		Handler: testFlaky(1, http.StatusTooManyRequests), Timeout: 30 * time.Second,
		WantRequests: 2, Want: nil,
	}, { // Test 2: Two server errors are retried inside the limit and the call succeeds.
		Handler: testFlaky(2, http.StatusInternalServerError), Timeout: 30 * time.Second,
		WantRequests: 3, Want: nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv := newTestServer(t, test.Handler)
			c := newTestClient(t, Options{Timeout: test.Timeout}, srv.URL,
				option.WithMaxRetries(2))
			start := time.Now()
			_, err := c.Complete(context.Background(), engine.Request{Prompt: "Draft one."})
			took := time.Since(start)
			if !errors.Is(err, test.Want) {
				t.Fatalf("Complete() error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantRequests, len(srv.Requests())); diff != "" {
				t.Errorf("request count mismatch (-want +got):\n%s", diff)
			}
			if err == nil {
				return
			}
			if diff := cmp.Diff(test.WantText, err.Error()); diff != "" {
				t.Errorf("message mismatch (-want +got):\n%s", diff)
			}
			if took > 2*time.Second {
				t.Errorf("call took %s, want it to end near the %s limit", took, test.Timeout)
			}
		})
	}
}

// TestCompleteLogs checks the debug line each call writes to the logger in its context: one
// line, with the step, the stop reason, and the token counts, and without the prompt, the
// system prompt, the reply, or the key.
func TestCompleteLogs(t *testing.T) {
	t.Parallel()
	const (
		system = "System marker zebra-stripes."
		prompt = "Prompt marker walrus-tusks."
		answer = "reply-marker-heron-legs"
	)
	tests := []struct {
		Want      error
		Body      string
		WantLines []string
		Status    int
	}{{ // Test 0: A good reply logs its stop reason and token counts.
		Status: http.StatusOK,
		Body:   testMessage("end_turn", `[{"type":"text","text":"`+answer+`"}]`, "null"),
		WantLines: []string{
			"debug\tmodel call\t", `"step": "draft"`, `"model": "claude-fable-5-2"`,
			`"stop_reason": "end_turn"`, `"input_tokens": 12`, `"output_tokens": 34`, `"elapsed"`,
		},
	}, { // Test 1: A truncated reply still logs what it cost.
		Status: http.StatusOK,
		Body:   testMessage("max_tokens", `[{"type":"text","text":"`+answer+`"}]`, "null"),
		Want:   ErrTruncated,
		WantLines: []string{
			"debug\tmodel call\t", `"stop_reason": "max_tokens"`, `"output_tokens": 34`,
		},
	}, { // Test 2: A rejected key logs the failure and never the key.
		Status: http.StatusUnauthorized,
		Body:   testAPIError("authentication_error", "invalid x-api-key"),
		Want:   ErrAuth,
		WantLines: []string{
			"debug\tmodel call failed\t", `"step": "draft"`, `"elapsed"`, "claude auth rejected",
		},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			log, err := logutil.New(&buf, "debug")
			if err != nil {
				t.Fatalf("logutil.New: %v", err)
			}
			srv := newTestServer(t, testReply(test.Status, test.Body))
			c := newTestClient(t, Options{Timeout: time.Minute}, srv.URL)
			ctx := logutil.WrapLogger(context.Background(), log)
			_, err = c.Complete(ctx, engine.Request{
				Step: engine.StepDraft, System: system, Prompt: prompt,
			})
			if !errors.Is(err, test.Want) {
				t.Fatalf("Complete() error = %v, want %v", err, test.Want)
			}
			got := buf.String()
			if diff := cmp.Diff(1, strings.Count(got, "\n")); diff != "" {
				t.Errorf("log line count mismatch (-want +got):\n%s\n%s", diff, got)
			}
			for _, want := range test.WantLines {
				if !strings.Contains(got, want) {
					t.Errorf("log lacks %q:\n%s", want, got)
				}
			}
			for _, secret := range []string{"zebra-stripes", "walrus-tusks", answer, testKey} {
				if strings.Contains(got, secret) {
					t.Errorf("log holds %q:\n%s", secret, got)
				}
			}
		})
	}
}
