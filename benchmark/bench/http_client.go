package bench

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/isucon/isucandar/agent"
)

// ShouldLogHTTPError returns true if the HTTP error should be logged.
// Returns false if the context is cancelled (benchmark timeout) to avoid noisy logs.
func ShouldLogHTTPError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	// Don't log if context was cancelled (benchmark timeout)
	if ctx.Err() != nil {
		return false
	}
	// Don't log deadline exceeded errors after timeout
	if strings.Contains(err.Error(), "context deadline exceeded") && ctx.Err() != nil {
		return false
	}
	return true
}

type HttpResponse struct {
	StatusCode int
	Body       []byte
}

func HttpGet(ctx context.Context, agent *agent.Agent, path string) (HttpResponse, error) {
	req, err := agent.GET(path)
	if err != nil {
		return HttpResponse{}, fmt.Errorf("failed to create GET request: %w", err)
	}

	resp, err := agent.Do(ctx, req)
	if err != nil {
		return HttpResponse{}, fmt.Errorf("failed to execute GET request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return HttpResponse{}, err
	}

	httpResp := HttpResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
	}

	return httpResp, nil
}

func HttpPost(ctx context.Context, agent *agent.Agent, path string, body io.Reader) (HttpResponse, error) {
	req, err := agent.POST(path, body)
	if err != nil {
		return HttpResponse{}, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := agent.Do(ctx, req)
	if err != nil {
		return HttpResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return HttpResponse{}, err
	}

	httpResp := HttpResponse{
		StatusCode: resp.StatusCode,
		Body:       respBody,
	}

	return httpResp, nil
}
