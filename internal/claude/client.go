// Package claude sends engine requests to Claude through the Anthropic Go SDK.
package claude

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"go.uber.org/zap"

	"github.com/dcadolph/ferry/internal/engine"
	"github.com/dcadolph/ferry/internal/logutil"
)

// Defaults for the Claude client.
const (
	// DefaultModel is the model called when Options.Model is empty.
	DefaultModel = "claude-fable-5-2"
	// DefaultEffort is the effort used when Options.Effort is empty.
	DefaultEffort = "high"
	// DefaultMaxTokens is the reply cap used when Options.MaxTokens is zero. It suits efforts
	// up to high.
	DefaultMaxTokens = 16000
	// DeepMaxTokens is the reply cap for the xhigh and max efforts, where thinking takes more
	// of the cap. The SDK accepts a cap this size on a request that is not streamed only when
	// Options.Timeout is set.
	DeepMaxTokens = 32000
)

// authHint tells the user how to give the SDK a credential.
const authHint = "set ANTHROPIC_API_KEY or run ant auth login"

// Options holds the settings for a Client.
type Options struct {
	// Model is the Claude model ID to call. Empty means DefaultModel.
	Model string
	// Effort is the reasoning effort: low, medium, high, xhigh, or max. Empty means DefaultEffort.
	Effort string
	// MaxTokens caps the length of each reply. Zero means DefaultMaxTokens.
	MaxTokens int64
	// Timeout is the total time one call to Complete may take, the SDK's retries included.
	// Zero keeps the SDK default, which limits each attempt and not the call.
	Timeout time.Duration
}

// Client sends engine requests to Claude.
type Client struct {
	// opts holds the client's settings with the defaults filled.
	opts Options
	// effort is opts.Effort as the SDK's effort value.
	effort anthropic.BetaOutputConfigEffort
	// sdk is the Anthropic SDK client that sends each request.
	sdk anthropic.Client
}

// Client satisfies the engine's Completer interface.
var _ engine.Completer = (*Client)(nil)

// New returns a Client for opts, filling empty fields with the defaults. The SDK resolves the
// credentials on its own: ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN, or an ant auth login profile.
func New(opts Options) (*Client, error) {
	return newClient(opts)
}

// Efforts returns the reasoning effort levels Claude accepts, lowest first.
func Efforts() []string {
	return []string{
		string(anthropic.BetaOutputConfigEffortLow),
		string(anthropic.BetaOutputConfigEffortMedium),
		string(anthropic.BetaOutputConfigEffortHigh),
		string(anthropic.BetaOutputConfigEffortXhigh),
		string(anthropic.BetaOutputConfigEffortMax),
	}
}

// CheckEffort returns ErrEffort when effort is not one of Efforts. Effort names are lowercase.
func CheckEffort(effort string) error {
	if slices.Contains(Efforts(), effort) {
		return nil
	}
	return fmt.Errorf("%w: %q: want one of %s", ErrEffort, effort, strings.Join(Efforts(), ", "))
}

// MaxTokensFor returns the reply cap that suits effort: DeepMaxTokens for xhigh and max, and
// DefaultMaxTokens for everything else.
func MaxTokensFor(effort string) int64 {
	switch anthropic.BetaOutputConfigEffort(effort) {
	case anthropic.BetaOutputConfigEffortXhigh, anthropic.BetaOutputConfigEffortMax:
		return DeepMaxTokens
	default:
		return DefaultMaxTokens
	}
}

// newClient returns a Client for opts and hands extra to the SDK after its own options. Tests
// use extra to point the client at a local server with a test key and to turn off retries.
func newClient(opts Options, extra ...option.RequestOption) (*Client, error) {
	if opts.Model == "" {
		opts.Model = DefaultModel
	}
	if opts.Effort == "" {
		opts.Effort = DefaultEffort
	}
	if opts.MaxTokens == 0 {
		opts.MaxTokens = DefaultMaxTokens
	}
	if err := CheckEffort(opts.Effort); err != nil {
		return nil, err
	}
	if opts.MaxTokens <= 0 {
		return nil, fmt.Errorf("%w: %d: must be positive", ErrMaxTokens, opts.MaxTokens)
	}
	if opts.Timeout < 0 {
		return nil, fmt.Errorf("%w: %s: must not be negative", ErrTimeout, opts.Timeout)
	}
	// The SDK applies this timeout to each attempt, and without it falls back to its own
	// default. Complete adds the limit on the whole call.
	var reqOpts []option.RequestOption
	if opts.Timeout > 0 {
		reqOpts = append(reqOpts, option.WithRequestTimeout(opts.Timeout))
	}
	sdk := anthropic.NewClient(append(reqOpts, extra...)...)
	effort := anthropic.BetaOutputConfigEffort(opts.Effort)
	return &Client{opts: opts, effort: effort, sdk: sdk}, nil
}

// Complete sends req to Claude and returns the reply text. The reply must match req.Schema
// when it is set, and an empty req.System sends no system prompt. Options.Timeout limits the
// whole call: the SDK still retries a rate limit or a server error, but only inside that
// limit, and a call that runs out of time is not sent again. Complete writes one debug line
// for each call to the logger ctx carries, with the step, the model, the stop reason, the
// token counts, and the time taken, and never the prompt, the reply, or a credential.
func (c *Client) Complete(ctx context.Context, req engine.Request) (string, error) {
	log := logutil.UnwrapLogger(ctx)
	callCtx := ctx
	if c.opts.Timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, c.opts.Timeout)
		defer cancel()
	}
	start := time.Now()
	msg, err := c.sdk.Beta.Messages.New(callCtx, c.params(req))
	if err != nil {
		if c.opts.Timeout > 0 && ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
			err = &timeoutError{limit: c.opts.Timeout}
		} else {
			err = classify(err)
		}
		log.Debug("model call failed",
			zap.String("step", string(req.Step)),
			zap.String("model", c.opts.Model),
			zap.Duration("elapsed", time.Since(start)),
			zap.Error(err),
		)
		return "", err
	}
	if msg != nil {
		log.Debug("model call",
			zap.String("step", string(req.Step)),
			zap.String("model", msg.Model),
			zap.String("stop_reason", string(msg.StopReason)),
			zap.Int64("input_tokens", msg.Usage.InputTokens),
			zap.Int64("output_tokens", msg.Usage.OutputTokens),
			zap.Duration("elapsed", time.Since(start)),
		)
	}
	return c.reply(msg)
}

// params builds the SDK request for req. Thinking stays unset because adaptive thinking is the
// default on current models and other settings are rejected.
func (c *Client) params(req engine.Request) anthropic.BetaMessageNewParams {
	params := anthropic.BetaMessageNewParams{
		Model:     c.opts.Model,
		MaxTokens: c.opts.MaxTokens,
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(req.Prompt)),
		},
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: c.effort},
	}
	if req.System != "" {
		params.System = []anthropic.BetaTextBlockParam{{
			Text:         req.System,
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		}}
	}
	if len(req.Schema) > 0 {
		params.OutputConfig.Format = anthropic.BetaJSONOutputFormatParam{Schema: req.Schema}
	}
	if fallbackModel(c.opts.Model) {
		params.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
		params.Betas = []anthropic.AnthropicBeta{
			anthropic.AnthropicBetaServerSideFallback2026_07_01,
		}
	}
	return params
}

// fallbackModel reports whether model takes the server-side refusal fallback.
func fallbackModel(model string) bool {
	switch model {
	case "claude-fable-5-2", "claude-fable-5-1", "claude-opus-5-5", "claude-opus-5",
		"claude-sonnet-5-5":
		return true
	default:
		return false
	}
}

// reply returns the joined text blocks of msg, or the error its stop reason calls for.
func (c *Client) reply(msg *anthropic.BetaMessage) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("%w: no message", ErrEmpty)
	}
	switch msg.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return "", refusal(msg.StopDetails)
	case anthropic.BetaStopReasonMaxTokens:
		return "", fmt.Errorf("%w: reached max tokens %d: lower --effort",
			ErrTruncated, c.opts.MaxTokens)
	case anthropic.BetaStopReasonModelContextWindowExceeded:
		return "", fmt.Errorf("%w: reached the model context window", ErrTruncated)
	}
	var b strings.Builder
	for _, block := range msg.Content {
		if text, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			b.WriteString(text.Text)
		}
	}
	if strings.TrimSpace(b.String()) == "" {
		return "", fmt.Errorf("%w: stop reason %s", ErrEmpty, msg.StopReason)
	}
	return b.String(), nil
}

// refusal returns ErrRefused with the category and explanation from details.
func refusal(details anthropic.BetaRefusalStopDetails) error {
	category := string(details.Category)
	if category == "" {
		category = "none"
	}
	explanation := details.Explanation
	if explanation == "" {
		explanation = "no explanation given"
	}
	return fmt.Errorf("%w: category %s: %s", ErrRefused, category, explanation)
}

// classify maps an error from the SDK to this package's errors and keeps err in the chain.
// Context errors keep their identity so callers can tell a cancel or a deadline apart.
func classify(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("claude request stopped: %w", err)
	}
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%w: %w", ErrRequest, err)
	}
	switch code := apiErr.StatusCode; {
	case code == http.StatusUnauthorized, code == http.StatusForbidden:
		return fmt.Errorf("%w: %s: %w", ErrAuth, authHint, err)
	case code == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %w", ErrRateLimit, err)
	case code >= http.StatusInternalServerError:
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	default:
		return fmt.Errorf("%w: %w", ErrAPI, err)
	}
}
