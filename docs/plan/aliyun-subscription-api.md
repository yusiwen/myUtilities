# Aliyun Subscription API Discovery (QoderCN credits)

## Background

`mu budget balance -p aliyun` can only check **account balance** and **resource packages**, but cannot query subscription credits usage info from "My Subscriptions" (such as QoderCN Personal edition).

## Currently Used APIs

`core/budget/providers/aliyun.go` calls the BSS (Billing Support System) OpenAPI:

| Action | Endpoint | Version | Return Content |
|---|---|---|---|
| `QueryAccountBalance` | `https://business.aliyuncs.com/` | 2017-12-14 | Account available balance / cash / credit control |
| `QueryResourcePackageInstances` | `https://business.aliyuncs.com/` | 2017-12-14 | **Resource package** instance remaining quantity |

Signature method: HMAC-SHA1 + GET query signature (`signAliyun` + `buildAliyunURL` in `aliyun_sign.go`).

## Root Cause

- Resource packages (returned by `QueryResourcePackageInstances`) and subscriptions ("My Subscriptions" page) are
  **two different billing models, two different data sources**.
- `QueryResourcePackageInstances` only returns resource package instances, inherently does not contain subscription credits/usage.

## Key Finding

The BSS OpenAPI corresponding to the "My Subscriptions" page (`billing-cost.console.aliyun.com/subscription/overview/list`) is **`QueryAvailableInstances`** (BssOpenApi 2017-12-14).

Fully same-source with existing Action:
- Endpoint `https://business.aliyuncs.com/`
- `Version: 2017-12-14`, HMAC-SHA1, GET + query signature
- Can fully reuse `signAliyun` + `buildAliyunURL`, zero new dependencies

Returns **subscription instance** metadata, typical fields:

| Field | Meaning |
|---|---|
| `InstanceId` | Instance ID |
| `ProductCode` / `ProductType` | Product |
| `SubscriptionType` | `Subscription` (subscription) / `PayAsYouGo` (pay-as-you-go) |
| `InstanceStatus` | Status (normal / arrears / expired, etc.) |
| `CreateTime` / `EndTime` / `ExpectedReleaseTime` | Order / expiry time |
| `RenewStatus` | Renewal status |

## Risks / Unconfirmed Items

1. `QueryAvailableInstances` returns subscription instance **ordering status metadata**, typically **does not contain** credits remaining quantity / usage.
2. QoderCN is an independent subscription (Personal edition), its credits usage likely needs to be queried via
    **QoderCN's own product backend / API**, Aliyun BSS cannot provide it.
3. Actual response fields need real AccessKey testing to confirm.

## Suggested Implementation Path

### Step 1: Add `GetSubscriptions()`

Add to `core/budget/providers/aliyun.go`:

```go
func (p *aliyunProvider) GetSubscriptions(ctx context.Context) ([]SubscriptionInstance, error) {
    params := map[string]string{
        "Action":           "QueryAvailableInstances",
        "AccessKeyId":      p.accessKeyID,
        "PageSize":         "100",
        "SubscriptionType": "Subscription", // subscriptions only
    }
    signAliyun(params, p.accessKeySecret)
    // Parse Data.InstanceList.Instance[]
}
```

### Step 2: Integrate into budget output

Add a "Subscriptions" section in the aliyun `printBalance` block (`budget/command.go`),
similar to existing resource package display: product, instance ID, status, expiry time.

### Step 3: Real testing and verification

Run `mu budget balance -p aliyun` with AccessKey:
- If response contains credits/usage fields → display directly
- If not → subscription list can be displayed, QoderCN credits need to find QoderCN's own API

## To Confirm

1. Is the goal "subscription list" (`QueryAvailableInstances` can satisfy) or
    "QoderCN credits remaining quantity" (needs QoderCN's own API).
2. Whether QoderCN has an open API / token / credits query interface.
