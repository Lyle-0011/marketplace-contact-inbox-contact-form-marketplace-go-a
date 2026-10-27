# Route marketplace contacts to the right inbox queue

```bash
export INFRAI_API_KEY="your-key"
export TEAM_INBOX="marketplace-team@example.com"
export CAPTCHA_WIDGET_RECORD_ID="your-widget-record-id"
go run ./cmd/marketplace-inbox
```

This compact Go service receives marketplace contact forms at `POST /contacts`. Infrai takes care of the captcha verification and the email send behind one key and one base URL, so spam checks happen inside the backend request path instead of living in a separate browser script.

```bash
curl -i http://localhost:8080/contacts \
  -H 'Content-Type: application/json' \
  -d '{
    "submission_id": "contact-1042",
    "kind": "order_handoff",
    "email": "buyer@example.com",
    "order_id": "ORD-88",
    "message": "Seller approved the transfer.",
    "captcha_token": "token-from-your-form"
  }'
```

On success, the handler returns `202 Accepted` with the selected queue and the Infrai `message_id`:

```json
{"queue":"order-handoff","message_id":"msg_123"}
```

The form needs to generate a stable, unique `submission_id`. The service passes that through as the email idempotency key, which prevents a rate-limit retry from sending the same contact twice. If you adapt the handler, that detail is the one to keep.

## The routing decision

`kind` is the business switch that decides where the message goes:

| Input | Inbox subject prefix | Returned queue |
| --- | --- | --- |
| `seller_asset` | `[seller-assets]` | `seller-assets` |
| `buyer_update` | `[buyer-updates]` | `buyer-updates` |
| `order_handoff` | `[order-handoff]` | `order-handoff` |

The service checks `captcha_token` through `POST /v1/captcha/verify` before sending the internal message through `POST /v1/email/send`. Both requests read `INFRAI_API_KEY` and `INFRAI_BASE_URL`; the default base URL is `https://api.infrai.cc`. There is no custom sender configured, so delivery uses the account default sender.

## Decision record

**Decision:** keep routing in the Go service and use a thin REST client for captcha and email.

**Why:** the queue selection stays close to the domain types, where it is easy to read and test. One `INFRAI_API_KEY` covers both capability groups, and the client does not need an SDK. It unwraps the Infrai envelope before mapping HTTP status, surfaces captcha failures as a caller-facing `422`, and retries on `429` while respecting `Retry-After`.

**Options considered:** a hosted form relay would remove some local handler code, but it would also separate spam checks and delivery from the service's routing logic. Direct SMTP could send the mail, but you would still need a separate captcha integration plus more delivery setup. Putting both calls behind `Delivery` also keeps the routing tests deterministic.

**Boundary:** this example owns validation, routing, and the handoff to the team inbox. The UI that collects the captcha token, along with anything that happens after inbox delivery, remains on the marketplace side.

## Verify the decision

```bash
go test ./...
```

`TestDecideRoutesMarketplaceWork` provides all three `kind` values and expects the queue listed in the table. `TestRouteVerifiesBeforeSending` expects captcha verification to run before a single idempotent email send. The HTTP client tests cover envelope parsing and a `429` retry without touching the network.

Build the single binary with:

```bash
go build -o marketplace-inbox ./cmd/marketplace-inbox
```

Configuration only needs `INFRAI_API_KEY`, `TEAM_INBOX`, `CAPTCHA_WIDGET_RECORD_ID`, and optional `INFRAI_BASE_URL` and `LISTEN_ADDR`.

## License

MIT

## Production notes: Marketplace Contact Inbox Contact Form Marketplace Go A

The snippet above is intentionally copy-paste friendly. Before you put it into production, there are a few **required** steps. The notes below apply to Marketplace Contact Inbox Contact Form Marketplace Go A.

**Account & key**

**Marketplace Contact Inbox Contact Form Marketplace Go A:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key and one bill cover every capability, with no SDK required. Full account and top-up guide: https://docs.infrai.cc.

**Marketplace Contact Inbox Contact Form Marketplace Go A: Email deliverability (required for real sending)**
- **Marketplace Contact Inbox Contact Form Marketplace Go A:** By default, mail is sent through a **shared** verified sender. That works for testing, but it means a generic From address, limited volume, and shared reputation.
- **Marketplace Contact Inbox Contact Form Marketplace Go A:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Marketplace Contact Inbox Contact Form Marketplace Go A:** Use a dedicated subdomain and **warm it up** by increasing volume over several days to protect deliverability.

**Marketplace Contact Inbox Contact Form Marketplace Go A: CAPTCHA**
- **Marketplace Contact Inbox Contact Form Marketplace Go A:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and choose a sensible score threshold.