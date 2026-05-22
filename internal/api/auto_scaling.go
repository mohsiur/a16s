package api

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
)

type AutoScalingData struct {
	Targets    []types.ScalableTarget
	Policies   []types.ScalingPolicy
	Actions    []types.ScheduledAction
	Activities []types.ScalingActivity
}

func (c *Clients) GetAutoscaling(serviceArn *string) (*AutoScalingData, error) {
	targets, err := c.describeScalableTargets(serviceArn)

	if err != nil {
		return nil, err
	}

	policies, err := c.describeScalingPolicies(serviceArn)

	if err != nil {
		return nil, err
	}

	activities, err := c.describeScalingActivities(serviceArn)

	if err != nil {
		return nil, err
	}

	actions, err := c.describeScheduledAction(serviceArn)

	if err != nil {
		return nil, err
	}

	return &AutoScalingData{
		Targets:    targets,
		Policies:   policies,
		Actions:    actions,
		Activities: activities,
	}, nil

}

// Equivalent to
// aws application-autoscaling describe-scaling-activities --service-namespace ecs --resource-id {ServiceArn}
// Auto scaling logs
func (c *Clients) describeScalingActivities(serviceArn *string) ([]types.ScalingActivity, error) {
	activitiesInput := &applicationautoscaling.DescribeScalingActivitiesInput{
		ServiceNamespace: "ecs",
		ResourceId:       serviceArn,
		MaxResults:       aws.Int32(10),
	}
	activitiesOutput, err := c.AutoScaling().DescribeScalingActivities(context.Background(), activitiesInput)

	if err != nil {
		slog.Warn("failed to run aws api to auto scaling activities", "serviceArn", *serviceArn, "error", err)
		return nil, err
	}

	return activitiesOutput.ScalingActivities, nil
}

// Equivalent to
// aws application-autoscaling describe-scalable-targets --service-namespace ecs --resource-ids {[ServiceArn]}
func (c *Clients) describeScalableTargets(serviceArn *string) ([]types.ScalableTarget, error) {
	targetsInput := &applicationautoscaling.DescribeScalableTargetsInput{
		ServiceNamespace: "ecs",
		ResourceIds:      []string{*serviceArn},
	}
	targetsOutput, err := c.AutoScaling().DescribeScalableTargets(context.Background(), targetsInput)

	if err != nil {
		slog.Warn("failed to run aws api to auto scaling activities", "serviceArn", *serviceArn, "error", err)
		return nil, err
	}

	return targetsOutput.ScalableTargets, nil
}

// Equivalent to
// aws application-autoscaling describe-scaling-policies --service-namespace ecs --resource-id "service/<ClusterName>/<ServiceName>"
func (c *Clients) describeScalingPolicies(serviceArn *string) ([]types.ScalingPolicy, error) {
	policiesInput := &applicationautoscaling.DescribeScalingPoliciesInput{
		ServiceNamespace: "ecs",
		ResourceId:       serviceArn,
	}
	policiesOutput, err := c.AutoScaling().DescribeScalingPolicies(context.Background(), policiesInput)

	if err != nil {
		slog.Warn("failed to run aws api to auto scaling activities", "serviceArn", *serviceArn, "error", err)
		return nil, err
	}

	return policiesOutput.ScalingPolicies, nil
}

// Equivalent to
// aws application-autoscaling describe-scheduled-actions --service-namespace ecs --resource-id "service/<ClusterName>/<ServiceName>"
func (c *Clients) describeScheduledAction(serviceArn *string) ([]types.ScheduledAction, error) {
	actionsInput := &applicationautoscaling.DescribeScheduledActionsInput{
		ServiceNamespace: "ecs",
		ResourceId:       serviceArn,
	}
	actionsOutput, err := c.AutoScaling().DescribeScheduledActions(context.Background(), actionsInput)

	if err != nil {
		slog.Warn("failed to run aws api to auto scaling scheduled actions", "serviceArn", *serviceArn, "error", err)
		return nil, err
	}

	return actionsOutput.ScheduledActions, nil
}
