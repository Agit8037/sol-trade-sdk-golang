package swqos

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gagliardetto/solana-go"
)

// MevProtectionLevel represents MEV protection levels
type MevProtectionLevel int

const (
	MevProtectionNone MevProtectionLevel = iota
	MevProtectionBasic
	MevProtectionEnhanced
	MevProtectionMaximum
)

// TransactionResult represents transaction submission result
type TransactionResult struct {
	Signature            solana.Signature
	Success              bool
	Provider             string
	LatencyMs            int64
	Slot                 uint64
	Error                string
	BundleID             string
	ConfirmationStatus   string
}

// SwqosConfigExtended extended configuration for SWQOS
type SwqosConfigExtended struct {
	Type                   SwqosType
	APIKey                 string
	Region                 SwqosRegion
	URL                    string
	TimeoutMs              int
	MaxRetries             int
	Enabled                bool
	PriorityFeeMultiplier  float64
	MevProtection          MevProtectionLevel
	CustomHeaders          map[string]string
	RateLimitRPS           int
}

// DefaultSwqosConfigExtended returns default extended config
func DefaultSwqosConfigExtended(swqosType SwqosType) *SwqosConfigExtended {
	return &SwqosConfigExtended{
		Type:                  swqosType,
		Region:                SwqosRegionDefault,
		TimeoutMs:             5000,
		MaxRetries:            3,
		Enabled:               true,
		PriorityFeeMultiplier: 1.0,
		MevProtection:         MevProtectionEnhanced,
		CustomHeaders:         make(map[string]string),
		RateLimitRPS:          100,
	}
}

// SwqosProviderBase base implementation for SWQOS providers
type SwqosProviderBase struct {
	config      *SwqosConfigExtended
	stats       ProviderStats
	lastRequest int64
	mu          sync.RWMutex
}

// ProviderStats represents provider statistics
type ProviderStats struct {
	Requests      int64
	Successes     int64
	Failures      int64
	AvgLatencyMs  int64
	LastError     string
}

// UpdateStats updates provider statistics
func (p *SwqosProviderBase) UpdateStats(success bool, latencyMs int64, err string) {
	atomic.AddInt64(&p.stats.Requests, 1)
	if success {
		atomic.AddInt64(&p.stats.Successes, 1)
	} else {
		atomic.AddInt64(&p.stats.Failures, 1)
		p.mu.Lock()
		p.stats.LastError = err
		p.mu.Unlock()
	}

	// Update average latency
	n := atomic.LoadInt64(&p.stats.Requests)
	oldAvg := atomic.LoadInt64(&p.stats.AvgLatencyMs)
	newAvg := (oldAvg*(n-1) + latencyMs) / n
	atomic.StoreInt64(&p.stats.AvgLatencyMs, newAvg)
}

// GetStats returns provider statistics
func (p *SwqosProviderBase) GetStats() ProviderStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return ProviderStats{
		Requests:     atomic.LoadInt64(&p.stats.Requests),
		Successes:    atomic.LoadInt64(&p.stats.Successes),
		Failures:     atomic.LoadInt64(&p.stats.Failures),
		AvgLatencyMs: atomic.LoadInt64(&p.stats.AvgLatencyMs),
		LastError:    p.stats.LastError,
	}
}

// RateLimitCheck checks and enforces rate limiting
func (p *SwqosProviderBase) RateLimitCheck() {
	if p.config.RateLimitRPS <= 0 {
		return
	}

	delay := time.Second / time.Duration(p.config.RateLimitRPS)
	last := atomic.LoadInt64(&p.lastRequest)
	now := time.Now().UnixNano()
	elapsed := time.Duration(now - last)

	if elapsed < delay {
		time.Sleep(delay - elapsed)
	}

	atomic.StoreInt64(&p.lastRequest, time.Now().UnixNano())
}

// ===== Additional Provider Implementations =====

// NextBlockClient NextBlock SWQOS client
type NextBlockClient struct {
	SwqosProviderBase
	apiURL string
}

// NewNextBlockClient creates new NextBlock client
func NewNextBlockClient(config *SwqosConfigExtended) *NextBlockClient {
	url := config.URL
	if url == "" {
		url = "https://api.nextblock.io"
	}
	return &NextBlockClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via NextBlock
func (c *NextBlockClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded, "tip": tip}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "NextBlock", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "NextBlock", LatencyMs: latency}, nil
}

// Node1Client Node1 SWQOS client
type Node1Client struct {
	SwqosProviderBase
	apiURL string
}

// NewNode1Client creates new Node1 client
func NewNode1Client(config *SwqosConfigExtended) *Node1Client {
	url := config.URL
	if url == "" {
		url = "https://api.node1.io"
	}
	return &Node1Client{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via Node1
func (c *Node1Client) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("X-API-Key", c.config.APIKey)
	}

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "Node1", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "Node1", LatencyMs: latency}, nil
}

// BlockRazorClient BlockRazor SWQOS client
type BlockRazorClient struct {
	SwqosProviderBase
	apiURL string
}

// NewBlockRazorClient creates new BlockRazor client
func NewBlockRazorClient(config *SwqosConfigExtended) *BlockRazorClient {
	url := config.URL
	if url == "" {
		url = "https://api.blockrazor.io"
	}
	return &BlockRazorClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via BlockRazor
func (c *BlockRazorClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "BlockRazor", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "BlockRazor", LatencyMs: latency}, nil
}

// AstralaneClient Astralane SWQOS client
type AstralaneClient struct {
	SwqosProviderBase
	apiURL string
}

// NewAstralaneClient creates new Astralane client
func NewAstralaneClient(config *SwqosConfigExtended) *AstralaneClient {
	url := config.URL
	if url == "" {
		url = "https://api.astralane.io"
	}
	return &AstralaneClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via Astralane
func (c *AstralaneClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "Astralane", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "Astralane", LatencyMs: latency}, nil
}

// StelliumClient Stellium SWQOS client
type StelliumClient struct {
	SwqosProviderBase
	apiURL string
}

// NewStelliumClient creates new Stellium client
func NewStelliumClient(config *SwqosConfigExtended) *StelliumClient {
	url := config.URL
	if url == "" {
		url = "https://api.stellium.io"
	}
	return &StelliumClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via Stellium
func (c *StelliumClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "Stellium", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "Stellium", LatencyMs: latency}, nil
}

// LightspeedClient Lightspeed SWQOS client
type LightspeedClient struct {
	SwqosProviderBase
	apiURL string
}

// NewLightspeedClient creates new Lightspeed client
func NewLightspeedClient(config *SwqosConfigExtended) *LightspeedClient {
	url := config.URL
	if url == "" {
		url = "https://api.lightspeed.trade"
	}
	return &LightspeedClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via Lightspeed
func (c *LightspeedClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "Lightspeed", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "Lightspeed", LatencyMs: latency}, nil
}

// SoyasClient Soyas SWQOS client
type SoyasClient struct {
	SwqosProviderBase
	apiURL string
}

// NewSoyasClient creates new Soyas client
func NewSoyasClient(config *SwqosConfigExtended) *SoyasClient {
	url := config.URL
	if url == "" {
		url = "https://api.soyas.io"
	}
	return &SoyasClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via Soyas
func (c *SoyasClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "Soyas", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "Soyas", LatencyMs: latency}, nil
}

// SpeedlandingClient Speedlanding SWQOS client
type SpeedlandingClient struct {
	SwqosProviderBase
	apiURL string
}

// NewSpeedlandingClient creates new Speedlanding client
func NewSpeedlandingClient(config *SwqosConfigExtended) *SpeedlandingClient {
	url := config.URL
	if url == "" {
		url = "https://api.speedlanding.io"
	}
	return &SpeedlandingClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via Speedlanding
func (c *SpeedlandingClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "Speedlanding", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "Speedlanding", LatencyMs: latency}, nil
}

// TritonClient Triton SWQOS client
type TritonClient struct {
	SwqosProviderBase
	apiURL string
}

// NewTritonClient creates new Triton client
func NewTritonClient(config *SwqosConfigExtended) *TritonClient {
	url := config.URL
	if url == "" {
		url = "https://api.triton.one"
	}
	return &TritonClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via Triton
func (c *TritonClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "Triton", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "Triton", LatencyMs: latency}, nil
}

// QuickNodeClient QuickNode SWQOS client
type QuickNodeClient struct {
	SwqosProviderBase
	apiURL string
}

// NewQuickNodeClient creates new QuickNode client
func NewQuickNodeClient(config *SwqosConfigExtended) *QuickNodeClient {
	url := config.URL
	if url == "" {
		url = "https://api.quicknode.com"
	}
	return &QuickNodeClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via QuickNode
func (c *QuickNodeClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "QuickNode", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "QuickNode", LatencyMs: latency}, nil
}

// SyndicaClient Syndica SWQOS client
type SyndicaClient struct {
	SwqosProviderBase
	apiURL string
}

// NewSyndicaClient creates new Syndica client
func NewSyndicaClient(config *SwqosConfigExtended) *SyndicaClient {
	url := config.URL
	if url == "" {
		url = "https://api.syndica.io"
	}
	return &SyndicaClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via Syndica
func (c *SyndicaClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "Syndica", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "Syndica", LatencyMs: latency}, nil
}

// FigmentClient Figment SWQOS client
type FigmentClient struct {
	SwqosProviderBase
	apiURL string
}

// NewFigmentClient creates new Figment client
func NewFigmentClient(config *SwqosConfigExtended) *FigmentClient {
	url := config.URL
	if url == "" {
		url = "https://api.figment.io"
	}
	return &FigmentClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via Figment
func (c *FigmentClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "Figment", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "Figment", LatencyMs: latency}, nil
}

// AlchemyClient Alchemy SWQOS client
type AlchemyClient struct {
	SwqosProviderBase
	apiURL string
}

// NewAlchemyClient creates new Alchemy client
func NewAlchemyClient(config *SwqosConfigExtended) *AlchemyClient {
	url := config.URL
	if url == "" {
		url = "https://api.alchemy.com"
	}
	return &AlchemyClient{
		SwqosProviderBase: SwqosProviderBase{config: config},
		apiURL:            url,
	}
}

// SubmitTransaction submits transaction via Alchemy
func (c *AlchemyClient) SubmitTransaction(ctx context.Context, tx []byte, tip uint64) (*TransactionResult, error) {
	c.RateLimitCheck()
	start := time.Now()

	encoded := base64.StdEncoding.EncodeToString(tx)
	payload := map[string]interface{}{"transaction": encoded}
	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/api/v1/submit", strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.UpdateStats(false, latency, err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Signature string `json:"signature"`
		Error     string `json:"error"`
	}
	json.Unmarshal(body, &result)

	if result.Error != "" {
		c.UpdateStats(false, latency, result.Error)
		return &TransactionResult{Success: false, Provider: "Alchemy", LatencyMs: latency, Error: result.Error}, nil
	}

	sig, _ := solana.SignatureFromBase58(result.Signature)
	c.UpdateStats(true, latency, "")
	return &TransactionResult{Signature: sig, Success: true, Provider: "Alchemy", LatencyMs: latency}, nil
}

// SwqosProviderFactory creates SWQOS providers
type SwqosProviderFactory struct{}

// CreateProvider creates a provider based on type
func (f *SwqosProviderFactory) CreateProvider(config *SwqosConfigExtended) (interface{}, error) {
	switch config.Type {
	case SwqosTypeJito:
		return NewJitoClient("", config.URL, config.APIKey), nil
	case SwqosTypeBloxroute:
		return NewBloxrouteClient("", config.URL, config.APIKey), nil
	case SwqosTypeZeroSlot:
		return NewZeroSlotClient("", config.URL, config.APIKey), nil
	case SwqosTypeNextBlock:
		return NewNextBlockClient(config), nil
	case SwqosTypeTemporal:
		return NewTemporalClient("", config.URL, config.APIKey), nil
	case SwqosTypeNode1:
		return NewNode1Client(config), nil
	case SwqosTypeFlashBlock:
		return NewFlashBlockClient("", config.URL, config.APIKey), nil
	case SwqosTypeBlockRazor:
		return NewBlockRazorClient(config), nil
	case SwqosTypeAstralane:
		return NewAstralaneClient(config), nil
	case SwqosTypeStellium:
		return NewStelliumClient(config), nil
	case SwqosTypeLightspeed:
		return NewLightspeedClient(config), nil
	case SwqosTypeSoyas:
		return NewSoyasClient(config), nil
	case SwqosTypeSpeedlanding:
		return NewSpeedlandingClient(config), nil
	case SwqosTypeHelius:
		return NewHeliusClient("", config.URL, &config.APIKey, false), nil
	case SwqosTypeDefault:
		return NewDefaultClient(""), nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %v", config.Type)
	}
}

// SwqosManager manages multiple SWQOS providers
type SwqosManager struct {
	providers map[SwqosType]interface{}
	mu        sync.RWMutex
}

// NewSwqosManager creates new SWQOS manager
func NewSwqosManager() *SwqosManager {
	return &SwqosManager{
		providers: make(map[SwqosType]interface{}),
	}
}

// AddProvider adds a provider
func (m *SwqosManager) AddProvider(swqosType SwqosType, provider interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[swqosType] = provider
}

// GetProvider gets a provider
func (m *SwqosManager) GetProvider(swqosType SwqosType) interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.providers[swqosType]
}

// GetAllProviders gets all providers
func (m *SwqosManager) GetAllProviders() map[SwqosType]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[SwqosType]interface{})
	for k, v := range m.providers {
		result[k] = v
	}
	return result
}
