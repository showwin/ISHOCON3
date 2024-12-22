package bench

import (
	"fmt"
	"io"
	"context"

	"github.com/isucon/isucandar/agent"
)

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
