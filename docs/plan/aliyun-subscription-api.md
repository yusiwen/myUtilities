# Aliyun Subscription API Discovery (QoderCN credits)

## 背景

`mu budget balance -p aliyun` 只能查到**账户余额**和**资源包**，但查不到
"我的订阅"里的订阅（如 QoderCN 个人版）的 credits 用量信息。

## 当前使用的 API

`core/budget/providers/aliyun.go` 调用 BSS（费用中心）OpenAPI：

| Action | 端点 | Version | 返回内容 |
|---|---|---|---|
| `QueryAccountBalance` | `https://business.aliyuncs.com/` | 2017-12-14 | 账户可用余额/现金/信控 |
| `QueryResourcePackageInstances` | `https://business.aliyuncs.com/` | 2017-12-14 | **资源包**实例剩余量 |

签名方式：HMAC-SHA1 + GET query 签名（`aliyun_sign.go` 的 `signAliyun` + `buildAliyunURL`）。

## 问题根因

- 资源包（`QueryResourcePackageInstances` 返回的）与订阅（"我的订阅"页）是
  **两个不同的计费模型、两个不同的数据源**。
- `QueryResourcePackageInstances` 只返回资源包实例，天然不含订阅的 credits/用量。

## 关键发现

"我的订阅"页（`billing-cost.console.aliyun.com/subscription/overview/list`）
对应的 BSS OpenAPI 是 **`QueryAvailableInstances`**（BssOpenApi 2017-12-14）。

与现有 Action 完全同源：
- 端点 `https://business.aliyuncs.com/`
- `Version: 2017-12-14`、HMAC-SHA1、GET + query 签名
- 可完全复用 `signAliyun` + `buildAliyunURL`，零新依赖

返回的是**订阅实例**的元数据，典型字段：

| 字段 | 含义 |
|---|---|
| `InstanceId` | 实例 ID |
| `ProductCode` / `ProductType` | 产品 |
| `SubscriptionType` | `Subscription`（订阅）/ `PayAsYouGo`（按量）|
| `InstanceStatus` | 状态（正常/欠费/已过期等）|
| `CreateTime` / `EndTime` / `ExpectedReleaseTime` | 订购/到期时间 |
| `RenewStatus` | 续费状态 |

## 风险点 / 未确认事项

1. `QueryAvailableInstances` 返回的是订阅实例的**订购状态元数据**，通常**不包含
   credits 剩余量/用量**。
2. QoderCN 是一个独立订阅（个人版），其 credits 用量很可能需要在
   **QoderCN 自己的产品后台/API** 查询，阿里云 BSS 拿不到。
3. 实际响应字段需用真实 AccessKey 实测确认。

## 建议实现路径

### 第一步：加 `GetSubscriptions()`

在 `core/budget/providers/aliyun.go` 增加：

```go
func (p *aliyunProvider) GetSubscriptions(ctx context.Context) ([]SubscriptionInstance, error) {
    params := map[string]string{
        "Action":           "QueryAvailableInstances",
        "AccessKeyId":      p.accessKeyID,
        "PageSize":         "100",
        "SubscriptionType": "Subscription", // 只看订阅
    }
    signAliyun(params, p.accessKeySecret)
    // 解析 Data.InstanceList.Instance[]
}
```

### 第二步：接入 budget 输出

在 aliyun 的 `printBalance` 区块（`budget/command.go`）增加"订阅"小节，
类似现有资源包展示：产品、实例 ID、状态、到期时间。

### 第三步：实测验证

用 AccessKey 跑 `mu budget balance -p aliyun`：
- 若响应含 credits/用量字段 → 直接展示
- 若不含 → 订阅列表可展示，QoderCN credits 需另找 QoderCN 自己的 API

## 待确认

1. 目标是"订阅列表"（`QueryAvailableInstances` 可满足）还是
   "QoderCN credits 剩余量"（需 QoderCN 自有 API）。
2. QoderCN 是否有开放 API / token / credits 查询接口。
