package plugin

import (
	"context"
	"errors"
	"fmt"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ess20220222 "github.com/alibabacloud-go/ess-20220222/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/nomad/api"
	"time"
)

const (
	defaultRetryInterval = 10 * time.Second
	defaultRetryLimit    = 15

	// acsEndpoint is the domain name that other services
	// can use to access ESS.
	acsEndpoint = "ess.aliyuncs.com"

	// nodeHostname is the node attribute to use when identifying the
	// ACS hostname of a node.
	nodeHostname = "unique.hostname"
)

func (t *TargetPlugin) setupAcsClients(config map[string]string) (err error) {
	accessKeyId, ok := config[configAccessKeyId]
	accessKeySecret, ok := config[configAccessKeySecret]
	endpoint, endpointOk := config[configEndpoint]
	if !ok {
		fmt.Errorf("required config param %s or %s not found", configAccessKeyId, configAccessKeySecret)
	}

	c := &openapi.Config{
		// AccessKey ID
		AccessKeyId: tea.String(accessKeyId),
		// AccessKey Secret
		AccessKeySecret: tea.String(accessKeySecret),

		Endpoint: tea.String(acsEndpoint),
	}

	if endpointOk {
		c.Endpoint = tea.String(endpoint)
	}

	t.client, err = ess20220222.NewClient(c)
	if err != nil {
		return fmt.Errorf("failed to create acs-ess ess sdk client: %v", err)
	}

	return nil
}

func (t *TargetPlugin) status(ctx context.Context, sg scalingGroup) (bool, int32, error) {
	return sg.status(ctx, t.client)
}

func (t *TargetPlugin) scaleOut(ctx context.Context, sg scalingGroup, num int32) error {
	log := t.logger.With("action", "scale_out", "scaling_group", sg.getId())
	activityId, err := sg.resize(ctx, t.client, num)
	if err != nil {
		return fmt.Errorf("failed to scale out acs-ess scaling group: %v", err)
	}
	if err := t.ensureScalingActivityIsDone(ctx, sg, tea.StringValue(activityId)); err != nil {
		return fmt.Errorf("failed to confirm scale out acs-ess scaling group: %v", err)
	}
	log.Debug("scale out acs-ess scaling group confirmed")
	return nil
}

func (t *TargetPlugin) scaleIn(ctx context.Context, sg scalingGroup, num int32, config map[string]string) error {
	// Create a logger for this action to pre-populate useful information we
	// would like on all log lines.
	log := t.logger.With("action", "scale_in", "instance_group", sg.getId())

	// Find instance IDs in the target scaling group and perform pre-scale tasks.
	instances, err := sg.listInstances(ctx, t.client)
	if err != nil {
		return fmt.Errorf("failed to list acs-ess scaling group instances: %v", err)
	}

	remoteIDs := []string{}
	for _, inst := range instances {
		if tea.StringValue(inst.HealthStatus) == "Healthy" && tea.StringValue(inst.LifecycleState) == "InService" {
			log.Debug("found healthy instance", "instance_id", tea.StringValue(inst.InstanceId))

			remoteIDs = append(remoteIDs, tea.StringValue(inst.InstanceId))
		} else {
			log.Debug("skipping instance", "instance_id", tea.StringValue(inst.InstanceId), "lifecycle_state",
				tea.StringValue(inst.LifecycleState))
		}
	}

	ids, err := t.clusterUtils.RunPreScaleInTasksWithRemoteCheck(ctx, config, remoteIDs, int(num))
	if err != nil {
		return fmt.Errorf("failed to perform pre-scale Nomad scale in tasks: %v", err)
	}

	// Grab the instanceIDs
	var instanceIDs []string

	for _, node := range ids {
		instanceIDs = append(instanceIDs, node.RemoteResourceID)
	}

	// Delete the instances from the scaling group. The targetSize of the group is will be reduced by the
	// number of instances that are deleted.
	log.Debug("deleting acs-ess scaling group instances", "instances", ids)

	activityId, err := sg.deleteInstances(ctx, t.client, instanceIDs)
	if err != nil {
		return fmt.Errorf("failed to delete instances: %v", err)
	}

	log.Info("successfully started deleted instances activity in acs-ess scaling group")
	if err := t.ensureScalingActivityIsDone(ctx, sg, tea.StringValue(activityId)); err != nil {
		return fmt.Errorf("failed to confirm scale in acs-ess scaling group: %v", err)
	}

	log.Debug("scale in acs-ess scaling group confirmed")

	// Run any post scale in tasks that are desired.
	if err := t.clusterUtils.RunPostScaleInTasks(ctx, config, ids); err != nil {
		return fmt.Errorf("failed to perform post-scale Nomad scale in tasks: %v", err)
	}

	return nil
}

func (t *TargetPlugin) ensureScalingActivityIsDone(ctx context.Context, group scalingGroup, scalingActivityId string) error {

	f := func(ctx context.Context) (bool, error) {
		ok, err := group.scalingActivityStatus(ctx, t.client, scalingActivityId)
		if ok || err != nil {
			return true, err
		} else {
			return false, errors.New("waiting for scaling activity to be done")
		}
	}

	return retry(ctx, defaultRetryInterval, defaultRetryLimit, f)
}

// acsNodeIDMap is used to identify the ESS Instance of a Nomad node using the
// relevant attribute value.
func acsNodeIDMap(n *api.Node) (string, error) {
	hostname, ok := n.Attributes[nodeHostname]
	if !ok {
		return "", fmt.Errorf("attribute %q not found", nodeHostname)
	}
	return hostname, nil
}
