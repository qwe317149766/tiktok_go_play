package main

import "time"

type Order struct {
	ID         int64
	OrderID    string
	APIKey     string
	AwemeID    string
	Link       string
	Quantity   int64
	Delivered  int64
	StartCount int64
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (o Order) Remains() int64 {
	r := o.Quantity - o.Delivered
	if r < 0 {
		return 0
	}
	return r
}

type APIKeyRow struct {
	Key       string
	MerchantName string
	IsActive  bool
	Credit    int64
	TotalCredit int64
	CreatedAt time.Time
	UpdatedAt time.Time
}


