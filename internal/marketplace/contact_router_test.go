package marketplace

import (
	"context"
	"testing"
)

func TestDecideRoutesMarketplaceWork(t *testing.T) {
	tests := []struct {
		name      string
		kind      ContactKind
		wantQueue string
		wantLabel string
	}{
		{name: "seller uploads", kind: SellerAsset, wantQueue: "seller-assets", wantLabel: "Seller asset"},
		{name: "buyer asks for progress", kind: BuyerUpdate, wantQueue: "buyer-updates", wantLabel: "Buyer update"},
		{name: "order changes owner", kind: OrderHandoff, wantQueue: "order-handoff", wantLabel: "Order handoff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue, label, err := Decide(tt.kind)
			if err != nil {
				t.Fatal(err)
			}
			if queue != tt.wantQueue || label != tt.wantLabel {
				t.Fatalf("Decide(%q) = %q, %q; want %q, %q", tt.kind, queue, label, tt.wantQueue, tt.wantLabel)
			}
		})
	}
}

func TestRouteVerifiesBeforeSending(t *testing.T) {
	delivery := &recordingDelivery{}
	router := Router{Inbox: "team@example.com", Infrai: delivery}
	receipt, err := router.Route(context.Background(), Contact{
		SubmissionID: "form-1042", Kind: OrderHandoff, Email: "buyer@example.com",
		OrderID: "ORD-88", Message: "Seller approved the transfer.", CaptchaToken: "browser-token",
	}, "192.0.2.4")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := delivery.calls, []string{"captcha", "email"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("calls = %v; want %v", got, want)
	}
	if delivery.idempotencyKey != "form-1042" {
		t.Fatalf("idempotency key = %q; want form-1042", delivery.idempotencyKey)
	}
	if receipt.Queue != "order-handoff" || receipt.MessageID != "msg-42" {
		t.Fatalf("receipt = %#v", receipt)
	}
}

type recordingDelivery struct {
	calls          []string
	idempotencyKey string
}

func (d *recordingDelivery) VerifyCaptcha(context.Context, string, string) error {
	d.calls = append(d.calls, "captcha")
	return nil
}

func (d *recordingDelivery) SendEmail(_ context.Context, key, _, _, _ string) (string, error) {
	d.calls = append(d.calls, "email")
	d.idempotencyKey = key
	return "msg-42", nil
}
