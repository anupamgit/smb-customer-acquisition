package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SquareSourceData is what the SMB Connector Client returns for
// normalization: raw Square records, kept server-side only. Customer
// names, emails, and phone numbers are present here (they arrive from
// Square) and must never be forwarded to Qloo or the LLM service; the
// normalizer keeps only the identifier and location fields.
type SquareCustomer struct {
	ID         string         `json:"id"`
	GivenName  string         `json:"given_name"`
	FamilyName string         `json:"family_name"`
	Email      string         `json:"email_address"`
	Phone      string         `json:"phone_number"`
	CreatedAt  string         `json:"created_at"`
	Address    *SquareAddress `json:"address"`
}

type SquareAddress struct {
	AddressLine1 string `json:"address_line_1"`
	AddressLine2 string `json:"address_line_2"`
	Locality     string `json:"locality"`
	AdminRegion  string `json:"administrative_district_level_1"`
	PostalCode   string `json:"postal_code"`
	Country      string `json:"country"`
}

type SquareMoney struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type SquareLineItem struct {
	Name       string       `json:"name"`
	Quantity   string       `json:"quantity"`
	TotalMoney *SquareMoney `json:"total_money"`
}

type SquareOrder struct {
	ID         string           `json:"id"`
	CustomerID string           `json:"customer_id"`
	CreatedAt  string           `json:"created_at"`
	ClosedAt   string           `json:"closed_at"`
	State      string           `json:"state"`
	TotalMoney *SquareMoney     `json:"total_money"`
	LineItems  []SquareLineItem `json:"line_items"`
}

type SquareLocation struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Address     *SquareAddress `json:"address"`
	Coordinates *struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"coordinates"`
}

type SquareSourceData struct {
	Customers []SquareCustomer
	Orders    []SquareOrder
	Locations []SquareLocation
}

// Connector is the interface the orchestrator uses to read business data.
// The production implementation is squareClient; tests use a fake.
type Connector interface {
	FetchSourceData(ctx context.Context) (*SquareSourceData, error)
}

// squareClient is the single SMB Connector Client (Square alone, Step 4).
// It performs read-only GET/POST calls within the documented read scopes
// (CUSTOMERS_READ, ORDERS_READ; Locations scope unverified — see the
// Step 7 summary). It sends no data back to Square. Endpoint paths are
// the Square REST API v2 paths documented on developer.squareup.com
// (GET /v2/customers, POST /v2/orders/search, GET /v2/locations);
// they were not verified against a live Square account in this workflow
// (no key was available) and must be confirmed on the first live run.
type squareClient struct {
	baseURL    string
	apiKey     string
	version    string
	httpClient *http.Client
}

func newSquareClient(cfg config) *squareClient {
	return &squareClient{
		baseURL:    strings.TrimRight(cfg.squareBaseURL, "/"),
		apiKey:     cfg.squareKey,
		version:    cfg.squareVersion,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// squareErrorEnvelope is Square's error shape; only code/detail are kept.
type squareErrorEnvelope struct {
	Errors []struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	} `json:"errors"`
}

func (c *squareClient) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	if c.apiKey == "" {
		return nil, errNotConfigured
	}
	var respBody []byte
	err := doWithRetry(retryDelays, func(attempt int) error {
		var reqBody io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return err
			}
			reqBody = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Square-Version", c.version)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return &transientError{err: err}
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return &transientError{err: err}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			msg := string(b)
			var env squareErrorEnvelope
			if json.Unmarshal(b, &env) == nil && len(env.Errors) > 0 {
				msg = env.Errors[0].Code + ": " + env.Errors[0].Detail
			}
			if len(msg) > 300 {
				msg = msg[:300]
			}
			return &statusError{status: resp.StatusCode, body: msg}
		}
		respBody = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return respBody, nil
}

func (c *squareClient) listLocations(ctx context.Context) ([]SquareLocation, error) {
	b, err := c.do(ctx, http.MethodGet, "/v2/locations", nil)
	if err != nil {
		return nil, fmt.Errorf("square locations read failed: %w", err)
	}
	var out struct {
		Locations []SquareLocation `json:"locations"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("square locations response could not be parsed: %w", err)
	}
	return out.Locations, nil
}

func (c *squareClient) listCustomers(ctx context.Context) ([]SquareCustomer, error) {
	var all []SquareCustomer
	cursor := ""
	for {
		path := "/v2/customers?limit=100"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		b, err := c.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("square customers read failed: %w", err)
		}
		var page struct {
			Customers []SquareCustomer `json:"customers"`
			Cursor    string           `json:"cursor"`
		}
		if err := json.Unmarshal(b, &page); err != nil {
			return nil, fmt.Errorf("square customers response could not be parsed: %w", err)
		}
		all = append(all, page.Customers...)
		if page.Cursor == "" {
			return all, nil
		}
		cursor = page.Cursor
	}
}

func (c *squareClient) searchOrders(ctx context.Context, locationIDs []string) ([]SquareOrder, error) {
	var all []SquareOrder
	cursor := ""
	for {
		reqBody := map[string]any{
			"location_ids": locationIDs,
			"query": map[string]any{
				"filter": map[string]any{
					"state_filter": map[string]any{"states": []string{"COMPLETED"}},
				},
				"sort": map[string]any{
					"sort_field": "CLOSED_AT",
					"sort_order": "DESC",
				},
			},
			"limit": 100,
		}
		if cursor != "" {
			reqBody["cursor"] = cursor
		}
		b, err := c.do(ctx, http.MethodPost, "/v2/orders/search", reqBody)
		if err != nil {
			return nil, fmt.Errorf("square orders search failed: %w", err)
		}
		var page struct {
			Orders []SquareOrder `json:"orders"`
			Cursor string        `json:"cursor"`
		}
		if err := json.Unmarshal(b, &page); err != nil {
			return nil, fmt.Errorf("square orders response could not be parsed: %w", err)
		}
		all = append(all, page.Orders...)
		if page.Cursor == "" {
			return all, nil
		}
		cursor = page.Cursor
	}
}

// FetchSourceData reads locations, customers, and completed orders.
// Locations are read first because SearchOrders requires location IDs.
// Any failure fails the whole read visibly; partial data is never used.
func (c *squareClient) FetchSourceData(ctx context.Context) (*SquareSourceData, error) {
	locations, err := c.listLocations(ctx)
	if err != nil {
		return nil, err
	}
	var locationIDs []string
	for _, l := range locations {
		if l.ID != "" {
			locationIDs = append(locationIDs, l.ID)
		}
	}
	customers, err := c.listCustomers(ctx)
	if err != nil {
		return nil, err
	}
	var orders []SquareOrder
	if len(locationIDs) > 0 {
		orders, err = c.searchOrders(ctx, locationIDs)
		if err != nil {
			return nil, err
		}
	}
	return &SquareSourceData{Customers: customers, Orders: orders, Locations: locations}, nil
}
