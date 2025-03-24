package v1beta4

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PatternSpecification struct {
	Client         client.Client
	NamespacedName types.NamespacedName
}

type Pattern interface {
	Setup(ctx context.Context) *ErrorPattern
	Diff(ctx context.Context) *ErrorPattern
	Remove(ctx context.Context) *ErrorPattern
}

type reason string

const (
	Errored reason = "Errored"
	Denied  reason = "Denied"
)

type ErrorPattern struct {
	Message string
	Reason  reason
}

func (e *ErrorPattern) Error() string {
	return e.Message
}

func Trigger(p Pattern, ctx context.Context) *ErrorPattern {

	diffErr := p.Diff(ctx)
	if diffErr != nil {
		return diffErr
	}
	removeErr := p.Remove(ctx)
	if removeErr != nil {
		return removeErr
	}
	setupErr := p.Setup(ctx)
	if setupErr != nil {
		return setupErr
	}

	return nil
}
