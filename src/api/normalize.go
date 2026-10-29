package main

import (
	"sort"
	"strings"
	"time"
)

// NormalizeSquare maps Square records into the canonical model (spec
// section 5): one record per customer, aggregating that customer's
// completed orders. Only the identifier and location fields are kept
// from the customer record; names, emails, and phone numbers are dropped
// here so no later stage can forward them. Missing fields are recorded
// in MissingFields. Orders with no customer_id cannot be attributed;
// they are counted (unattributed) by the caller and excluded.
func NormalizeSquare(data *SquareSourceData) ([]CanonicalRecord, int) {
	byID := map[string]*CanonicalRecord{}
	orderIdx := map[string][]SquareOrder{}
	for _, o := range data.Orders {
		if o.CustomerID == "" {
			continue
		}
		orderIdx[o.CustomerID] = append(orderIdx[o.CustomerID], o)
	}
	unattributed := 0
	for _, o := range data.Orders {
		if o.CustomerID == "" {
			unattributed++
		}
	}

	for _, cust := range data.Customers {
		if cust.ID == "" {
			continue
		}
		rec := &CanonicalRecord{CustomerID: cust.ID}
		if cust.Address != nil {
			rec.Locality = strings.TrimSpace(cust.Address.Locality)
			rec.AdminRegion = strings.TrimSpace(cust.Address.AdminRegion)
			rec.PostalCode = strings.TrimSpace(cust.Address.PostalCode)
			rec.Country = strings.TrimSpace(cust.Address.Country)
		}
		rec.HasLocation = rec.Locality != "" || rec.PostalCode != ""
		if !rec.HasLocation {
			rec.MissingFields = append(rec.MissingFields, "location")
		}
		items := map[string]bool{}
		for _, o := range orderIdx[cust.ID] {
			rec.OrderCount++
			if o.TotalMoney != nil {
				rec.TotalMinor += o.TotalMoney.Amount
				if rec.Currency == "" {
					rec.Currency = o.TotalMoney.Currency
				}
			}
			if t, err := time.Parse(time.RFC3339, o.ClosedAt); err == nil {
				if rec.LastOrderAt == nil || t.After(*rec.LastOrderAt) {
					tt := t
					rec.LastOrderAt = &tt
				}
				if rec.FirstOrderAt == nil || t.Before(*rec.FirstOrderAt) {
					tt := t
					rec.FirstOrderAt = &tt
				}
			}
			for _, li := range o.LineItems {
				name := strings.TrimSpace(li.Name)
				if name != "" {
					items[name] = true
				}
			}
		}
		for name := range items {
			rec.ItemNames = append(rec.ItemNames, name)
		}
		sort.Strings(rec.ItemNames)
		if rec.OrderCount == 0 {
			rec.MissingFields = append(rec.MissingFields, "transactions")
		}
		byID[cust.ID] = rec
	}

	// Customers with orders but no customer record: keep them under their
	// Square customer ID with location missing, so their revenue is not
	// silently lost. Their location is recorded as missing.
	for id, orders := range orderIdx {
		if _, ok := byID[id]; ok {
			continue
		}
		rec := &CanonicalRecord{CustomerID: id, MissingFields: []string{"location", "customer_record"}}
		items := map[string]bool{}
		for _, o := range orders {
			rec.OrderCount++
			if o.TotalMoney != nil {
				rec.TotalMinor += o.TotalMoney.Amount
				if rec.Currency == "" {
					rec.Currency = o.TotalMoney.Currency
				}
			}
			if t, err := time.Parse(time.RFC3339, o.ClosedAt); err == nil {
				if rec.LastOrderAt == nil || t.After(*rec.LastOrderAt) {
					tt := t
					rec.LastOrderAt = &tt
				}
				if rec.FirstOrderAt == nil || t.Before(*rec.FirstOrderAt) {
					tt := t
					rec.FirstOrderAt = &tt
				}
			}
			for _, li := range o.LineItems {
				if name := strings.TrimSpace(li.Name); name != "" {
					items[name] = true
				}
			}
		}
		for name := range items {
			rec.ItemNames = append(rec.ItemNames, name)
		}
		sort.Strings(rec.ItemNames)
		byID[id] = rec
	}

	out := make([]CanonicalRecord, 0, len(byID))
	for _, rec := range byID {
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CustomerID < out[j].CustomerID })
	return out, unattributed
}
