package model

import "time"

type Webhook struct {
    ID         int       `json:"id"`
    Source     string    `json:"source"`
    Headers    string    `json:"headers"`
    Payload    []byte    `json:"payload"`
    ReceivedAt time.Time `json:"received_at"`
    Status     string    `json:"status"`
}