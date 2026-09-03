## azure

The azure monitor collection plugin collects Azure Monitor metrics.


## Azure Monitor 

This plugin uses the [Azure Monitor](https://learn.microsoft.com/en-us/azure/azure-monitor/) API to collect metrics for Azure resources. The plugin requires `client_id`, `client_secret` and `tenant_id` for access-token authentication. `subscription_id` is required to access Azure resources.

See the [supported metrics page](https://learn.microsoft.com/en-us/azure/azure-monitor/reference/metrics-index) for the available resource types and their metrics.

> [Important]
>
> The Azure API has a limit of 12,000 reads per hour. Please make sure the total number of metrics you collect within the configured interval does not exceed this limit.


## Register a Service Principal
You need to register a service principal.

## Authorization
The permission required to access Azure Monitor data is `Monitor Reader` (the Monitoring Reader role).


## Where to Find the Properties

`subscription_id` can be found under `Overview > Essentials` in the Azure portal for your application/service.

`client_id` and `client_secret` can be obtained by registering an application under `Azure Active Directory`.

`tenant_id` can be found under `Azure Active Directory > Properties`.

The resource target `resource_id` can be found under `Overview > Essentials > JSON View` in the Azure portal for your application/service.

`cloud_option` defines the optional API endpoint value used to fetch metrics from Azure sovereign clouds such as `AzureChina`, `AzureGovernment` or `AzurePublic`.
The default is AzurePublic.

## Usage

Use `resource_targets` to collect metrics from specific resources by resource ID.

Use `resource_group_targets` to collect metrics from resources of a specific resource type under a resource group.

Use `subscription_targets` to collect metrics from resources of a specific resource type under a subscription.


## Configuration
```toml
# Gather Azure resources metrics from Azure Monitor API
# Query azure monitor every 2m; the query time range is also 2m
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


# Aggregation granularity
# For example, if the collection interval is 10m and the metric reporting period is 30s, there will be 20 raw data points within 10m
# After aggregation with this parameter, e.g. with a granularity of 1m, you will get 10 points
# Note: the minimum value is 1m
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
