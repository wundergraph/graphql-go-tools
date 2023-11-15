package graphql

import (
	"context"
	"errors"
	"sync"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/operationreport"
)

var (
	ErrRequiredStagesMissing = errors.New("required stages for custom execution engine v2 are missing")
)

type CustomExecutionEngineV2NormalizerStage interface {
	Normalize(operation *Request) error
}

type CustomExecutionEngineV2ValidatorStage interface {
	ValidateForSchema(operation *Request) error
}

type CustomExecutionEngineV2InputValidationStage interface {
	InputValidation(operation *Request) error
}

type CustomExecutionEngineV2ResolverStage interface {
	Setup(ctx context.Context, postProcessor *postprocess.Processor, resolveContext *resolve.Context, operation *Request, options ...ExecutionOptionsV2)
	Plan(postProcessor *postprocess.Processor, operation *Request, report *operationreport.Report) (plan.Plan, error)
	Resolve(resolveContext *resolve.Context, planResult plan.Plan, writer resolve.FlushWriter) error
	Teardown()
}

type CustomExecutionEngineV2 interface {
	CustomExecutionEngineV2NormalizerStage
	CustomExecutionEngineV2ValidatorStage
	CustomExecutionEngineV2ResolverStage
	CustomExecutionEngineV2InputValidationStage
}

type ExecutionEngineV2Executor interface {
	Execute(ctx context.Context, operation *Request, writer resolve.FlushWriter, options ...ExecutionOptionsV2) error
}

type CustomExecutionEngineV2Stages struct {
	RequiredStages CustomExecutionEngineV2RequiredStages
	OptionalStages *CustomExecutionEngineV2OptionalStages
}

func (c *CustomExecutionEngineV2Stages) AllRequiredStagesProvided() bool {
	return c.RequiredStages.ResolverStage != nil
}

type CustomExecutionEngineV2RequiredStages struct {
	ResolverStage CustomExecutionEngineV2ResolverStage
}

type CustomExecutionEngineV2OptionalStages struct {
	NormalizerStage      CustomExecutionEngineV2NormalizerStage
	ValidatorStage       CustomExecutionEngineV2ValidatorStage
	InputValidationStage CustomExecutionEngineV2InputValidationStage
}

type CustomExecutionEngineV2Executor struct {
	ExecutionStages              CustomExecutionEngineV2Stages
	internalExecutionContextPool sync.Pool
}

func NewCustomExecutionEngineV2Executor(executionEngineV2 CustomExecutionEngineV2) (*CustomExecutionEngineV2Executor, error) {
	executionStages := CustomExecutionEngineV2Stages{
		RequiredStages: CustomExecutionEngineV2RequiredStages{
			ResolverStage: executionEngineV2,
		},
		OptionalStages: &CustomExecutionEngineV2OptionalStages{
			NormalizerStage: executionEngineV2,
			ValidatorStage:  executionEngineV2,
		},
	}

	return NewCustomExecutionEngineV2ExecutorByStages(executionStages)
}

func NewCustomExecutionEngineV2ExecutorByStages(executionStages CustomExecutionEngineV2Stages) (*CustomExecutionEngineV2Executor, error) {
	return &CustomExecutionEngineV2Executor{
		ExecutionStages: executionStages,
		internalExecutionContextPool: sync.Pool{
			New: func() interface{} {
				return newInternalExecutionContext()
			},
		},
	}, nil
}

func (c *CustomExecutionEngineV2Executor) getExecutionCtx() *internalExecutionContext {
	return c.internalExecutionContextPool.Get().(*internalExecutionContext)
}

func (c *CustomExecutionEngineV2Executor) putExecutionCtx(ctx *internalExecutionContext) {
	ctx.reset()
	c.internalExecutionContextPool.Put(ctx)
}

func (c *CustomExecutionEngineV2Executor) Execute(ctx context.Context, operation *Request, writer resolve.FlushWriter, options ...ExecutionOptionsV2) error {
	if !c.ExecutionStages.AllRequiredStagesProvided() {
		return ErrRequiredStagesMissing
	}

	var err error
	if c.ExecutionStages.OptionalStages != nil && c.ExecutionStages.OptionalStages.NormalizerStage != nil {
		err = c.ExecutionStages.OptionalStages.NormalizerStage.Normalize(operation)
		if err != nil {
			return err
		}
	}

	if c.ExecutionStages.OptionalStages != nil && c.ExecutionStages.OptionalStages.ValidatorStage != nil {
		err = c.ExecutionStages.OptionalStages.ValidatorStage.ValidateForSchema(operation)
		if err != nil {
			return err
		}
	}

	if c.ExecutionStages.OptionalStages != nil && c.ExecutionStages.OptionalStages.InputValidationStage != nil {
		if err := c.ExecutionStages.OptionalStages.InputValidationStage.InputValidation(operation); err != nil {
			return err
		}
	}

	execContext := c.getExecutionCtx()
	defer c.putExecutionCtx(execContext)
	execContext.prepare(ctx, operation.Variables, operation.request)
	c.ExecutionStages.RequiredStages.ResolverStage.Setup(ctx, execContext.postProcessor, execContext.resolveContext, operation, options...)

	var report operationreport.Report
	planResult, err := c.ExecutionStages.RequiredStages.ResolverStage.Plan(execContext.postProcessor, operation, &report)
	if err != nil {
		return err
	} else if report.HasErrors() {
		return report
	}

	err = c.ExecutionStages.RequiredStages.ResolverStage.Resolve(execContext.resolveContext, planResult, writer)
	if err != nil {
		return err
	}

	c.ExecutionStages.RequiredStages.ResolverStage.Teardown()
	return nil
}

// Interface Guards
var (
	_ ExecutionEngineV2Executor = (*CustomExecutionEngineV2Executor)(nil)
)
