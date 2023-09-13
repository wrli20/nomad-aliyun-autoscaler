package plugin

import (
	"context"
	"fmt"
	ess20220222 "github.com/alibabacloud-go/ess-20220222/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

type scalingGroup interface {
	getId() string
	status(ctx context.Context, client *ess20220222.Client) (bool, int32, error)
	listInstances(ctx context.Context, client *ess20220222.Client) ([]*ess20220222.DescribeScalingInstancesResponseBodyScalingInstances, error)
	resize(ctx context.Context, client *ess20220222.Client, num int32) (*string, error)
	deleteInstances(ctx context.Context, client *ess20220222.Client, instanceIDs []string) (*string, error)
	scalingActivityStatus(ctx context.Context, client *ess20220222.Client, scalingActivityId string) (bool, error)
}

type regionalScalingGroup struct {
	region string
	id     string
}

func (r *regionalScalingGroup) getId() string {
	return r.id
}

func (r *regionalScalingGroup) status(ctx context.Context, client *ess20220222.Client) (bool, int32, error) {
	request := &ess20220222.DescribeScalingGroupsRequest{
		RegionId:        tea.String(r.region),
		ScalingGroupIds: []*string{tea.String(r.id)},
	}

	res, err := client.DescribeScalingGroups(request)
	if err != nil {
		return false, -1, err
	}

	if tea.Int32Value(res.Body.TotalCount) != 1 {
		return false, -1, fmt.Errorf("required scaling group id %s not found", r.id)
	}
	return tea.StringValue(res.Body.ScalingGroups[0].LifecycleState) == "Active", tea.Int32Value(res.Body.ScalingGroups[0].TotalCapacity), nil
}

func (r *regionalScalingGroup) listInstances(ctx context.Context, client *ess20220222.Client) ([]*ess20220222.DescribeScalingInstancesResponseBodyScalingInstances, error) {
	request := &ess20220222.DescribeScalingInstancesRequest{
		RegionId:       tea.String(r.region),
		ScalingGroupId: tea.String(r.id),
	}

	res, err := client.DescribeScalingInstances(request)
	if err != nil {
		return nil, err
	}
	return res.Body.ScalingInstances, nil
}

func (r *regionalScalingGroup) resize(ctx context.Context, client *ess20220222.Client, num int32) (*string, error) {
	request := &ess20220222.ScaleWithAdjustmentRequest{
		ScalingGroupId:  tea.String(r.id),
		AdjustmentType:  tea.String("TotalCapacity"),
		AdjustmentValue: tea.Int32(num),
	}
	res, err := client.ScaleWithAdjustment(request)
	if err != nil {
		return nil, err
	}
	return res.Body.ScalingActivityId, err
}

func (r *regionalScalingGroup) deleteInstances(ctx context.Context, client *ess20220222.Client, instanceIDs []string) (*string, error) {
	request := &ess20220222.RemoveInstancesRequest{
		ScalingGroupId: tea.String(r.id),
		InstanceIds:    tea.StringSlice(instanceIDs),
	}
	res, err := client.RemoveInstances(request)
	if err != nil {
		return nil, err
	}
	return res.Body.ScalingActivityId, err
}

func (r *regionalScalingGroup) scalingActivityStatus(ctx context.Context, client *ess20220222.Client, scalingActivityId string) (bool, error) {
	request := &ess20220222.DescribeScalingActivitiesRequest{
		RegionId:           tea.String(r.region),
		ScalingGroupId:     tea.String(r.id),
		ScalingActivityIds: []*string{tea.String(scalingActivityId)},
	}

	res, err := client.DescribeScalingActivities(request)
	if err != nil {
		return false, err
	}

	if tea.Int32Value(res.Body.TotalCount) != 1 {
		return false, fmt.Errorf("required scaling activity id %s not found", scalingActivityId)
	}
	return tea.Int32Value(res.Body.ScalingActivities[0].Progress) == 100, nil
}
