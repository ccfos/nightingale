## azure

azure monitor 采集插件， 采集azure monitor指标


## Azure Monitor 

此插件使用 [Azure Monitor](https://learn.microsoft.com/en-us/azure/azure-monitor/) API 收集 Azure 资源的指标。插件需要 `client_id`、`client_secret` 和 `tenant_id` 用于访问令牌认证。访问 Azure 资源需要 `subscription_id`。

查看[支持的指标页面](https://learn.microsoft.com/en-us/azure/azure-monitor/reference/metrics-index)了解可用的资源类型及其指标。

> [重要]
>
> Azure API 每小时有 12,000 次读取限制。请确保在配置的时间间隔内，您的总指标数量不超过此限制。


## 注册服务主体
需要注册一个服务主体(service principal)

## 授权
访问azure monitor数据需要的权限是`Monitor Reader`,对应中文是`监视查阅者`


## 属性位置

`subscription_id` 可以在 Azure 门户中您的应用程序或服务的 `概述 > 要点` 下找到。

`client_id` 和 `client_secret` 可以通过在 `Azure Active Directory` 下注册应用程序获得。

`tenant_id` 可以在 `Azure Active Directory > 属性` 下找到。

资源目标 `resource_id` 可以在 Azure 门户中您的应用程序或服务的 `概述 > 要点 > JSON 视图` 下找到。

`cloud_option` 定义了 API 端点的可选值，用于从 Azure 主权云（如 `AzureChina`、`AzureGovernment` 或 `AzurePublic`）获取指标。
默认值为 AzurePublic。

## 使用方法

使用 `resource_targets` 通过资源 ID 从特定资源收集指标。

使用 `resource_group_targets` 从资源组下具有特定资源类型的资源收集指标。

使用 `subscription_targets` 从订阅下具有特定资源类型的资源收集指标。


## Configuration
```toml
# Gather Azure resources metrics from Azure Monitor API
# 每2m查询一次azure monitor, 查询时间范围也为2m
interval = "2m"
[[instances]]
# can be found under Overview->Essentials in the Azure portal for your application/service
subscription_id = ""
# can be obtained by registering an application under Azure Active Directory
client_id = ""
# can be obtained by registering an application under Azure Active Directory.
# If not specified Default Azure Credentials chain will be attempted:
# - Environment credentials (AZURE_*)
# - Workload Identity in Kubernetes cluster
# - Managed Identity
# - Azure CLI auth
# - Developer Azure CLI auth
client_secret = ""
# can be found under Azure Active Directory->Properties
tenant_id = ""
# Define the optional Azure cloud option e.g. AzureChina, AzureGovernment or AzurePublic. The default is AzurePublic.
# cloud_option = "AzurePublic"


# 聚合粒度
# 比如我采集周期是10m, 指标上报周期是30s, 那么在10m内会有20个原始点
# 经过该参数的聚合处理后，比如聚合粒度是1m, 那么就会获取到10个点
# 注意，最小值是1m
aggregation_interval = "1m"
# resource target #1 to collect metrics from
# [[instances.resource_target]]
# can be found under Overview->Essentials->JSON View in the Azure portal for your application/service
# must start with 'resourceGroups/...' ('/subscriptions/xxxxxxxx-xxxx-xxxx-xxx-xxxxxxxxxxxx'
# must be removed from the beginning of Resource ID property value)
# resource_id = "resourceGroups/flashcat/providers/Microsoft.Compute/virtualMachines/lfn-test"

# the metric names to collect
# leave the array empty to use all metrics available to this resource
# metrics = [ "<<METRIC>>", "<<METRIC>>" ]
# metrics aggregation type value to collect
# can be 'Total', 'Count', 'Average', 'Minimum', 'Maximum'
# leave the array empty to collect all aggregation types values for each metric
# aggregations = [ "<<AGGREGATION>>", "<<AGGREGATION>>" ]

# resource target #2 to collect metrics from
# [[instances.resource_target]]
# resource_id = "<<RESOURCE_ID>>"
# metrics = [ "<<METRIC>>", "<<METRIC>>" ]
# aggregations = [ "<<AGGREGATION>>", "<<AGGREGATION>>" ]
#
# # resource group target #1 to collect metrics from resources under it with resource type
[[instances.resource_group_target]]
# # the resource group name
resource_group = "flashcat"
#
# # defines the resources to collect metrics from
[[instances.resource_group_target.resource]]
# # the resource type
# resource_type = "Microsoft.Compute/virtualMachines"
# metrics = [ "<<METRIC>>", "<<METRIC>>" ]
# aggregations = [ "<<AGGREGATION>>", "<<AGGREGATION>>" ]
#
# # defines the resources to collect metrics from
# [[instances.resource_group_target.resource]]
# resource_type = "<<RESOURCE_TYPE>>"
# metrics = [ "<<METRIC>>", "<<METRIC>>" ]
# aggregations = [ "<<AGGREGATION>>", "<<AGGREGATION>>" ]
#
# # resource group target #2 to collect metrics from resources under it with resource type
# [[instances.resource_group_target]]
# resource_group = "<<RESOURCE_GROUP_NAME>>"
#
# [[instances.resource_group_target.resource]]
# resource_type = "<<RESOURCE_TYPE>>"
# metrics = [ "<<METRIC>>", "<<METRIC>>" ]
# aggregations = [ "<<AGGREGATION>>", "<<AGGREGATION>>" ]
#
# # subscription target #1 to collect metrics from resources under it with resource type
# [[instances.subscription_target]]
# resource_type = "<<RESOURCE_TYPE>>"
# metrics = [ "<<METRIC>>", "<<METRIC>>" ]
# aggregations = [ "<<AGGREGATION>>", "<<AGGREGATION>>" ]
#
# # subscription target #2 to collect metrics from resources under it with resource type
# [[instances.subscription_target]]
# resource_type = "<<RESOURCE_TYPE>>"
# metrics = [ "<<METRIC>>", "<<METRIC>>" ]
# aggregations = [ "<<AGGREGATION>>", "<<AGGREGATION>>" ]

```
