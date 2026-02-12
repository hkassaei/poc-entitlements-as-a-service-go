package eapaka

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HSSVectors holds authentication vectors returned by the HSS.
type HSSVectors struct {
	RAND []byte
	AUTN []byte
	XRES []byte
	CK   []byte
	IK   []byte
}

// ResyncResult holds the result of an SQN resync request.
type ResyncResult struct {
	Success bool
	Vectors *HSSVectors
	Error   string
}

// ErrHSSSubscriberNotFound is returned when the HSS doesn't know the IMSI.
var ErrHSSSubscriberNotFound = errors.New("HSS: subscriber not found")

// HSSClient is an HTTP client for the mock-hss.
type HSSClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHSSClient creates a new HSSClient.
func NewHSSClient(baseURL string) *HSSClient {
	return &HSSClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type vectorsRequest struct {
	IMSI string `json:"imsi"`
}

type vectorsResponse struct {
	RAND string `json:"rand"`
	AUTN string `json:"autn"`
	XRES string `json:"xres"`
	CK   string `json:"ck"`
	IK   string `json:"ik"`
}

// FetchVectors fetches authentication vectors from the HSS for the given IMSI.
func (c *HSSClient) FetchVectors(ctx context.Context, imsi string) (*HSSVectors, error) {
	body, err := json.Marshal(vectorsRequest{IMSI: imsi})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/vectors", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch vectors: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrHSSSubscriberNotFound
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HSS returned %d: %s", resp.StatusCode, string(respBody))
	}

	var data vectorsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return decodeVectors(&data)
}

type resyncRequest struct {
	IMSI string `json:"imsi"`
	RAND string `json:"rand"`
	AUTS string `json:"auts"`
}

// ResyncVectors requests SQN resynchronization after a SYNC_FAILURE.
func (c *HSSClient) ResyncVectors(ctx context.Context, imsi string, randVal, auts []byte) *ResyncResult {
	body, err := json.Marshal(resyncRequest{
		IMSI: imsi,
		RAND: base64.StdEncoding.EncodeToString(randVal),
		AUTS: base64.StdEncoding.EncodeToString(auts),
	})
	if err != nil {
		return &ResyncResult{Success: false, Error: "marshal request: " + err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/resync", bytes.NewReader(body))
	if err != nil {
		return &ResyncResult{Success: false, Error: "create request: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &ResyncResult{Success: false, Error: "HSS communication error"}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &ResyncResult{Success: false, Error: "Subscriber not found"}
	}

	if resp.StatusCode == http.StatusBadRequest {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return &ResyncResult{Success: false, Error: errResp.Error}
		}
		return &ResyncResult{Success: false, Error: "AUTS validation failed"}
	}

	if resp.StatusCode != http.StatusOK {
		return &ResyncResult{Success: false, Error: fmt.Sprintf("HSS returned %d", resp.StatusCode)}
	}

	var data vectorsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return &ResyncResult{Success: false, Error: "decode response: " + err.Error()}
	}

	vectors, err := decodeVectors(&data)
	if err != nil {
		return &ResyncResult{Success: false, Error: err.Error()}
	}

	return &ResyncResult{Success: true, Vectors: vectors}
}

func decodeVectors(data *vectorsResponse) (*HSSVectors, error) {
	randBytes, err := base64.StdEncoding.DecodeString(data.RAND)
	if err != nil {
		return nil, fmt.Errorf("decode RAND: %w", err)
	}
	autnBytes, err := base64.StdEncoding.DecodeString(data.AUTN)
	if err != nil {
		return nil, fmt.Errorf("decode AUTN: %w", err)
	}
	xresBytes, err := base64.StdEncoding.DecodeString(data.XRES)
	if err != nil {
		return nil, fmt.Errorf("decode XRES: %w", err)
	}
	ckBytes, err := base64.StdEncoding.DecodeString(data.CK)
	if err != nil {
		return nil, fmt.Errorf("decode CK: %w", err)
	}
	ikBytes, err := base64.StdEncoding.DecodeString(data.IK)
	if err != nil {
		return nil, fmt.Errorf("decode IK: %w", err)
	}

	return &HSSVectors{
		RAND: randBytes, AUTN: autnBytes, XRES: xresBytes,
		CK: ckBytes, IK: ikBytes,
	}, nil
}
