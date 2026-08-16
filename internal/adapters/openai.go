package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hn-tran/n0ding-bench/internal/bench"
)

type OpenAICompatible struct {
	Endpoint, Model, APIKey string
	Client                  *http.Client
}

func (o *OpenAICompatible) Identity() bench.TargetIdentity {
	return bench.TargetIdentity{Kind: "openai-compatible", Name: o.Model, Model: o.Model, Endpoint: o.Endpoint}
}
func (o *OpenAICompatible) Invoke(ctx context.Context, r bench.TargetRequest) (bench.TargetResponse, error) {
	u, err := url.Parse(o.Endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return bench.TargetResponse{}, errors.New("invalid OpenAI-compatible endpoint")
	}
	if u.Scheme != "https" && !isLoopback(u.Hostname()) {
		return bench.TargetResponse{}, errors.New("non-loopback endpoint requires HTTPS")
	}
	if err := validateEndpointHost(ctx, u.Hostname()); err != nil {
		return bench.TargetResponse{}, err
	}
	body, _ := json.Marshal(map[string]any{"model": o.Model, "messages": []map[string]string{{"role": "user", "content": r.Input}}, "seed": r.Seed, "temperature": 0})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return bench.TargetResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
	client := o.Client
	if client == nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !allowedIP(ip.IP, isLoopback(u.Hostname())) {
					continue
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			}
			return nil, errors.New("endpoint resolves to a prohibited address")
		}}
		client = &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return bench.TargetResponse{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return bench.TargetResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		e := fmt.Errorf("target returned HTTP %d", resp.StatusCode)
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return bench.TargetResponse{}, TemporaryError{e}
		}
		return bench.TargetResponse{}, e
	}
	var v struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &v) != nil || len(v.Choices) == 0 {
		return bench.TargetResponse{}, errors.New("invalid target response")
	}
	return bench.TargetResponse{Output: v.Choices[0].Message.Content}, nil
}
func isLoopback(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }

func validateEndpointHost(ctx context.Context, host string) error {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve endpoint: %w", err)
	}
	allowLoopback := isLoopback(host)
	for _, ip := range ips {
		if !allowedIP(ip.IP, allowLoopback) {
			return errors.New("endpoint resolves to a prohibited address")
		}
	}
	return nil
}

func allowedIP(ip net.IP, allowLoopback bool) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return false
	}
	if ip.IsLoopback() {
		return allowLoopback
	}
	return true
}
