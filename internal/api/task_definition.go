package api

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/mohsiur/a16s/internal/utils"
	"golang.org/x/sync/errgroup"
)

const (
	MaxTaskDefinitionFamily   = 100
	MaxTaskDefinitionRevision = 20
)

// Equivalent to
// aws ecs describe-task-definition --task-definition ${taskDefinition}
func (c *Clients) DescribeTaskDefinition(tdArn *string) (types.TaskDefinition, error) {

	include := []types.TaskDefinitionField{
		types.TaskDefinitionFieldTags,
	}
	taskDefinition, err := c.ECS().DescribeTaskDefinition(context.Background(), &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: tdArn,
		Include:        include,
	})
	if err != nil {
		slog.Warn("failed to run aws api to describe task definition", "error", err)
		return types.TaskDefinition{}, err
	}

	return *taskDefinition.TaskDefinition, nil
}

type TaskDefinitionRevision = []string

// Equivalent to
// aws ecs list-task-definitions --family-prefix ${prefix}
func (c *Clients) ListTaskDefinition(familyName *string) (TaskDefinitionRevision, error) {
	listTaskDefinitions, err := c.ECS().ListTaskDefinitions(context.Background(), &ecs.ListTaskDefinitionsInput{
		FamilyPrefix: familyName,
		MaxResults:   aws.Int32(MaxTaskDefinitionRevision),
		Sort:         types.SortOrderDesc,
	})
	if err != nil {
		slog.Warn("failed to run aws api to list task definitions", "error", err)
		return nil, err
	}

	return listTaskDefinitions.TaskDefinitionArns, nil
}

// List given task definition revision with contents
// Equivalent to
// aws ecs list-task-definitions --family-prefix ${prefix}
// aws ecs describe-task-definition --task-definition ${taskDefinition}
func (c *Clients) ListFullTaskDefinition(taskDefinition *string) ([]types.TaskDefinition, error) {
	td := strings.Split(utils.ArnToName(taskDefinition), ":")
	familyName := td[0]
	list, err := c.ListTaskDefinition(&familyName)

	if err != nil {
		slog.Warn("failed to run aws api to run list task definition in ListFullTaskDefinition", "error", err)
		return []types.TaskDefinition{}, err
	}

	results := []types.TaskDefinition{}
	g := new(errgroup.Group)

	for _, t := range list {
		t := t
		g.Go(func() error {
			d, err := c.DescribeTaskDefinition(&t)
			if err != nil {
				slog.Warn("failed to run aws api to describe task definition", "error", err)
				return err
			}
			results = append(results, d)
			return nil
		})
	}

	err = g.Wait()

	return results, err
}

// Equivalent to
// aws ecs list-task-definition-families --family-prefix ${prefix}
func (c *Clients) ListTaskDefinitionFamilies(familyPrefix *string) ([]string, error) {
	familiesOutput, err := c.ECS().ListTaskDefinitionFamilies(context.Background(), &ecs.ListTaskDefinitionFamiliesInput{
		FamilyPrefix: familyPrefix,
		MaxResults:   aws.Int32(MaxTaskDefinitionFamily),
		Status:       types.TaskDefinitionFamilyStatusActive,
	})
	if err != nil {
		slog.Warn("failed to run aws api to list task definition families", "error", err)
		return nil, err
	}

	return familiesOutput.Families, nil
}
