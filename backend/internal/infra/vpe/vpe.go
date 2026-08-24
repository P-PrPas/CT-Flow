// Package vpe is the client for the inference sidecar (backend/inference/service.py).
//
// Everything that needs a model or the prompt bank goes through here. Ported
// from the FastAPI service's services/vpe_client.py, and deliberately just as thin: the less
// judgement it carries the less there is that can disagree with the sidecar.
//
// A failure the sidecar reported comes back as *Error carrying the status it
// chose (409 model mismatch, 400 empty bank, 403 bad path), so the API can pass
// it through unchanged. The frontend matches on these exact messages, and it
// must not be able to tell that a check moved out of process.
package vpe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Error is a refusal the sidecar chose the status for.
type Error struct {
	Status int
	Detail string
}

func (e *Error) Error() string { return e.Detail }

type Client struct {
	base string
	http *http.Client
}

// New builds a client with no overall timeout, deliberately. http.Client's
// Timeout covers reading the body too, so any value here is really a cap on a
// whole inference pass -- and a 30 minute one silently contradicted the six
// hours jobs.go budgets, cutting a large pool's pass off mid-stream after the
// database had already taken half its writes.
//
// The context is the one deadline, and every call below carries one:
// request-scoped calls die with the request, background passes with
// jobContext(). One place to look, and it cannot disagree with itself.
func New(baseURL string) *Client {
	return &Client{base: baseURL, http: &http.Client{}}
}

// ClassCount is one taught class and how many instances it holds.
type ClassCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// BankSummary is the bank's half of what the frontend calls BankSummary. The
// API service joins it with image status from PostgreSQL; the sidecar has no
// database connection and cannot know that half.
//
// Model is nil until the first box is saved, which is what keeps the model
// picker editable until a project is actually taught something.
type BankSummary struct {
	Classes []ClassCount `json:"classes"`
	Model   *string      `json:"model"`
}

// Detection is one predicted box.
type Detection struct {
	Cls  string     `json:"cls"`
	Box  [4]float64 `json:"box"`
	Conf float64    `json:"conf"`
}

// Box is one box being taught. No Conf: a box a person drew has no confidence.
type Box struct {
	Cls string     `json:"cls"`
	Box [4]float64 `json:"box"`
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("inference service unreachable: %w", err)
	}
	return resp, nil
}

// asError turns a non-2xx response into *Error, preserving the sidecar's status
// and message. The body is always consumed so the connection can be reused.
func asError(resp *http.Response) error {
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var payload struct {
		Detail string `json:"detail"`
	}
	if json.Unmarshal(raw, &payload) == nil && payload.Detail != "" {
		return &Error{Status: resp.StatusCode, Detail: payload.Detail}
	}
	return &Error{Status: resp.StatusCode, Detail: string(raw)}
}

func (c *Client) json(ctx context.Context, method, path string, body, out any) error {
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return asError(resp)
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

// Bank reports what this project's prompt bank has been taught.
func (c *Client) Bank(ctx context.Context, stateDir string) (BankSummary, error) {
	var out BankSummary
	err := c.json(ctx, http.MethodGet, "/vpe/bank?state_dir="+url.QueryEscape(stateDir), nil, &out)
	if out.Classes == nil {
		out.Classes = []ClassCount{}
	}
	return out, err
}

// InstanceTotals is how many embeddings a reembed will reprocess, so the caller
// can size its progress bar before starting the stream.
type InstanceTotals struct {
	Total int     `json:"total"`
	Model *string `json:"model"`
}

func (c *Client) TotalInstances(ctx context.Context, stateDir string) (InstanceTotals, error) {
	var out InstanceTotals
	err := c.json(ctx, http.MethodGet,
		"/vpe/total_instances?state_dir="+url.QueryEscape(stateDir), nil, &out)
	return out, err
}

// Teach extracts embeddings for these boxes and adds them to the bank.
//
// The sidecar groups boxes by class itself -- one embedding per class per save,
// averaged over that class's boxes in the image -- because that is a property of
// what the bank stores, not of the HTTP layer.
//
// modelID may be empty for "whatever the default is"; a mismatch with what the
// bank is already locked to comes back as a 409 *Error, raised before any
// inference runs.
func (c *Client) Teach(ctx context.Context, stateDir, image string, boxes []Box,
	modelID string, labeledBy *string) (BankSummary, error) {
	var out BankSummary
	err := c.json(ctx, http.MethodPost, "/vpe/teach", map[string]any{
		"state_dir": stateDir, "image": image, "boxes": boxes,
		"model_id": modelID, "labeled_by": labeledBy,
	}, &out)
	return out, err
}

// Predict is the model's guesses for one image. An empty bank costs nothing:
// no checkpoint load, no forward pass.
func (c *Client) Predict(ctx context.Context, stateDir, image string,
	conf float64, confByClass map[string]float64) ([]Detection, error) {
	var out struct {
		Boxes []Detection `json:"boxes"`
	}
	err := c.json(ctx, http.MethodPost, "/vpe/predict", map[string]any{
		"state_dir": stateDir, "image": image,
		"conf": conf, "conf_by_class": orEmpty(confByClass),
	}, &out)
	if out.Boxes == nil {
		out.Boxes = []Detection{}
	}
	return out.Boxes, err
}

// StreamLine is one line of a streamed pass: a per-image result, a progress
// tick, or the terminator.
type StreamLine struct {
	Image string      `json:"image"`
	Boxes []Detection `json:"boxes"`
	Sig   []int       `json:"sig"`
	// DoneCount is a reembed progress tick, counted in instances.
	DoneCount int `json:"done_count"`
	// Done marks the last line. Its absence at the end of a stream means the
	// pass was cut short, which is why it exists at all.
	Done bool `json:"done"`
	// Error is set when the sidecar failed after its headers went out; there is
	// no status code left to change at that point.
	Error string `json:"error"`
}

// stream consumes an NDJSON response, calling onLine for each line until the
// terminator.
//
// The pass must be consumed to the end rather than abandoned part way: the
// sidecar holds the checkpoint's lock for the whole stream, so walking away
// leaves it held until the request context is cancelled.
func (c *Client) stream(ctx context.Context, path string, body any, onLine func(StreamLine) error) error {
	resp, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return asError(resp)
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // a busy image can carry hundreds of boxes
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var line StreamLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			return fmt.Errorf("malformed line from the inference service: %w", err)
		}
		if line.Error != "" {
			return &Error{Status: http.StatusInternalServerError, Detail: line.Error}
		}
		if line.Done {
			return nil
		}
		if err := onLine(line); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	// Reaching here means the body ended without the terminator: the pass was
	// truncated, and reporting it as success would record a partial run as a
	// complete one.
	return fmt.Errorf("the inference service closed the stream before finishing")
}

// PredictStream runs one inference pass over many images, one line per image.
// wantSig asks for the 8x8 thumbnail the pool rescore uses (FR-18).
func (c *Client) PredictStream(ctx context.Context, stateDir string, imagePaths []string,
	conf float64, confByClass map[string]float64, wantSig bool, onLine func(StreamLine) error) error {
	return c.stream(ctx, "/vpe/predict_stream", map[string]any{
		"state_dir": stateDir, "images": imagePaths, "conf": conf,
		"conf_by_class": orEmpty(confByClass), "want_sig": wantSig,
	}, onLine)
}

// ReembedStream re-extracts every stored instance under a different checkpoint
// and swaps the bank's lock, committing once at the end.
func (c *Client) ReembedStream(ctx context.Context, stateDir, modelID string,
	onLine func(StreamLine) error) error {
	return c.stream(ctx, "/vpe/reembed_stream",
		map[string]any{"state_dir": stateDir, "model_id": modelID}, onLine)
}

// orEmpty keeps a nil map out of the request body: the sidecar treats a missing
// conf_by_class as {}, but sending null where a dict is expected is a needless
// thing to depend on.
func orEmpty(m map[string]float64) map[string]float64 {
	if m == nil {
		return map[string]float64{}
	}
	return m
}
