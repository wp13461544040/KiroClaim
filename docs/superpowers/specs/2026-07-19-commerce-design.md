# KiroClaim Commerce Design

## Scope

Add a guest storefront for purchasing cards, an administrator commerce console, configurable payment channels, manual payment review, automatic gateway callbacks, two inventory strategies, proof uploads, and SMTP notifications.

## Payment Channels

Channels are records, not hard-coded singleton settings. A channel has a display type (`wechat`, `alipay`, `third_party`, `crypto`, or `manual`), public checkout instructions, and encrypted private configuration.

Automatic channels use a normalized HMAC HTTP gateway contract. KiroClaim sends an order creation request to the configured gateway and receives a payment URL or QR payload. Gateways post normalized payment results to a channel-specific callback URL. Signatures cover the timestamp and raw request body; event IDs and provider transaction IDs are unique and idempotent. This lets administrators configure multiple official-provider bridges, aggregate gateways, or crypto processors without coupling order state to a vendor protocol.

Manual channels display configured QR codes, wallet addresses, bank details, or instructions. Each channel configures which proof fields are required. Submitted orders enter review and stop expiring until an administrator approves, rejects, or asks for more evidence.

## Catalog And Inventory

Products contain a name, description, base amount/currency, account subscription, account count, active flag, and inventory mode. Channel prices may override amount and currency.

`generated` inventory creates a new Card only after payment confirmation. It reserves capacity with an order reservation and rechecks delivery capacity during fulfillment. `pre_generated` inventory assigns unused Cards to a product pool; order creation atomically reserves one pool entry and expiration releases it.

## Guest Orders

Guests provide contact information and receive an order number plus a generated query password. Only a hash is stored. Querying, uploading proof, and viewing the delivered card require both values. Order snapshots preserve product, price, currency, channel, subscription, and account-count values.

Statuses are `pending_payment`, `payment_review`, `paid`, `fulfilling`, `completed`, `paid_attention`, `expired`, `cancelled`, `rejected`, and `refunded`. Payment confirmation and fulfillment are idempotent. A paid delivery failure becomes `paid_attention` and is never silently completed or returned to unpaid state.

## Storage And Notifications

Proof storage supports local private files in the first-party implementation and an S3-compatible endpoint through a small storage interface. Uploads are limited by MIME type, file count, and size, and are served through authorized handlers rather than a public static directory.

SMTP settings include host, port, TLS mode, credentials, sender identity, completion notifications, and whether a message may include the full card code. Mail failures are logged and retryable, and never roll back a completed order.

## Administration

The commerce console manages products, channel instances, channel prices, pre-generated inventory, orders, manual review, paid-attention recovery, refund records, SMTP, and proof storage settings. Sensitive channel and SMTP secrets are write-only in API responses.

## Security

Amounts use integer minor units. Order transitions use transactions and conditional updates. Query passwords are bcrypt hashes. Callback signatures are constant-time checked with a replay window. Proof filenames are generated and paths are not accepted from clients. Administrative mutations use existing JWT middleware and operation logging.
