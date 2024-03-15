package graphql

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

var (
	errFailedNormalizationStage = errors.New("normalization stage failed")
	errFailedValidationStage    = errors.New("validation stage failed")
	errFailedPlanning           = errors.New("planning failed")
	errFailedResolve            = errors.New("resolve failed")
)

var (
	reportPlanError = operationreport.ExternalError{
		Message:   "planner report error",
		Path:      nil,
		Locations: nil,
	}
)

func TestCustomExecutionEngineV2Stages_AllRequiredStagesProvided(t *testing.T) {
	t.Run("should return false when requirements are not met", func(t *testing.T) {
		stages := CustomExecutionEngineV2Stages{}
		assert.False(t, stages.AllRequiredStagesProvided())
	})

	t.Run("should return true when requirements are met", func(t *testing.T) {
		stages := CustomExecutionEngineV2Stages{
			RequiredStages: CustomExecutionEngineV2RequiredStages{
				ResolverStage: testCustomExecutionV2ResolverStage{},
			},
		}
		assert.True(t, stages.AllRequiredStagesProvided())
	})
}

func TestCustomExecutionEngineV2Executor_Execute(t *testing.T) {
	run := func(stages CustomExecutionEngineV2Stages, expectedErr error) func(t *testing.T) {
		return func(t *testing.T) {
			writer := NewEngineResultWriter()
			executor, err := NewCustomExecutionEngineV2ExecutorByStages(stages)
			require.NoError(t, err)
			actualErr := executor.Execute(context.Background(), &Request{}, &writer, nil)
			assert.Equal(t, expectedErr, actualErr)
		}
	}

	t.Run("should return error when requirements are not met", run(
		CustomExecutionEngineV2Stages{},
		ErrRequiredStagesMissing,
	))

	t.Run("should return error when normalization stage fails", run(
		CustomExecutionEngineV2Stages{
			RequiredStages: CustomExecutionEngineV2RequiredStages{
				ResolverStage: testCustomExecutionV2ResolverStage{},
			},
			OptionalStages: &CustomExecutionEngineV2OptionalStages{
				NormalizerStage: testCustomExecutionV2NormalizationStage{
					failNormalization: true,
				},
			},
		},
		errFailedNormalizationStage,
	))

	t.Run("should return error when validation stage fails", run(
		CustomExecutionEngineV2Stages{
			RequiredStages: CustomExecutionEngineV2RequiredStages{
				ResolverStage: testCustomExecutionV2ResolverStage{},
			},
			OptionalStages: &CustomExecutionEngineV2OptionalStages{
				ValidatorStage: testCustomExecutionV2ValidationStage{
					failValidation: true,
				},
			},
		},
		errFailedValidationStage,
	))

	t.Run("should return error when planning fails", run(
		CustomExecutionEngineV2Stages{
			RequiredStages: CustomExecutionEngineV2RequiredStages{
				ResolverStage: testCustomExecutionV2ResolverStage{
					failPlan: true,
				},
			},
		},
		errFailedPlanning,
	))

	t.Run("should return error when planner report has errors", run(
		CustomExecutionEngineV2Stages{
			RequiredStages: CustomExecutionEngineV2RequiredStages{
				ResolverStage: testCustomExecutionV2ResolverStage{
					planReportHasErrors: true,
				},
			},
		},
		&operationreport.Report{
			ExternalErrors: []operationreport.ExternalError{
				reportPlanError,
			},
		},
	))

	t.Run("should return error when resolve fails", run(
		CustomExecutionEngineV2Stages{
			RequiredStages: CustomExecutionEngineV2RequiredStages{
				ResolverStage: testCustomExecutionV2ResolverStage{
					failResolve: true,
				},
			},
		},
		errFailedResolve,
	))

	t.Run("should not return an error when nothing fails", run(
		CustomExecutionEngineV2Stages{
			RequiredStages: CustomExecutionEngineV2RequiredStages{
				ResolverStage: testCustomExecutionV2ResolverStage{},
			},
			OptionalStages: &CustomExecutionEngineV2OptionalStages{
				NormalizerStage: testCustomExecutionV2NormalizationStage{},
				ValidatorStage:  testCustomExecutionV2ValidationStage{},
			},
		},
		nil,
	))
}

type testCustomExecutionV2NormalizationStage struct {
	failNormalization bool
}

func (t testCustomExecutionV2NormalizationStage) Normalize(operation *Request) error {
	if t.failNormalization {
		return errFailedNormalizationStage
	}
	return nil
}

type testCustomExecutionV2ResolverStage struct {
	failPlan            bool
	planReportHasErrors bool
	failResolve         bool
}

type testCustomExecutionV2ValidationStage struct {
	failValidation bool
}

func (t testCustomExecutionV2ValidationStage) ValidateForSchema(operation *Request) error {
	if t.failValidation {
		return errFailedValidationStage
	}
	return nil
}

func (t testCustomExecutionV2ResolverStage) Setup(ctx context.Context, postProcessor *postprocess.Processor, resolveContext *resolve.Context, operation *Request, options ...ExecutionOptionsV2) {
}

func (t testCustomExecutionV2ResolverStage) Plan(postProcessor *postprocess.Processor, operation *Request, report *operationreport.Report) (plan.Plan, error) {
	if t.failPlan {
		return nil, errFailedPlanning
	}
	if t.planReportHasErrors {
		report.AddExternalError(reportPlanError)
		return nil, report
	}
	return &plan.SynchronousResponsePlan{}, nil
}

func (t testCustomExecutionV2ResolverStage) Resolve(resolveContext *resolve.Context, planResult plan.Plan, writer resolve.SubscriptionResponseWriter) error {
	if t.failResolve {
		return errFailedResolve
	}
	return nil
}

func (t testCustomExecutionV2ResolverStage) Teardown() {}
