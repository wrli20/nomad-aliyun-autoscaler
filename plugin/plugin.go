package plugin

import (
	"context"
	"fmt"
	ess20220222 "github.com/alibabacloud-go/ess-20220222/v2/client"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/nomad-autoscaler/plugins"
	"github.com/hashicorp/nomad-autoscaler/plugins/base"
	"github.com/hashicorp/nomad-autoscaler/plugins/target"
	"github.com/hashicorp/nomad-autoscaler/sdk"
	"github.com/hashicorp/nomad-autoscaler/sdk/helper/nomad"
	"github.com/hashicorp/nomad-autoscaler/sdk/helper/scaleutils"
)

const (
	// pluginName is the unique name of the this plugin amongst Target plugins.
	pluginName = "acs-ess"

	configAccessKeyId     = "accessKeyId"
	configAccessKeySecret = "accessKeySecret"
	configEndpoint        = "endpoint"
	configRegion          = "region"
	configScalingGroupId  = "scalingGroupId"
)

var (
	PluginConfig = &plugins.InternalPluginConfig{
		Factory: func(l hclog.Logger) interface{} { return NewAcsPlugin(l) },
	}

	pluginInfo = &base.PluginInfo{
		Name:       pluginName,
		PluginType: sdk.PluginTypeTarget,
	}
)

// Assert that TargetPlugin meets the target.Target interface.
var _ target.Target = (*TargetPlugin)(nil)

// TargetPlugin is the acs-ess implementation of the target.Target interface.
type TargetPlugin struct {
	config map[string]string
	logger hclog.Logger

	// clusterUtils provides general cluster scaling utilities for querying the
	// state of nodes pools and performing scaling tasks.
	clusterUtils *scaleutils.ClusterScaleUtils
	client       *ess20220222.Client
}

// NewAcsPlugin returns the acs-ess implementation of the target.Target
// interface.
func NewAcsPlugin(log hclog.Logger) *TargetPlugin {
	return &TargetPlugin{
		logger: log,
	}
}

// SetConfig satisfies the SetConfig function on the base.Base interface.
func (t *TargetPlugin) SetConfig(config map[string]string) error {

	t.config = config

	if err := t.setupAcsClients(config); err != nil {
		return err
	}

	clusterUtils, err := scaleutils.NewClusterScaleUtils(nomad.ConfigFromNamespacedMap(config), t.logger)
	if err != nil {
		return err
	}

	// Store and set the remote ID callback function.
	t.clusterUtils = clusterUtils
	t.clusterUtils.ClusterNodeIDLookupFunc = acsNodeIDMap

	return nil
}

// PluginInfo satisfies the PluginInfo function on the base.Base interface.
func (t *TargetPlugin) PluginInfo() (*base.PluginInfo, error) {
	return pluginInfo, nil
}

// Scale satisfies the Scale function on the target.Target interface.
func (t *TargetPlugin) Scale(action sdk.ScalingAction, config map[string]string) error {

	// acs-ess can't support dry-run like Nomad, so just exit.
	if action.Count == sdk.StrategyActionMetaValueDryRunCount {
		return nil
	}

	sg, err := t.calculateScalingGroup(config)
	if err != nil {
		return err
	}

	ctx := context.Background()

	_, currentCount, err := t.status(ctx, sg)
	if err != nil {
		return fmt.Errorf("failed to describe acs-ess scaling group: %v", err)
	}

	num, direction := t.calculateDirection(int64(currentCount), action.Count)

	switch direction {
	case "in":
		err = t.scaleIn(ctx, sg, int32(num), config)
	case "out":
		err = t.scaleOut(ctx, sg, int32(num))
	default:
		t.logger.Info("scaling not required", "ess_id", sg.getId(),
			"current_count", currentCount, "strategy_count", action.Count)
		return nil
	}

	// If we received an error while scaling, format this with an outer message
	// so its nice for the operators and then return any error to the caller.
	if err != nil {
		err = fmt.Errorf("failed to perform scaling action: %v", err)
	}
	return err
}

// Status satisfies the Status function on the target.Target interface.
func (t *TargetPlugin) Status(config map[string]string) (*sdk.TargetStatus, error) {

	// Perform our check of the Nomad node pool. If the pool is not ready, we
	// can exit here and avoid calling the acs-ess API as it won't affect the
	// outcome.
	ready, err := t.clusterUtils.IsPoolReady(config)
	if err != nil {
		return nil, fmt.Errorf("failed to run Nomad node readiness check: %v", err)
	}
	if !ready {
		return &sdk.TargetStatus{Ready: ready}, nil
	}

	group, err := t.calculateScalingGroup(config)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	stable, currentCount, err := t.status(ctx, group)
	if err != nil {
		return nil, fmt.Errorf("failed to describe acs-ess scaling group: %v", err)
	}
	resp := sdk.TargetStatus{
		Ready: stable,
		Count: int64(currentCount),
		Meta:  make(map[string]string),
	}

	return &resp, nil
}

func (t *TargetPlugin) calculateDirection(migTarget, strategyDesired int64) (int64, string) {
	if strategyDesired < migTarget {
		return migTarget - strategyDesired, "in"
	}
	if strategyDesired > migTarget {
		return strategyDesired, "out"
	}
	return 0, ""
}

func (t *TargetPlugin) calculateScalingGroup(config map[string]string) (scalingGroup, error) {

	// We cannot scale an acs-ess scaling group without knowing the group region.
	region, ok := t.getValue(config, configRegion)
	if !ok {
		return nil, fmt.Errorf("required config param %s not found", configRegion)
	}

	// We cannot scale an acs-ess scaling group without knowing the group id.
	id, ok := t.getValue(config, configScalingGroupId)
	if !ok {
		return nil, fmt.Errorf("required config param %s not found", configScalingGroupId)
	}
	return &regionalScalingGroup{
		region: region,
		id:     id,
	}, nil
}

func (t *TargetPlugin) getValue(config map[string]string, name string) (string, bool) {
	v, ok := config[name]
	if ok {
		return v, true
	}

	v, ok = t.config[name]
	if ok {
		return v, true
	}

	return "", false
}
