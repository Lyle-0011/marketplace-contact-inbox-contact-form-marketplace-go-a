package marketplace

import (
	"context"
	"fmt"
	"strings"
)

type ContactKind string

const (
	SellerAsset  ContactKind = "seller_asset"
	BuyerUpdate  ContactKind = "buyer_update"
	OrderHandoff ContactKind = "order_handoff"
)

type Contact struct {
	SubmissionID string      `json:"submission_id"`
	Kind         ContactKind `json:"kind"`
	Email        string      `json:"email"`
	OrderID      string      `json:"order_id"`
	Message      string      `json:"message"`
	CaptchaToken string      `json:"captcha_token"`
}

type Receipt struct {
	Queue     string `json:"queue"`
	MessageID string `json:"message_id"`
}

type Delivery interface {
	VerifyCaptcha(context.Context, string, string) error
	SendEmail(context.Context, string, string, string, string) (string, error)
}

type Router struct {
	Inbox  string
	Infrai Delivery
}

type route struct {
	queue string
	label string
}

func Decide(kind ContactKind) (string, string, error) {
	routes := map[ContactKind]route{
		SellerAsset:  {queue: "seller-assets", label: "Seller asset"},
		BuyerUpdate:  {queue: "buyer-updates", label: "Buyer update"},
		OrderHandoff: {queue: "order-handoff", label: "Order handoff"},
	}
	selected, ok := routes[kind]
	if !ok {
		return "", "", fmt.Errorf("unknown contact kind %q", kind)
	}
	return selected.queue, selected.label, nil
}

func (r Router) Route(ctx context.Context, contact Contact, ip string) (Receipt, error) {
	queue, label, err := Decide(contact.Kind)
	if err != nil {
		return Receipt{}, err
	}
	if strings.TrimSpace(contact.SubmissionID) == "" || strings.TrimSpace(contact.Email) == "" || strings.TrimSpace(contact.Message) == "" || strings.TrimSpace(contact.CaptchaToken) == "" {
		return Receipt{}, fmt.Errorf("submission_id, email, message, and captcha_token are required")
	}
	if err := r.Infrai.VerifyCaptcha(ctx, contact.CaptchaToken, ip); err != nil {
		return Receipt{}, err
	}
	subject := fmt.Sprintf("[%s] %s", queue, label)
	body := fmt.Sprintf("Reply to: %s\nOrder: %s\nQueue: %s\n\n%s", contact.Email, contact.OrderID, queue, contact.Message)
	messageID, err := r.Infrai.SendEmail(ctx, contact.SubmissionID, r.Inbox, subject, body)
	if err != nil {
		return Receipt{}, err
	}
	return Receipt{Queue: queue, MessageID: messageID}, nil
}
