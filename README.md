# nomad-aliyun-autoscaler

The `aliyun` target plugin allows for the scaling of the Nomad cluster clients via creating and 
destroying aliyun ECS.

## Requirements

* nomad autoscaler
* aliyun account

## Documentation

### Agent Configuration Options

To use the `aliyun` target plugin, the agent configuration needs to be populated with the appropriate target block.
Currently, Acsess Key is the only method of authenticating with the API. You can manage your key at the [aliyun control panel](https://ram.console.aliyun.com/).

```
target "acs-ess" {
  driver = "acs-ess"
  config = {
    accessKeyId      = "AABBCC332211"
    accessKeySecret  = "EFG323DDEEFF"
  }
}
```
### Policy Configuration Options

``` hcl
check "hashistack-allocated-cpu" {
  # ...
  target "acs-ess" {
      region                  = "cn-shanghai"
      scalingGroupId          = "asg-123aaabbbccc456"
      node_class              = "aliyunNodeClassName"
      node_purge              = "true"
      node_drain_deadline     = "1m"
      node_selector_strategy  = "empty"
  }
  # ...
}
```

- `scalingGroupId` `(string: <required>)` - The unique ID for aliyun scaling group, you need to create a scaling group at first.
  
- `region` `(string: <required>)` - The region to start in.

- `node_class` `(string: <required>)` - The Nomad [client node class](https://www.nomadproject.io/docs/configuration/client#node_class)
  identifier used to group nodes into a pool of resource. Conflicts with
  `datacenter`.

- `node_drain_deadline` `(duration: "15m")` The Nomad [drain deadline](https://www.nomadproject.io/api-docs/nodes#deadline) to use when performing node draining
  actions. **Note that the default value for this setting differs from Nomad's
  default of 1h.**

- `node_drain_ignore_system_jobs` `(bool: "false")` A boolean flag used to
  control if system jobs should be stopped when performing node draining
  actions.

- `node_purge` `(bool: "false")` A boolean flag to determine whether Nomad
  clients should be [purged](https://www.nomadproject.io/api-docs/nodes#purge-node) when performing scale in
  actions.

- `node_selector_strategy` `(string: "least_busy")` The strategy to use when
  selecting nodes for termination. Refer to the [node selector
  strategy](https://www.nomadproject.io/docs/autoscaling/internals/node-selector-strategy) documentation for more information.
