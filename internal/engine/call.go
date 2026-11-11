package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/dcadolph/ferry/internal/schema"
)

// attempts is how many replies a step asks for at most: the first and one corrective retry.
const attempts = 2

// send renders the prompt for step with data, sends it, and decodes the reply into out, which
// must be a pointer to the step's reply type. It returns a problem, in words the model can act
// on, when the reply does not decode, and an error when the call itself fails.
func (r *run) send(ctx context.Context, step Step, data promptData, out any) (string, error) {
	prompt, err := render(templateFor(step), data)
	if err != nil {
		return "", err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf("%s step: %w", step, ctxErr)
	}
	req := Request{Step: step, System: r.system, Prompt: prompt, Schema: schema.For(out)}
	reply, err := r.e.completer.Complete(ctx, req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("%s step: %w", step, ctxErr)
		}
		return "", fmt.Errorf("%w: %s step: %w", ErrModelCall, step, err)
	}
	if jsonErr := json.Unmarshal([]byte(reply), out); jsonErr != nil {
		return fmt.Sprintf("the reply is not valid JSON for the schema: %v", jsonErr), nil
	}
	return "", nil
}

// ask sends the request for step and decodes the reply into a T, which clean tidies and checks.
// A reply that does not decode, or that clean finds a problem with, gets one corrective retry
// that names the problem. A second bad reply is ErrBadReply.
func ask[T any](ctx context.Context, r *run, step Step, data promptData, clean func(*T) string) (T, error) {
	var zero T
	for range attempts {
		var v T
		problem, err := r.send(ctx, step, data, &v)
		if err != nil {
			return zero, err
		}
		if problem == "" {
			problem = clean(&v)
		}
		if problem == "" {
			return v, nil
		}
		data.Problem = problem
	}
	return zero, fmt.Errorf("%w: %s step: %s", ErrBadReply, step, data.Problem)
}

// each calls fn for every index below n, running at most workers calls at once. It stops
// starting new calls once ctx is done and waits for the calls already running.
func each(ctx context.Context, n, workers int, fn func(i int)) {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i := range n {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i)
		}(i)
	}
	wg.Wait()
}
